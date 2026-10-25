package web

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	vncDialTimeout  = 5 * time.Second
	wsWriteTimeout  = 30 * time.Second
	maxClientPacket = 8 << 20
)

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !s.authenticated(r) {
		writeError(w, http.StatusUnauthorized, "authentication_required")
		return
	}
	if !s.originAllowed(r, true) {
		writeError(w, http.StatusForbidden, "origin_not_allowed")
		return
	}

	select {
	case s.clientSlots <- struct{}{}:
		defer func() { <-s.clientSlots }()
	default:
		writeError(w, http.StatusServiceUnavailable, "session_limit_reached")
		return
	}

	dialer := net.Dialer{Timeout: vncDialTimeout, KeepAlive: 30 * time.Second}
	vncConn, err := dialer.DialContext(r.Context(), "tcp", s.cfg.VNCAddress)
	if err != nil {
		s.logger.Warn("connect to wayvnc failed", "address", s.cfg.VNCAddress, "error", err)
		writeError(w, http.StatusBadGateway, "vnc_unavailable")
		return
	}
	if tcp, ok := vncConn.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
		_ = tcp.SetKeepAlive(true)
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 32 * 1024,
		CheckOrigin:     func(*http.Request) bool { return true },
	}
	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		_ = vncConn.Close()
		s.logger.Warn("websocket upgrade failed", "error", err)
		return
	}
	wsConn.SetReadLimit(maxClientPacket)
	wsConn.SetReadDeadline(time.Time{})

	s.addClient()
	started := time.Now()
	s.logger.Info("remote session started",
		"client", s.clientIP(r),
		"active_clients", s.activeClient.Load(),
	)
	defer func() {
		remaining := s.removeClient()
		s.logger.Info("remote session closed",
			"client", s.clientIP(r),
			"duration_ms", time.Since(started).Milliseconds(),
			"active_clients", remaining,
		)
		if remaining == 0 {
			go s.restorePrivacyWhenIdle()
		}
	}()

	runProxy(wsConn, vncConn)
}

func runProxy(wsConn *websocket.Conn, vncConn net.Conn) {
	var closeOnce sync.Once
	closeBoth := func() {
		closeOnce.Do(func() {
			_ = wsConn.Close()
			_ = vncConn.Close()
		})
	}
	defer closeBoth()

	go func() {
		defer closeBoth()
		for {
			messageType, payload, err := wsConn.ReadMessage()
			if err != nil {
				return
			}
			if messageType != websocket.BinaryMessage {
				continue
			}
			if _, err := vncConn.Write(payload); err != nil {
				return
			}
		}
	}()

	buffer := make([]byte, 32*1024)
	for {
		n, err := vncConn.Read(buffer)
		if n > 0 {
			_ = wsConn.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
			if writeErr := wsConn.WriteMessage(websocket.BinaryMessage, buffer[:n]); writeErr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}
