package powerlock

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

type fakeBackend struct {
	mu        sync.Mutex
	available bool
	lockErr   error
	calls     int
}

func (f *fakeBackend) Available() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.available
}

func (f *fakeBackend) Lock(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.lockErr
}

func (f *fakeBackend) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestControllerToggleAndLock(t *testing.T) {
	backend := &fakeBackend{available: true}
	controller := NewControllerWithBackend(testLogger(), backend)

	state := controller.State()
	if !state.Supported || state.Enabled || state.LastError != "" {
		t.Fatalf("unexpected initial state: %+v", state)
	}

	if err := controller.Lock(t.Context()); err != nil {
		t.Fatal(err)
	}
	if backend.callCount() != 0 {
		t.Fatal("disabled Power Lock should not lock")
	}

	if err := controller.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	if state := controller.State(); !state.Enabled {
		t.Fatal("expected Power Lock to be enabled")
	}

	if err := controller.Lock(t.Context()); err != nil {
		t.Fatal(err)
	}
	if backend.callCount() != 1 {
		t.Fatalf("unexpected lock calls: %d", backend.callCount())
	}

	if err := controller.SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	if err := controller.Lock(t.Context()); err != nil {
		t.Fatal(err)
	}
	if backend.callCount() != 1 {
		t.Fatal("disabled Power Lock should not lock again")
	}
}

func TestControllerUnavailable(t *testing.T) {
	backend := &fakeBackend{available: false}
	controller := NewControllerWithBackend(testLogger(), backend)

	if err := controller.SetEnabled(true); err == nil {
		t.Fatal("expected enabling Power Lock without niri to fail")
	}
	state := controller.State()
	if state.Supported || state.Enabled || state.LastError == "" {
		t.Fatalf("unexpected unavailable state: %+v", state)
	}

	if err := controller.SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	if state := controller.State(); state.LastError != "" {
		t.Fatalf("expected disable to clear the error: %+v", state)
	}
}

func TestControllerLockRecordsError(t *testing.T) {
	backend := &fakeBackend{available: true, lockErr: errors.New("locker missing")}
	controller := NewControllerWithBackend(testLogger(), backend)
	if err := controller.SetEnabled(true); err != nil {
		t.Fatal(err)
	}

	err := controller.Lock(t.Context())
	if err == nil || !strings.Contains(err.Error(), "locker missing") {
		t.Fatalf("unexpected lock error: %v", err)
	}
	if state := controller.State(); !strings.Contains(state.LastError, "locker missing") {
		t.Fatalf("expected last error to be recorded: %+v", state)
	}

	backend.mu.Lock()
	backend.lockErr = nil
	backend.mu.Unlock()
	if err := controller.Lock(t.Context()); err != nil {
		t.Fatal(err)
	}
	if state := controller.State(); state.LastError != "" {
		t.Fatalf("expected a successful lock to clear the error: %+v", state)
	}
}
