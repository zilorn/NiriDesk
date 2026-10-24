// Package audio captures the host's PulseAudio/PipeWire monitor source and
// exposes fixed-size PCM frames for the authenticated WebSocket audio stream.
package audio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/zilorn/niridesk/internal/config"
)

const (
	SampleRate    = 48000
	Channels      = 2
	FrameDuration = 20 * time.Millisecond
	FrameSamples  = SampleRate / 50
	FrameBytes    = FrameSamples * Channels * 2
)

// Frame is one timestamped block of interleaved little-endian signed 16-bit
// PCM samples.
type Frame struct {
	Sequence  uint32
	Timestamp time.Time
	PCM       []byte
}

// Stream owns one capture process. The first frame timestamp is the process
// start time, so consecutive timestamps preserve the capture cadence even when
// individual writes are delayed by the browser or the network.
type Stream struct {
	cfg      config.Audio
	cancel   context.CancelFunc
	cmd      *exec.Cmd
	stdout   io.ReadCloser
	started  time.Time
	sequence uint32
	done     chan struct{}
	stderr   *tailBuffer

	mu      sync.Mutex
	waitErr error
}

// Available reports whether audio capture is enabled and the capture binary can
// be found.
func Available(cfg config.Audio) bool {
	if !cfg.Enabled {
		return false
	}
	_, err := exec.LookPath(cfg.Binary)
	return err == nil
}

// Start launches parec (or a compatible capture command) and begins reading
// raw PCM from its standard output.
func Start(parent context.Context, cfg config.Audio) (*Stream, error) {
	if !cfg.Enabled {
		return nil, errors.New("audio capture is disabled")
	}
	if cfg.Binary == "" || cfg.Device == "" {
		return nil, errors.New("audio capture is not configured")
	}

	ctx, cancel := context.WithCancel(parent)
	args := []string{
		"--raw",
		"--format=s16le",
		"--rate=" + strconv.Itoa(SampleRate),
		"--channels=" + strconv.Itoa(Channels),
		"--device=" + cfg.Device,
		"--latency-msec=20",
		"--process-time-msec=20",
	}
	cmd := exec.CommandContext(ctx, cfg.Binary, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open audio capture pipe: %w", err)
	}
	stderr := &tailBuffer{limit: 4 << 10}
	cmd.Stderr = stderr
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start audio capture: %w", err)
	}

	stream := &Stream{
		cfg:     cfg,
		cancel:  cancel,
		cmd:     cmd,
		stdout:  stdout,
		started: time.Now(),
		done:    make(chan struct{}),
		stderr:  stderr,
	}
	go func() {
		waitErr := cmd.Wait()
		stream.mu.Lock()
		stream.waitErr = waitErr
		stream.mu.Unlock()
		close(stream.done)
	}()
	return stream, nil
}

// NextFrame blocks until one complete PCM frame is available.
func (s *Stream) NextFrame() (Frame, error) {
	pcm := make([]byte, FrameBytes)
	if _, err := io.ReadFull(s.stdout, pcm); err != nil {
		return Frame{}, s.terminated(err)
	}
	frame := Frame{
		Sequence:  s.sequence,
		Timestamp: s.started.Add(time.Duration(s.sequence) * FrameDuration),
		PCM:       pcm,
	}
	s.sequence++
	return frame, nil
}

// Close stops the capture process. It is safe to call more than once.
func (s *Stream) Close() error {
	s.cancel()
	select {
	case <-s.done:
	case <-time.After(3 * time.Second):
		return errors.New("audio capture did not stop")
	}
	_ = s.stdout.Close()
	return nil
}

func (s *Stream) terminated(readErr error) error {
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
		return fmt.Errorf("read audio capture: %w", readErr)
	}
	s.mu.Lock()
	waitErr := s.waitErr
	s.mu.Unlock()
	if waitErr == nil {
		waitErr = readErr
	}
	if details := s.stderr.String(); details != "" {
		return fmt.Errorf("audio capture stopped: %w: %s", waitErr, details)
	}
	return fmt.Errorf("audio capture stopped: %w", waitErr)
}

type tailBuffer struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, p...)
	if len(b.data) > b.limit {
		b.data = append([]byte(nil), b.data[len(b.data)-b.limit:]...)
	}
	return len(p), nil
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.data)
}
