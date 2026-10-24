package audio

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zilorn/niridesk/internal/config"
)

func fakeCapture(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "parec")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestStreamDeliversTimestampedFrames(t *testing.T) {
	binary := fakeCapture(t, "head -c 7680 /dev/zero")
	stream, err := Start(context.Background(), config.Audio{
		Enabled: true,
		Binary:  binary,
		Device:  "@DEFAULT_MONITOR@",
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer stream.Close()

	first, err := stream.NextFrame()
	if err != nil {
		t.Fatalf("first frame: %v", err)
	}
	second, err := stream.NextFrame()
	if err != nil {
		t.Fatalf("second frame: %v", err)
	}
	if first.Sequence != 0 || second.Sequence != 1 {
		t.Fatalf("unexpected sequences: %d, %d", first.Sequence, second.Sequence)
	}
	if len(first.PCM) != FrameBytes || len(second.PCM) != FrameBytes {
		t.Fatalf("unexpected frame sizes: %d, %d", len(first.PCM), len(second.PCM))
	}
	if delta := second.Timestamp.Sub(first.Timestamp); delta != FrameDuration {
		t.Fatalf("unexpected timestamp delta: %s", delta)
	}
	if _, err := stream.NextFrame(); err == nil {
		t.Fatal("expected capture EOF")
	} else if elapsed := time.Since(first.Timestamp); elapsed > time.Minute {
		t.Fatalf("unexpected timestamp drift: %s", elapsed)
	}
}

func TestStartRejectsDisabledCapture(t *testing.T) {
	if _, err := Start(context.Background(), config.Audio{}); err == nil {
		t.Fatal("expected disabled capture to fail")
	}
	if Available(config.Audio{}) {
		t.Fatal("disabled audio must not be reported as available")
	}
}

func TestAvailableFindsCaptureBinary(t *testing.T) {
	binary := fakeCapture(t, "exit 0")
	if !Available(config.Audio{Enabled: true, Binary: binary}) {
		t.Fatal("expected fake capture binary to be available")
	}
	if Available(config.Audio{Enabled: true, Binary: filepath.Join(t.TempDir(), "missing")}) {
		t.Fatal("missing binary reported as available")
	}
}
