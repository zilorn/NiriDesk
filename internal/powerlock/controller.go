// Package powerlock locks the host session after the last remote viewer
// disconnects. The lock request is sent through niri's IPC socket so it runs in
// the host session, then the physical outputs are powered off.
package powerlock

import (
	"context"
	"errors"
	"log/slog"
	"sync"
)

// State is the observable Power Lock preference.
type State struct {
	Enabled   bool   `json:"enabled"`
	Supported bool   `json:"supported"`
	LastError string `json:"last_error,omitempty"`
}

// Backend performs the host-side lock. It is an interface so the controller
// can be tested without a running niri session.
type Backend interface {
	Available() bool
	Lock(context.Context) error
}

// Controller owns the shared Power Lock preference.
type Controller struct {
	logger  *slog.Logger
	backend Backend

	mu        sync.Mutex
	enabled   bool
	locking   bool
	lastError string
}

// NewController creates a Power Lock controller backed by niri IPC.
func NewController(logger *slog.Logger) *Controller {
	return NewControllerWithBackend(logger, NewNiriBackend())
}

// NewControllerWithBackend creates a Power Lock controller with an injected
// backend.
func NewControllerWithBackend(logger *slog.Logger, backend Backend) *Controller {
	return &Controller{logger: logger, backend: backend}
}

// State returns a snapshot of the current Power Lock state.
func (c *Controller) State() State {
	c.mu.Lock()
	state := State{
		Enabled:   c.enabled,
		LastError: c.lastError,
	}
	c.mu.Unlock()
	state.Supported = c.backend.Available()
	return state
}

// SetEnabled changes whether the host should be locked after the last remote
// viewer disconnects.
func (c *Controller) SetEnabled(enabled bool) error {
	if enabled && !c.backend.Available() {
		err := errors.New("niri IPC is not available")
		c.mu.Lock()
		c.lastError = err.Error()
		c.mu.Unlock()
		return err
	}

	c.mu.Lock()
	c.enabled = enabled
	c.lastError = ""
	c.mu.Unlock()
	c.logger.Info("power lock preference changed", "enabled", enabled)
	return nil
}

// Lock runs the host lock sequence once. It is a no-op when Power Lock is
// disabled or another lock is already in flight.
func (c *Controller) Lock(ctx context.Context) error {
	c.mu.Lock()
	if !c.enabled || c.locking {
		c.mu.Unlock()
		return nil
	}
	c.locking = true
	c.mu.Unlock()

	err := c.backend.Lock(ctx)

	c.mu.Lock()
	c.locking = false
	if err != nil {
		c.lastError = err.Error()
	} else {
		c.lastError = ""
	}
	c.mu.Unlock()

	if err != nil {
		c.logger.Warn("power lock failed", "error", err)
		return err
	}
	c.logger.Info("host screen locked after the last remote session")
	return nil
}
