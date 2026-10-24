// Package privacy implements anti-peek mode by setting the physical outputs'
// gamma tables to black through wlr-gamma-control. Wayland screencopy renders
// the scene before the output gamma transform, so wayvnc still captures the
// normal desktop while the local display appears black. Input events do not
// clear this state, and the compositor restores gamma when this connection is
// closed.
package privacy

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

const sessionCloseTimeout = 2 * time.Second

// State is the observable anti-peek state.
type State struct {
	Enabled   bool   `json:"enabled"`
	Supported bool   `json:"supported"`
	LastError string `json:"last_error,omitempty"`
}

// Controller owns the shared anti-peek session.
type Controller struct {
	logger *slog.Logger

	mu        sync.Mutex
	starting  bool
	enabled   bool
	lastError string
	session   *gammaSession
}

// NewController creates an anti-peek controller.
func NewController(logger *slog.Logger) *Controller {
	return &Controller{logger: logger}
}

// State returns a snapshot of the current anti-peek state.
func (c *Controller) State() State {
	c.mu.Lock()
	state := State{
		Enabled:   c.enabled,
		Supported: DisplayAvailable(),
		LastError: c.lastError,
	}
	session := c.session
	c.mu.Unlock()
	if session != nil {
		if err := session.Err(); err != nil {
			state.LastError = err.Error()
		}
	}
	return state
}

// Enable blacks out every physical output while keeping screencopy active.
func (c *Controller) Enable(ctx context.Context) error {
	c.mu.Lock()
	if c.enabled || c.starting {
		c.mu.Unlock()
		return nil
	}
	c.starting = true
	c.mu.Unlock()

	session, err := startGammaSession(ctx, c.logger)

	c.mu.Lock()
	c.starting = false
	if err != nil {
		c.lastError = err.Error()
		c.mu.Unlock()
		return err
	}
	c.enabled = true
	c.session = session
	c.lastError = ""
	c.mu.Unlock()

	c.logger.Info("anti-peek mode enabled", "outputs", session.OutputCount())
	go c.watch(session)
	return nil
}

// Disable releases gamma control, restoring the physical displays.
func (c *Controller) Disable(context.Context) error {
	c.mu.Lock()
	if !c.enabled {
		c.mu.Unlock()
		return nil
	}
	session := c.session
	c.session = nil
	c.enabled = false
	c.mu.Unlock()

	err := session.Close()
	c.mu.Lock()
	if err != nil {
		c.lastError = err.Error()
	} else {
		c.lastError = ""
	}
	c.mu.Unlock()
	if err == nil {
		c.logger.Info("anti-peek mode disabled")
	}
	return err
}

// Close restores displays during process shutdown.
func (c *Controller) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), sessionCloseTimeout)
	defer cancel()
	if err := c.Disable(ctx); err != nil {
		c.logger.Warn("failed to restore displays on shutdown", "error", err)
	}
}

func (c *Controller) watch(session *gammaSession) {
	<-session.done
	_ = session.Close()
	err := session.Err()
	if err == nil {
		err = errors.New("anti-peek session ended")
	}

	c.mu.Lock()
	current := c.session == session
	if current {
		c.session = nil
		c.enabled = false
		c.lastError = err.Error()
	}
	c.mu.Unlock()
	if current {
		c.logger.Warn("anti-peek mode stopped", "error", err)
	}
}
