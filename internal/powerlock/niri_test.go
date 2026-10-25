package powerlock

import (
	"bufio"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNiriSocketPaths(t *testing.T) {
	dir := t.TempDir()
	socket := filepath.Join(dir, "niri.wayland-test.42.sock")
	listener := listenUnix(t, socket)
	defer listener.Close()
	other := filepath.Join(dir, "niri.wayland-other.7.sock")
	otherListener := listenUnix(t, other)
	defer otherListener.Close()

	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("WAYLAND_DISPLAY", "wayland-test")
	t.Setenv("NIRI_SOCKET", "")

	paths, err := niriSocketPaths()
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != socket {
		t.Fatalf("unexpected socket paths: %v", paths)
	}
	if !NewNiriBackend().Available() {
		t.Fatal("expected niri IPC to be available")
	}

	t.Setenv("NIRI_SOCKET", other)
	paths, err = niriSocketPaths()
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != other || paths[1] != socket {
		t.Fatalf("expected NIRI_SOCKET to be preferred: %v", paths)
	}

	t.Setenv("NIRI_SOCKET", "")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	if _, err := niriSocketPaths(); err == nil {
		t.Fatal("expected a missing niri socket to fail")
	}
	if NewNiriBackend().Available() {
		t.Fatal("did not expect niri IPC to be available")
	}
}

func TestNiriBackendLockSequence(t *testing.T) {
	dir := t.TempDir()
	socket := filepath.Join(dir, "niri.wayland-test.99.sock")
	listener := listenUnix(t, socket)
	defer listener.Close()
	t.Setenv("NIRI_SOCKET", socket)
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("WAYLAND_DISPLAY", "wayland-test")

	requests := make(chan string, 2)
	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		for range 2 {
			line, err := reader.ReadString('\n')
			if err != nil {
				done <- err
				return
			}
			requests <- strings.TrimSuffix(line, "\n")
			if _, err := conn.Write([]byte(`{"Ok":"Handled"}` + "\n")); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()

	if err := NewNiriBackend().Lock(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	close(requests)

	var got []string
	for request := range requests {
		got = append(got, request)
	}
	if len(got) != 2 || got[0] != niriLockRequest || got[1] != niriPowerOffRequest {
		t.Fatalf("unexpected niri requests: %v", got)
	}
}

func TestNiriBackendLockError(t *testing.T) {
	dir := t.TempDir()
	socket := filepath.Join(dir, "niri.wayland-test.100.sock")
	listener := listenUnix(t, socket)
	defer listener.Close()
	t.Setenv("NIRI_SOCKET", socket)
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("WAYLAND_DISPLAY", "wayland-test")

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = bufio.NewReader(conn).ReadString('\n')
		_, _ = conn.Write([]byte(`{"Err":"no session"}` + "\n"))
	}()

	err := NewNiriBackend().Lock(t.Context())
	if err == nil || !strings.Contains(err.Error(), "no session") {
		t.Fatalf("unexpected lock error: %v", err)
	}
	<-done
}

func TestNiriBackendFallsBackToNextSocket(t *testing.T) {
	dir := t.TempDir()
	stalePath := filepath.Join(dir, "niri.wayland-test.1.sock")
	stale := listenUnix(t, stalePath)
	if unix, ok := stale.(*net.UnixListener); ok {
		unix.SetUnlinkOnClose(false)
	}
	_ = stale.Close()
	if !isSocket(stalePath) {
		t.Fatal("expected the stale socket path to remain")
	}

	livePath := filepath.Join(dir, "niri.wayland-test.2.sock")
	live := listenUnix(t, livePath)
	defer live.Close()
	t.Setenv("NIRI_SOCKET", stalePath)
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("WAYLAND_DISPLAY", "wayland-test")

	done := make(chan error, 1)
	go func() {
		conn, err := live.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		for range 2 {
			if _, err := reader.ReadString('\n'); err != nil {
				done <- err
				return
			}
			if _, err := conn.Write([]byte(`{"Ok":"Handled"}` + "\n")); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()

	if err := NewNiriBackend().Lock(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestNiriBackendHonorsContextDeadline(t *testing.T) {
	dir := t.TempDir()
	socket := filepath.Join(dir, "niri.wayland-test.101.sock")
	listener := listenUnix(t, socket)
	defer listener.Close()
	t.Setenv("NIRI_SOCKET", socket)
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("WAYLAND_DISPLAY", "wayland-test")

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = bufio.NewReader(conn).ReadString('\n')
		time.Sleep(300 * time.Millisecond)
	}()

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if err := NewNiriBackend().Lock(ctx); err == nil {
		t.Fatal("expected the context deadline to abort the lock")
	}
	<-done
}

func listenUnix(t *testing.T, path string) net.Listener {
	t.Helper()
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

func TestIsSocketRejectsRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-socket")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if isSocket(path) {
		t.Fatal("regular file must not be treated as a socket")
	}
}
