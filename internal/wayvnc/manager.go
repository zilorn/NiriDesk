package wayvnc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/zilorn/niridesk/internal/config"
)

// Output describes a Wayland output reported by wayvncctl.
type Output struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Captured    bool   `json:"captured"`
	Power       string `json:"power"`
}

// State is the observable supervisor state.
type State struct {
	Managed   bool   `json:"managed"`
	Running   bool   `json:"running"`
	Restarts  int    `json:"restarts"`
	LastError string `json:"last_error,omitempty"`
	Version   string `json:"version,omitempty"`
	NeatVNC   string `json:"neatvnc,omitempty"`
	AML       string `json:"aml,omitempty"`
}

// Manager owns the wayvnc process and its control socket.
type Manager struct {
	cfg        config.Wayvnc
	vncAddress string
	logger     *slog.Logger
	done       chan struct{}

	mu        sync.RWMutex
	running   bool
	restarts  int
	lastError string
}

// NewManager creates a manager. Call Start to launch the supervised process.
func NewManager(cfg config.Wayvnc, vncAddress string, logger *slog.Logger) *Manager {
	m := &Manager{cfg: cfg, vncAddress: vncAddress, logger: logger, done: make(chan struct{})}
	if !cfg.Managed {
		close(m.done)
	}
	return m
}

// Start launches the supervisor loop.
func (m *Manager) Start(ctx context.Context) {
	if !m.cfg.Managed {
		m.logger.Info("wayvnc supervision disabled; using an external VNC server")
		return
	}
	go m.supervise(ctx)
}

// Wait blocks until supervision has stopped.
func (m *Manager) Wait() {
	<-m.done
}

func (m *Manager) supervise(ctx context.Context) {
	defer close(m.done)
	delay := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		started := time.Now()
		err := m.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}

		if err != nil {
			m.setFailure(err)
			m.logger.Error("wayvnc stopped", "error", err)
		} else {
			m.setFailure(errors.New("wayvnc exited"))
			m.logger.Warn("wayvnc exited unexpectedly")
		}

		m.mu.Lock()
		m.restarts++
		m.mu.Unlock()

		if time.Since(started) > time.Minute {
			delay = time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if delay < 30*time.Second {
			delay *= 2
		}
	}
}

func (m *Manager) runOnce(ctx context.Context) error {
	if err := m.prepareControlSocket(); err != nil {
		return err
	}

	host, port, err := net.SplitHostPort(m.vncAddressForWayvnc())
	if err != nil {
		return fmt.Errorf("resolve VNC address: %w", err)
	}

	args := []string{
		"-L", m.cfg.LogLevel,
		"-S", m.cfg.ControlSocket,
		"-f", strconv.Itoa(m.cfg.MaxFPS),
	}
	if m.cfg.ShowCursor {
		args = append(args, "-r")
	}
	if m.cfg.DisableInput {
		args = append(args, "-d")
	}
	if m.cfg.Output != "" {
		args = append(args, "-o", m.cfg.Output)
	}
	args = append(args, host, port)

	cmd := exec.CommandContext(ctx, m.cfg.Binary, args...)
	cmd.Env = os.Environ()
	cmd.Stdout = &lineWriter{logger: m.logger, stream: "stdout"}
	cmd.Stderr = &lineWriter{logger: m.logger, stream: "stderr"}
	cmd.WaitDelay = 3 * time.Second

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start wayvnc: %w", err)
	}

	m.mu.Lock()
	m.running = true
	m.lastError = ""
	m.mu.Unlock()
	m.logger.Info("wayvnc started", "pid", cmd.Process.Pid, "address", net.JoinHostPort(host, port))

	err = cmd.Wait()

	m.mu.Lock()
	m.running = false
	m.mu.Unlock()

	if ctx.Err() != nil {
		return nil
	}
	if err != nil {
		return fmt.Errorf("wayvnc process: %w", err)
	}
	return nil
}

func (m *Manager) prepareControlSocket() error {
	info, err := os.Lstat(m.cfg.ControlSocket)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect wayvnc control socket: %w", err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("refusing to remove non-socket control path %q", m.cfg.ControlSocket)
	}
	if err := os.Remove(m.cfg.ControlSocket); err != nil {
		return fmt.Errorf("remove stale wayvnc control socket: %w", err)
	}
	return nil
}

func (m *Manager) setFailure(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err == nil {
		m.lastError = ""
		return
	}
	m.lastError = err.Error()
}

// State returns a snapshot of the supervisor state and wayvnc versions.
func (m *Manager) State(ctx context.Context) State {
	m.mu.RLock()
	state := State{
		Managed:   m.cfg.Managed,
		Running:   m.running,
		Restarts:  m.restarts,
		LastError: m.lastError,
	}
	m.mu.RUnlock()

	if versions, err := m.versions(ctx); err == nil {
		state.Version = versions["wayvnc"]
		state.NeatVNC = versions["neatvnc"]
		state.AML = versions["aml"]
	}
	return state
}

// Outputs queries the current output list through the wayvnc control socket.
func (m *Manager) Outputs(ctx context.Context) ([]Output, error) {
	raw, err := m.control(ctx, "-j", "output-list")
	if err != nil {
		return nil, err
	}
	var outputs []Output
	if err := json.Unmarshal(raw, &outputs); err != nil {
		return nil, fmt.Errorf("decode wayvnc output list: %w", err)
	}
	return outputs, nil
}

// SetOutput asks wayvnc to switch the captured output.
func (m *Manager) SetOutput(ctx context.Context, name string) error {
	if !config.ValidOutputName(name) {
		return errors.New("invalid output name")
	}
	if _, err := m.control(ctx, "output-set", name); err != nil {
		return err
	}
	return nil
}

func (m *Manager) versions(ctx context.Context) (map[string]string, error) {
	raw, err := m.control(ctx, "-j", "version")
	if err != nil {
		return nil, err
	}
	var versions map[string]string
	if err := json.Unmarshal(raw, &versions); err != nil {
		return nil, fmt.Errorf("decode wayvnc version: %w", err)
	}
	return versions, nil
}

func (m *Manager) control(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	fullArgs := append([]string{"-S", m.cfg.ControlSocket}, args...)
	cmd := exec.CommandContext(ctx, m.cfg.ControlBinary, fullArgs...)
	raw, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("wayvncctl %s: %s", args[len(args)-1], string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("wayvncctl %s: %w", args[len(args)-1], err)
	}
	return raw, nil
}

type lineWriter struct {
	logger *slog.Logger
	stream string
	mu     sync.Mutex
	buf    []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		index := -1
		for i, b := range w.buf {
			if b == '\n' {
				index = i
				break
			}
		}
		if index < 0 {
			break
		}
		line := string(w.buf[:index])
		w.buf = append(w.buf[:0], w.buf[index+1:]...)
		if line != "" {
			level := slog.LevelInfo
			if w.stream == "stderr" {
				level = slog.LevelWarn
			}
			w.logger.Log(context.Background(), level, "wayvnc", "stream", w.stream, "message", line)
		}
	}
	if len(w.buf) > 32*1024 {
		w.logger.Warn("wayvnc emitted an oversized log line", "bytes", len(w.buf))
		w.buf = w.buf[:0]
	}
	return len(p), nil
}

// vncAddressForWayvnc returns an address bound to loopback. VNC traffic must
// never be exposed directly; the authenticated WebSocket gateway is the only
// client-facing entry point.
func (m *Manager) vncAddressForWayvnc() string {
	_, port, err := net.SplitHostPort(m.vncAddress)
	if err != nil {
		return "127.0.0.1:5900"
	}
	return net.JoinHostPort("127.0.0.1", port)
}
