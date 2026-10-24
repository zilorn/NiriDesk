package wayvnc

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zilorn/niridesk/internal/config"
)

func fakeControlBinary(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	binary := filepath.Join(dir, "wayvncctl")
	script := `#!/bin/sh
printf '%s\n' "$@" > "$ARGS_FILE"
case " $* " in
  *" output-list "*)
    printf '[{"name":"DP-1","description":"Test Display","width":1920,"height":1080,"captured":true,"power":"ON"}]'
    ;;
  *" version "*)
    printf '{"wayvnc":"0.9.1","neatvnc":"0.9.1","aml":"0.3.0"}'
    ;;
  *) ;;
esac
`
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ARGS_FILE", argsFile)
	return binary, argsFile
}

func testManager(t *testing.T, binary string) *Manager {
	t.Helper()
	return NewManager(config.Wayvnc{
		Managed:       true,
		Binary:        "wayvnc",
		ControlBinary: binary,
		ControlSocket: "/tmp/test-wayvnc.sock",
		MaxFPS:        30,
		ShowCursor:    true,
		LogLevel:      "info",
	}, "127.0.0.1:5900", testLogger())
}

func TestOutputsAndVersion(t *testing.T) {
	binary, _ := fakeControlBinary(t)
	manager := testManager(t, binary)

	outputs, err := manager.Outputs(context.Background())
	if err != nil {
		t.Fatalf("outputs: %v", err)
	}
	if len(outputs) != 1 || outputs[0].Name != "DP-1" || !outputs[0].Captured {
		t.Fatalf("unexpected outputs: %+v", outputs)
	}
	state := manager.State(context.Background())
	if state.Version != "0.9.1" || state.NeatVNC != "0.9.1" {
		t.Fatalf("unexpected versions: %+v", state)
	}
}

func TestSetOutput(t *testing.T) {
	binary, argsFile := fakeControlBinary(t)
	manager := testManager(t, binary)

	if err := manager.SetOutput(context.Background(), "DP-1"); err != nil {
		t.Fatalf("set output: %v", err)
	}
	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "output-set\nDP-1") {
		t.Fatalf("unexpected control arguments: %q", raw)
	}

	if err := manager.SetOutput(context.Background(), "-bad name"); err == nil {
		t.Fatal("expected invalid output name to fail")
	}
}

func TestVNCAddressIsForcedToLoopback(t *testing.T) {
	manager := NewManager(config.Wayvnc{}, "192.0.2.20:5999", testLogger())
	if got := manager.vncAddressForWayvnc(); got != "127.0.0.1:5999" {
		t.Fatalf("unexpected address: %s", got)
	}
	if got := manager.vncAddressForWayvnc(); strings.Contains(got, "192.0.2.20") {
		t.Fatalf("VNC address leaked a non-loopback host: %s", got)
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
