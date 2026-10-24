package privacy

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	wlDisplayID              = 1
	wlOutputInterface        = "wl_output"
	gammaManagerInterface    = "zwlr_gamma_control_manager_v1"
	gammaControlMaxPayload   = 64 << 10
	gammaControlSetupTimeout = 4 * time.Second
	gammaTableTempPattern    = "niridesk-gamma-*"
)

type gammaGlobal struct {
	name    uint32
	iface   string
	version uint32
}

type gammaControl struct {
	id       uint32
	outputID uint32
	size     uint32
	initial  bool
}

type gammaSession struct {
	conn   *net.UnixConn
	reader *bufio.Reader
	logger *slog.Logger

	writeMu sync.Mutex
	nextID  uint32

	registryID uint32
	managerID  uint32

	initializing    bool
	initialCount    int
	initialSizes    int
	initialFailures int
	globals         []gammaGlobal
	controls        map[uint32]*gammaControl
	outputControls  map[uint32]uint32
	callbacks       map[uint32]bool

	done      chan struct{}
	closeOnce sync.Once
	closeMu   sync.Mutex
	closing   bool
	errMu     sync.Mutex
	lastError string

	outputCount int
}

// DisplayAvailable reports whether a Wayland socket is configured and present.
func DisplayAvailable() bool {
	_, err := waylandSocketPath()
	return err == nil
}

func waylandSocketPath() (string, error) {
	display := strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY"))
	if display == "" {
		return "", errors.New("WAYLAND_DISPLAY is not set")
	}
	path := display
	if !filepath.IsAbs(path) {
		runtimeDir := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR"))
		if runtimeDir == "" {
			return "", errors.New("XDG_RUNTIME_DIR is not set")
		}
		path = filepath.Join(runtimeDir, display)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat Wayland socket: %w", err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return "", fmt.Errorf("Wayland path is not a socket: %s", path)
	}
	return path, nil
}

func startGammaSession(ctx context.Context, logger *slog.Logger) (*gammaSession, error) {
	path, err := waylandSocketPath()
	if err != nil {
		return nil, err
	}
	dialer := net.Dialer{Timeout: gammaControlSetupTimeout}
	rawConn, err := dialer.DialContext(ctx, "unix", path)
	if err != nil {
		return nil, fmt.Errorf("connect Wayland display: %w", err)
	}
	conn, ok := rawConn.(*net.UnixConn)
	if !ok {
		_ = rawConn.Close()
		return nil, errors.New("Wayland connection is not a Unix socket")
	}
	session := &gammaSession{
		conn:           conn,
		reader:         bufio.NewReaderSize(conn, 32<<10),
		logger:         logger,
		nextID:         2,
		controls:       make(map[uint32]*gammaControl),
		outputControls: make(map[uint32]uint32),
		callbacks:      make(map[uint32]bool),
		done:           make(chan struct{}),
	}
	if err := session.initialize(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	go session.run()
	return session, nil
}

func (s *gammaSession) initialize(ctx context.Context) error {
	deadline := time.Now().Add(gammaControlSetupTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = s.conn.SetReadDeadline(deadline)
	defer func() { _ = s.conn.SetReadDeadline(time.Time{}) }()

	s.initializing = true
	s.registryID = s.newID()
	if err := s.sendRequest(wlDisplayID, 1, uint32Payload(s.registryID)); err != nil {
		return fmt.Errorf("get Wayland registry: %w", err)
	}
	if err := s.roundtrip(); err != nil {
		return fmt.Errorf("read Wayland registry: %w", err)
	}

	for _, global := range s.globals {
		switch global.iface {
		case gammaManagerInterface:
			s.managerID = s.newID()
			if err := s.bindGlobal(global, s.managerID, 1); err != nil {
				return fmt.Errorf("bind gamma control manager: %w", err)
			}
		}
	}
	if s.managerID == 0 {
		return errors.New("compositor does not support wlr-gamma-control")
	}

	outputs := make([]gammaGlobal, 0)
	for _, global := range s.globals {
		if global.iface == wlOutputInterface {
			outputs = append(outputs, global)
		}
	}
	if len(outputs) == 0 {
		return errors.New("no Wayland outputs available")
	}
	s.initialCount = len(outputs)
	s.initializing = false
	for _, output := range outputs {
		if err := s.bindOutput(output, true); err != nil {
			return fmt.Errorf("bind Wayland output %s: %w", output.iface, err)
		}
	}

	for s.initialSizes+s.initialFailures < s.initialCount {
		if err := s.readAndDispatch(); err != nil {
			return fmt.Errorf("wait for gamma controls: %w", err)
		}
	}
	if s.initialFailures > 0 {
		return fmt.Errorf("gamma control failed for %d output(s)", s.initialFailures)
	}
	if s.initialSizes == 0 {
		return errors.New("no gamma-capable outputs")
	}

	// Ensure the black ramps were processed before declaring the mode active.
	if err := s.roundtrip(); err != nil {
		return fmt.Errorf("apply gamma tables: %w", err)
	}
	if s.initialFailures > 0 {
		return fmt.Errorf("gamma control failed for %d output(s)", s.initialFailures)
	}
	s.outputCount = s.initialSizes
	return nil
}

func (s *gammaSession) run() {
	defer close(s.done)
	defer s.conn.Close()
	for {
		if err := s.readAndDispatch(); err != nil {
			if !s.isClosing() {
				s.setLastError(err)
			}
			return
		}
	}
}

// Close drops the Wayland connection. The compositor restores every gamma
// table when the gamma control objects are destroyed implicitly.
func (s *gammaSession) Close() error {
	s.closeOnce.Do(func() {
		s.closeMu.Lock()
		s.closing = true
		s.closeMu.Unlock()
		_ = s.conn.Close()
	})
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
		return errors.New("Wayland gamma session did not stop")
	}
	return nil
}

func (s *gammaSession) isClosing() bool {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	return s.closing
}

func (s *gammaSession) Err() error {
	s.errMu.Lock()
	defer s.errMu.Unlock()
	if s.lastError == "" {
		return nil
	}
	return errors.New(s.lastError)
}

func (s *gammaSession) setLastError(err error) {
	s.errMu.Lock()
	defer s.errMu.Unlock()
	s.lastError = err.Error()
}

func (s *gammaSession) OutputCount() int {
	return s.outputCount
}

func (s *gammaSession) newID() uint32 {
	id := s.nextID
	s.nextID++
	return id
}

func (s *gammaSession) bindGlobal(global gammaGlobal, id, version uint32) error {
	if version > global.version {
		version = global.version
	}
	payload := make([]byte, 0, 64)
	payload = appendUint32(payload, global.name)
	payload = appendString(payload, global.iface)
	payload = appendUint32(payload, version)
	payload = appendUint32(payload, id)
	return s.sendRequest(s.registryID, 0, payload)
}

func (s *gammaSession) bindOutput(global gammaGlobal, initial bool) error {
	version := global.version
	if version > 4 {
		version = 4
	}
	outputID := s.newID()
	if err := s.bindGlobal(global, outputID, version); err != nil {
		return err
	}
	controlID := s.newID()
	payload := make([]byte, 8)
	binary.NativeEndian.PutUint32(payload[0:4], controlID)
	binary.NativeEndian.PutUint32(payload[4:8], outputID)
	if err := s.sendRequest(s.managerID, 0, payload); err != nil {
		return err
	}
	s.controls[controlID] = &gammaControl{id: controlID, outputID: outputID, initial: initial}
	s.outputControls[outputID] = controlID
	return nil
}

func (s *gammaSession) roundtrip() error {
	callbackID := s.newID()
	s.callbacks[callbackID] = true
	if err := s.sendRequest(wlDisplayID, 0, uint32Payload(callbackID)); err != nil {
		return err
	}
	for s.callbacks[callbackID] {
		if err := s.readAndDispatch(); err != nil {
			return err
		}
	}
	delete(s.callbacks, callbackID)
	return nil
}

func (s *gammaSession) readAndDispatch() error {
	objectID, opcode, payload, err := readWaylandMessage(s.reader)
	if err != nil {
		return err
	}
	return s.dispatch(objectID, opcode, payload)
}

func (s *gammaSession) dispatch(objectID uint32, opcode uint16, payload []byte) error {
	switch objectID {
	case wlDisplayID:
		if opcode == 0 {
			return parseDisplayError(payload)
		}
		return nil
	case s.registryID:
		switch opcode {
		case 0:
			global, err := parseGlobal(payload)
			if err != nil {
				return err
			}
			s.handleGlobal(global)
		case 1:
			// Output removal is reported by the gamma control failed event.
		}
		return nil
	}
	if s.callbacks[objectID] {
		if opcode == 0 {
			s.callbacks[objectID] = false
		}
		return nil
	}
	if _, isOutput := s.outputControls[objectID]; isOutput {
		if opcode == 2 {
			s.handleOutputDone(objectID)
		}
		return nil
	}
	control, ok := s.controls[objectID]
	if !ok {
		return nil
	}
	switch opcode {
	case 0:
		if len(payload) < 4 {
			return errors.New("short gamma_size event")
		}
		size := binary.NativeEndian.Uint32(payload[0:4])
		control.size = size
		if control.initial {
			s.initialSizes++
		}
		if size == 0 {
			return s.gammaFailed(control, errors.New("output has no gamma table"))
		}
		return s.setGammaBlack(control)
	case 1:
		return s.gammaFailed(control, errors.New("gamma control was rejected"))
	}
	return nil
}

func (s *gammaSession) handleGlobal(global gammaGlobal) {
	if s.initializing {
		s.globals = append(s.globals, global)
		return
	}
	if global.iface != wlOutputInterface || s.managerID == 0 {
		return
	}
	if err := s.bindOutput(global, false); err != nil {
		s.setLastError(err)
		s.logger.Warn("failed to bind new Wayland output for anti-peek; restoring displays", "error", err)
		_ = s.conn.Close()
	}
}

func (s *gammaSession) gammaFailed(control *gammaControl, cause error) error {
	if control.initial {
		s.initialFailures++
	}
	delete(s.controls, control.id)
	delete(s.outputControls, control.outputID)
	err := fmt.Errorf("%w (output control %d)", cause, control.id)
	s.setLastError(err)
	return err
}

func (s *gammaSession) setGammaBlack(control *gammaControl) error {
	if control.size == 0 {
		return errors.New("gamma table size is zero")
	}
	total := int64(control.size) * 3 * 2
	file, err := os.CreateTemp("", gammaTableTempPattern)
	if err != nil {
		return fmt.Errorf("create gamma table: %w", err)
	}
	name := file.Name()
	defer func() {
		_ = file.Close()
		_ = os.Remove(name)
	}()
	if err := file.Truncate(total); err != nil {
		return fmt.Errorf("size gamma table: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek gamma table: %w", err)
	}
	message := waylandMessage(control.id, 0, nil)
	s.writeMu.Lock()
	_, _, err = s.conn.WriteMsgUnix(message, syscall.UnixRights(int(file.Fd())), nil)
	s.writeMu.Unlock()
	if err != nil {
		return fmt.Errorf("send gamma table: %w", err)
	}
	return nil
}

func (s *gammaSession) sendRequest(objectID uint32, opcode uint16, payload []byte) error {
	message := waylandMessage(objectID, opcode, payload)
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.conn.Write(message)
	if err != nil {
		return fmt.Errorf("write Wayland request: %w", err)
	}
	return nil
}

func (s *gammaSession) handleOutputDone(outputID uint32) {
	controlID, ok := s.outputControls[outputID]
	if !ok {
		return
	}
	control := s.controls[controlID]
	if control == nil || control.size == 0 {
		return
	}
	if err := s.setGammaBlack(control); err != nil {
		s.setLastError(err)
		s.logger.Warn("failed to reapply anti-peek gamma", "error", err)
	}
}

func waylandMessage(objectID uint32, opcode uint16, payload []byte) []byte {
	size := 8 + len(payload)
	message := make([]byte, size)
	binary.NativeEndian.PutUint32(message[0:4], objectID)
	binary.NativeEndian.PutUint32(message[4:8], uint32(size)<<16|uint32(opcode))
	copy(message[8:], payload)
	return message
}

func readWaylandMessage(reader *bufio.Reader) (uint32, uint16, []byte, error) {
	var header [8]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return 0, 0, nil, err
	}
	objectID := binary.NativeEndian.Uint32(header[0:4])
	second := binary.NativeEndian.Uint32(header[4:8])
	size := int(second >> 16)
	opcode := uint16(second)
	if size < 8 || size > gammaControlMaxPayload {
		return 0, 0, nil, fmt.Errorf("invalid Wayland message size %d", size)
	}
	payload := make([]byte, size-8)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return 0, 0, nil, err
	}
	return objectID, opcode, payload, nil
}

func parseGlobal(payload []byte) (gammaGlobal, error) {
	if len(payload) < 4 {
		return gammaGlobal{}, errors.New("short registry global")
	}
	global := gammaGlobal{name: binary.NativeEndian.Uint32(payload[0:4])}
	offset := 4
	iface, next, err := readWaylandString(payload, offset)
	if err != nil {
		return gammaGlobal{}, err
	}
	global.iface = iface
	if len(payload) < next+4 {
		return gammaGlobal{}, errors.New("short registry global version")
	}
	global.version = binary.NativeEndian.Uint32(payload[next : next+4])
	return global, nil
}

func parseDisplayError(payload []byte) error {
	if len(payload) < 8 {
		return errors.New("Wayland protocol error")
	}
	objectID := binary.NativeEndian.Uint32(payload[0:4])
	code := binary.NativeEndian.Uint32(payload[4:8])
	message, _, err := readWaylandString(payload, 8)
	if err != nil {
		return fmt.Errorf("Wayland protocol error on object %d (code %d)", objectID, code)
	}
	return fmt.Errorf("Wayland protocol error on object %d (code %d): %s", objectID, code, message)
}

func readWaylandString(payload []byte, offset int) (string, int, error) {
	if len(payload) < offset+4 {
		return "", offset, errors.New("short Wayland string length")
	}
	length := int(binary.NativeEndian.Uint32(payload[offset : offset+4]))
	offset += 4
	if length <= 0 || len(payload) < offset+length {
		return "", offset, errors.New("short Wayland string")
	}
	value := string(payload[offset : offset+length-1])
	offset += (length + 3) &^ 3
	return value, offset, nil
}

func appendUint32(payload []byte, value uint32) []byte {
	var buf [4]byte
	binary.NativeEndian.PutUint32(buf[:], value)
	return append(payload, buf[:]...)
}

func appendString(payload []byte, value string) []byte {
	length := len(value) + 1
	payload = appendUint32(payload, uint32(length))
	payload = append(payload, value...)
	payload = append(payload, 0)
	for len(payload)%4 != 0 {
		payload = append(payload, 0)
	}
	return payload
}

func uint32Payload(value uint32) []byte {
	payload := make([]byte, 4)
	binary.NativeEndian.PutUint32(payload, value)
	return payload
}
