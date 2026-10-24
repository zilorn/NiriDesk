package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/zilorn/niridesk/internal/config"
	"github.com/zilorn/niridesk/internal/wayvnc"
	"github.com/zilorn/niridesk/internal/web"
	webassets "github.com/zilorn/niridesk/web"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "healthcheck":
			os.Exit(healthcheck())
		case "version", "--version", "-version":
			fmt.Printf("niridesk %s (%s)\n", version, commit)
			return
		default:
			fmt.Fprintf(os.Stderr, "usage: niridesk [healthcheck|version]\n")
			os.Exit(2)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(2)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)
	if cfg.SessionSecretGenerated {
		logger.Warn("SESSION_SECRET is not set; generated an in-memory secret, so sessions will not survive restarts")
	}
	if !webassets.HasFrontend() {
		logger.Error("embedded frontend assets are missing", "hint", "run make web-build or rebuild with Docker")
		os.Exit(2)
	}
	if cfg.TLSCertFile == "" && !isLoopbackListen(cfg.ListenAddr) {
		logger.Warn("serving plain HTTP on a non-loopback address; use HTTPS or a trusted tunnel before exposing niridesk")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	manager := wayvnc.NewManager(cfg.Wayvnc, cfg.VNCAddress, logger)
	manager.Start(ctx)

	handler := web.New(cfg, manager, web.BuildInfo{Version: version, Commit: commit}, logger)
	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("niridesk listening", "address", cfg.ListenAddr, "version", version, "commit", commit)
		if cfg.TLSCertFile != "" {
			serveErr <- server.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
			return
		}
		serveErr <- server.ListenAndServe()
	}()

	exitCode := 0
	select {
	case <-ctx.Done():
		logger.Info("shutting down")
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server stopped", "error", err)
			exitCode = 1
			stop()
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Warn("graceful shutdown timed out", "error", err)
		_ = server.Close()
	}

	waitDone := make(chan struct{})
	go func() {
		manager.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-time.After(4 * time.Second):
		logger.Warn("wayvnc did not stop before shutdown deadline")
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

func healthcheck() int {
	address := strings.TrimSpace(os.Getenv("LISTEN_ADDR"))
	if address == "" {
		address = ":8080"
	}
	_, port, err := net.SplitHostPort(address)
	if err != nil || port == "" {
		fmt.Fprintf(os.Stderr, "invalid LISTEN_ADDR %q\n", address)
		return 2
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", port), 2*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck failed: %v\n", err)
		return 1
	}
	_ = conn.Close()
	return 0
}

func isLoopbackListen(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	if host == "" {
		return false
	}
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}
