package privacy

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"testing"
)

func TestWaylandMessageRoundTrip(t *testing.T) {
	payload := appendUint32(nil, 42)
	message := waylandMessage(7, 3, payload)
	if len(message) != 12 {
		t.Fatalf("unexpected message length: %d", len(message))
	}
	if binary.NativeEndian.Uint32(message[0:4]) != 7 {
		t.Fatal("unexpected object id")
	}
	second := binary.NativeEndian.Uint32(message[4:8])
	if int(second>>16) != len(message) || uint16(second) != 3 {
		t.Fatalf("unexpected size/opcode word: %#x", second)
	}
	objectID, opcode, gotPayload, err := readWaylandMessage(bufio.NewReader(bytes.NewReader(message)))
	if err != nil {
		t.Fatal(err)
	}
	if objectID != 7 || opcode != 3 || !bytes.Equal(gotPayload, payload) {
		t.Fatalf("unexpected message: %d %d %v", objectID, opcode, gotPayload)
	}
}

func TestParseRegistryGlobal(t *testing.T) {
	payload := appendUint32(nil, 9)
	payload = appendString(payload, "wl_output")
	payload = appendUint32(payload, 4)
	global, err := parseGlobal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if global.name != 9 || global.iface != "wl_output" || global.version != 4 {
		t.Fatalf("unexpected global: %+v", global)
	}
}

func TestParseDisplayError(t *testing.T) {
	payload := appendUint32(nil, 12)
	payload = appendUint32(payload, 3)
	payload = appendString(payload, "bad request")
	err := parseDisplayError(payload)
	if err == nil || err.Error() != "Wayland protocol error on object 12 (code 3): bad request" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWaylandSocketPath(t *testing.T) {
	dir := t.TempDir()
	socket := filepath.Join(dir, "wayland-test")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("WAYLAND_DISPLAY", "wayland-test")
	path, err := waylandSocketPath()
	if err != nil || path != socket {
		t.Fatalf("unexpected socket path: %q %v", path, err)
	}
	if !DisplayAvailable() {
		t.Fatal("expected display to be available")
	}
}

func TestControllerEnableWithoutDisplay(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "")
	controller := NewController(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if controller.State().Supported {
		t.Fatal("did not expect gamma control without a display")
	}
	if err := controller.Enable(t.Context()); err == nil {
		t.Fatal("expected enable to fail")
	}
	if controller.State().LastError == "" {
		t.Fatal("expected an error to be recorded")
	}
}
