package powerlock

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	niriDialTimeout = 2 * time.Second
	niriIOTimeout   = 3 * time.Second

	// These are fixed niri IPC requests. The lock command intentionally avoids
	// a shell and does not interpolate client input.
	niriLockRequest     = `{"Action":{"Spawn":{"command":["loginctl","lock-session"]}}}`
	niriPowerOffRequest = `{"Action":{"PowerOffMonitors":{}}}`
)

// NiriBackend locks the host through niri IPC. niri executes the request in
// the host session, where logind and the screen locker are available.
type NiriBackend struct{}

// NewNiriBackend creates a backend that talks to the niri IPC socket.
func NewNiriBackend() *NiriBackend {
	return &NiriBackend{}
}

// Available reports whether a niri IPC socket can be found. It does not prove
// that a logind lock handler is running; that failure is reported by Lock.
func (b *NiriBackend) Available() bool {
	_, err := niriSocketPaths()
	return err == nil
}

// Lock asks the host's logind session to lock, then powers off the physical
// outputs. Powering off after the lock request means local input wakes the
// lock screen rather than the desktop.
func (b *NiriBackend) Lock(ctx context.Context) error {
	paths, err := niriSocketPaths()
	if err != nil {
		return err
	}

	var lastErr error
	for _, path := range paths {
		if err := lockViaNiri(ctx, path); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return fmt.Errorf("lock host session: %w", lastErr)
}

func lockViaNiri(ctx context.Context, path string) error {
	dialer := net.Dialer{Timeout: niriDialTimeout}
	conn, err := dialer.DialContext(ctx, "unix", path)
	if err != nil {
		return fmt.Errorf("connect niri IPC: %w", err)
	}
	defer conn.Close()

	deadline := time.Now().Add(niriIOTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetDeadline(deadline)

	reader := bufio.NewReader(conn)
	if err := niriRequest(conn, reader, niriLockRequest); err != nil {
		return fmt.Errorf("request session lock: %w", err)
	}
	if err := niriRequest(conn, reader, niriPowerOffRequest); err != nil {
		return fmt.Errorf("power off monitors: %w", err)
	}
	return nil
}

func niriRequest(conn net.Conn, reader *bufio.Reader, request string) error {
	if _, err := conn.Write([]byte(request + "\n")); err != nil {
		return err
	}
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return err
	}

	var reply struct {
		Ok  json.RawMessage `json:"Ok"`
		Err *string         `json:"Err"`
	}
	if err := json.Unmarshal(line, &reply); err != nil {
		return fmt.Errorf("decode niri reply: %w", err)
	}
	if reply.Err != nil {
		return errors.New(*reply.Err)
	}
	if len(reply.Ok) == 0 {
		return errors.New("niri returned an empty reply")
	}
	return nil
}

// niriSocketPaths returns candidate IPC sockets for the current niri session.
// niri names them niri.<WAYLAND_DISPLAY>.<pid>.sock in XDG_RUNTIME_DIR. The
// directory is mounted read-only in the container, but connecting to the
// socket is still permitted.
func niriSocketPaths() ([]string, error) {
	var paths []string
	seen := make(map[string]bool)
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" || seen[path] || !isSocket(path) {
			return
		}
		seen[path] = true
		paths = append(paths, path)
	}

	if explicit := strings.TrimSpace(os.Getenv("NIRI_SOCKET")); explicit != "" {
		add(explicit)
	}

	display := filepath.Base(strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")))
	runtimeDir := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR"))
	if display != "" && display != "." && runtimeDir != "" {
		entries, err := os.ReadDir(runtimeDir)
		if err != nil && len(paths) == 0 {
			return nil, fmt.Errorf("read Wayland runtime directory: %w", err)
		}
		prefix := "niri." + display + "."
		for _, entry := range entries {
			name := entry.Name()
			if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".sock") {
				add(filepath.Join(runtimeDir, name))
			}
		}
	}

	if len(paths) == 0 {
		return nil, errors.New("niri IPC socket is not available")
	}
	return paths, nil
}

func isSocket(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode()&os.ModeSocket != 0
}
