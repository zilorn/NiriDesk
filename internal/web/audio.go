package web

import (
	"context"
	"encoding/binary"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/zilorn/niridesk/internal/audio"
)

const (
	audioFrameHeaderSize = 24
	audioProtocolVersion = 1
	audioFormatS16LE     = 1
)

var audioFrameMagic = [4]byte{'N', 'D', 'A', '1'}

type audioStatus struct {
	Enabled       bool   `json:"enabled"`
	Available     bool   `json:"available"`
	ActiveClients int    `json:"active_clients"`
	Error         string `json:"error,omitempty"`
}

func (s *Server) handleAudioWebSocket(w http.ResponseWriter, r *http.Request) {
	if !s.authenticated(r) {
		writeError(w, http.StatusUnauthorized, "authentication_required")
		return
	}
	if !s.originAllowed(r, true) {
		writeError(w, http.StatusForbidden, "origin_not_allowed")
		return
	}
	if !s.cfg.Audio.Enabled || !audio.Available(s.cfg.Audio) {
		writeError(w, http.StatusServiceUnavailable, "audio_unavailable")
		return
	}

	select {
	case s.audioSlots <- struct{}{}:
		defer func() { <-s.audioSlots }()
	default:
		writeError(w, http.StatusServiceUnavailable, "session_limit_reached")
		return
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 8 * 1024,
		CheckOrigin:     func(*http.Request) bool { return true },
	}
	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Warn("audio websocket upgrade failed", "error", err)
		return
	}
	wsConn.SetReadLimit(1024)

	ctx, cancel := context.WithCancel(r.Context())
	stream, err := audio.Start(ctx, s.cfg.Audio)
	if err != nil {
		s.setAudioError(err)
		s.logger.Warn("audio capture failed to start", "error", err)
		_ = wsConn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "audio_capture_failed"),
			time.Now().Add(time.Second),
		)
		_ = wsConn.Close()
		cancel()
		return
	}
	defer cancel()

	s.audioActive.Add(1)
	s.setAudioError(nil)
	started := time.Now()
	s.logger.Info("audio stream started", "client", s.clientIP(r), "active_audio_clients", s.audioActive.Load())
	defer func() {
		s.audioActive.Add(-1)
		s.logger.Info("audio stream closed",
			"client", s.clientIP(r),
			"duration_ms", time.Since(started).Milliseconds(),
			"active_audio_clients", s.audioActive.Load(),
		)
	}()

	var closeOnce sync.Once
	stop := func() {
		closeOnce.Do(func() {
			cancel()
			_ = stream.Close()
			_ = wsConn.Close()
		})
	}
	defer stop()

	var readers sync.WaitGroup
	readers.Add(1)
	go func() {
		defer readers.Done()
		defer stop()
		for {
			if _, _, err := wsConn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		defer stop()
		for {
			frame, err := stream.NextFrame()
			if err != nil {
				if ctx.Err() == nil {
					s.setAudioError(err)
					s.logger.Warn("audio capture stopped", "error", err)
					_ = wsConn.WriteControl(
						websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "audio_capture_failed"),
						time.Now().Add(time.Second),
					)
				}
				return
			}
			_ = wsConn.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
			if err := wsConn.WriteMessage(websocket.BinaryMessage, encodeAudioFrame(frame)); err != nil {
				return
			}
		}
	}()

	<-writeDone
	readers.Wait()
}

func encodeAudioFrame(frame audio.Frame) []byte {
	payload := make([]byte, audioFrameHeaderSize+len(frame.PCM))
	copy(payload[0:4], audioFrameMagic[:])
	payload[4] = audioProtocolVersion
	payload[5] = audioFormatS16LE
	binary.LittleEndian.PutUint16(payload[6:8], audio.Channels)
	binary.LittleEndian.PutUint32(payload[8:12], audio.SampleRate)
	binary.LittleEndian.PutUint64(payload[12:20], uint64(frame.Timestamp.UnixNano()/int64(time.Microsecond)))
	binary.LittleEndian.PutUint32(payload[20:24], frame.Sequence)
	copy(payload[audioFrameHeaderSize:], frame.PCM)
	return payload
}

func (s *Server) setAudioError(err error) {
	s.audioErrMu.Lock()
	defer s.audioErrMu.Unlock()
	if err == nil {
		s.audioLastError = ""
		return
	}
	message := err.Error()
	if len(message) > 512 {
		message = message[:512]
	}
	s.audioLastError = message
}

func (s *Server) audioStatus() audioStatus {
	s.audioErrMu.RLock()
	lastError := s.audioLastError
	s.audioErrMu.RUnlock()
	return audioStatus{
		Enabled:       s.cfg.Audio.Enabled,
		Available:     audio.Available(s.cfg.Audio),
		ActiveClients: int(s.audioActive.Load()),
		Error:         lastError,
	}
}
