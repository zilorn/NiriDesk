package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zilorn/niridesk/internal/auth"
	"github.com/zilorn/niridesk/internal/config"
	"github.com/zilorn/niridesk/internal/powerlock"
	"github.com/zilorn/niridesk/internal/privacy"
	"github.com/zilorn/niridesk/internal/wayvnc"
	webassets "github.com/zilorn/niridesk/web"
)

const (
	sessionCookie = "niridesk_session"

	// A short grace period lets a page refresh or a network handoff reconnect
	// before the physical screen is locked.
	defaultPowerLockDelay = 5 * time.Second
	powerLockTimeout      = 5 * time.Second
)

// BuildInfo describes the running niridesk build.
type BuildInfo struct {
	Version string
	Commit  string
}

// WayvncController is the supervisor surface used by the HTTP layer.
type WayvncController interface {
	State(context.Context) wayvnc.State
	Outputs(context.Context) ([]wayvnc.Output, error)
	SetOutput(context.Context, string) error
}

// PrivacyController is the compositor surface used by the anti-peek mode.
type PrivacyController interface {
	State() privacy.State
	Enable(context.Context) error
	Disable(context.Context) error
	Close()
}

// PowerLockController is the host surface used to lock the screen after the
// last remote viewer disconnects.
type PowerLockController interface {
	State() powerlock.State
	SetEnabled(bool) error
	Lock(context.Context) error
}

// Server implements the authenticated HTTP and WebSocket API.
type Server struct {
	cfg       config.Config
	auth      *auth.Manager
	limiter   *auth.Limiter
	wayvnc    WayvncController
	privacy   PrivacyController
	powerLock PowerLockController
	build     BuildInfo
	logger    *slog.Logger

	staticFS       fs.FS
	mux            *http.ServeMux
	clientSlots    chan struct{}
	audioSlots     chan struct{}
	startedAt      time.Time
	activeClient   atomic.Int64
	audioActive    atomic.Int64
	audioErrMu     sync.RWMutex
	audioLastError string

	powerLockMu         sync.Mutex
	powerLockTimer      *time.Timer
	powerLockGeneration uint64
	powerLockDelay      time.Duration
}

// New builds the HTTP handler with the embedded frontend build.
func New(cfg config.Config, controller WayvncController, build BuildInfo, logger *slog.Logger) *Server {
	staticFS, err := webassets.DistFS()
	if err != nil {
		panic(fmt.Sprintf("open embedded web assets: %v", err))
	}
	return newServer(cfg, controller, build, logger, staticFS)
}

// newServer builds the HTTP handler with an injected asset filesystem so tests
// do not depend on the generated frontend build.
func newServer(cfg config.Config, controller WayvncController, build BuildInfo, logger *slog.Logger, staticFS fs.FS) *Server {
	s := &Server{
		cfg:            cfg,
		auth:           auth.New(cfg.Password, cfg.SessionSecret, cfg.SessionTTL),
		limiter:        auth.NewLimiter(5, 15*time.Minute, 5*time.Minute),
		wayvnc:         controller,
		privacy:        privacy.NewController(logger),
		powerLock:      powerlock.NewController(logger),
		build:          build,
		logger:         logger,
		staticFS:       staticFS,
		startedAt:      time.Now(),
		clientSlots:    make(chan struct{}, cfg.MaxClients),
		audioSlots:     make(chan struct{}, cfg.MaxClients),
		powerLockDelay: defaultPowerLockDelay,
	}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("POST /api/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/logout", s.handleLogout)
	s.mux.HandleFunc("GET /api/session", s.handleSession)
	s.mux.HandleFunc("GET /api/status", s.handleStatus)
	s.mux.HandleFunc("POST /api/output", s.handleOutput)
	s.mux.HandleFunc("POST /api/privacy", s.handlePrivacy)
	s.mux.HandleFunc("POST /api/power-lock", s.handlePowerLock)
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /websocket", s.handleWebSocket)
	s.mux.HandleFunc("GET /audio", s.handleAudioWebSocket)
	s.mux.HandleFunc("/", s.handleStatic)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if recovered := recover(); recovered != nil {
			s.logger.Error("panic serving request", "error", recovered, "stack", string(debug.Stack()))
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
	}()

	secure := s.requestIsHTTPS(r)
	setSecurityHeaders(w, secure)

	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Cache-Control", "no-store")
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		s.mux.ServeHTTP(recorder, r)
		level := slog.LevelDebug
		switch r.URL.Path {
		case "/api/login", "/api/logout", "/api/output":
			level = slog.LevelInfo
		}
		if recorder.status >= http.StatusInternalServerError {
			level = slog.LevelError
		} else if recorder.status >= http.StatusBadRequest && level < slog.LevelWarn {
			level = slog.LevelWarn
		}
		s.logger.Log(r.Context(), level, "api request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"duration_ms", time.Since(started).Milliseconds(),
			"client", s.clientIP(r),
		)
		return
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": s.build.Version,
	})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	authenticated := s.authenticated(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": authenticated,
		"version":       s.build.Version,
	})
}

type loginRequest struct {
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.originAllowed(r, false) {
		writeError(w, http.StatusForbidden, "origin_not_allowed")
		return
	}
	ip := s.clientIP(r)
	if allowed, retry := s.limiter.Allowed(ip); !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "too_many_attempts")
		return
	}

	var request loginRequest
	if err := decodeJSON(w, r, &request, 8<<10); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if !s.auth.CheckPassword(request.Password) {
		s.limiter.Failure(ip)
		s.logger.Warn("login rejected", "client", ip)
		writeError(w, http.StatusUnauthorized, "invalid_password")
		return
	}

	s.limiter.Success(ip)
	token, expires, err := s.auth.Issue()
	if err != nil {
		s.logger.Error("issue session", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	s.setSessionCookie(w, r, token, expires)
	s.logger.Info("login accepted", "client", ip)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if !s.originAllowed(r, false) {
		writeError(w, http.StatusForbidden, "origin_not_allowed")
		return
	}
	s.clearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	vncReachable := probeTCP(ctx, s.cfg.VNCAddress)
	managerState := s.wayvnc.State(ctx)
	outputs, outputErr := s.wayvnc.Outputs(ctx)
	if outputs == nil {
		outputs = []wayvnc.Output{}
	}

	display, socketPresent := waylandSocket()
	response := statusResponse{
		Version:       s.build.Version,
		Commit:        s.build.Commit,
		UptimeSeconds: int64(time.Since(s.startedAt).Seconds()),
		VNC: vncStatus{
			Reachable: vncReachable,
			Wayvnc:    managerState,
		},
		Wayland: waylandStatus{
			Display:       display,
			SocketPresent: socketPresent,
		},
		Outputs:       outputs,
		ActiveClients: int(s.activeClient.Load()),
		MaxClients:    s.cfg.MaxClients,
		MaxFPS:        s.cfg.Wayvnc.MaxFPS,
		Audio:         s.audioStatus(),
		Privacy:       s.privacy.State(),
		PowerLock:     s.powerLock.State(),
	}
	if outputErr != nil {
		response.OutputError = outputErr.Error()
	}
	writeJSON(w, http.StatusOK, response)
}

type statusResponse struct {
	Version       string          `json:"version"`
	Commit        string          `json:"commit"`
	UptimeSeconds int64           `json:"uptime_seconds"`
	VNC           vncStatus       `json:"vnc"`
	Wayland       waylandStatus   `json:"wayland"`
	Outputs       []wayvnc.Output `json:"outputs"`
	OutputError   string          `json:"output_error,omitempty"`
	ActiveClients int             `json:"active_clients"`
	MaxClients    int             `json:"max_clients"`
	MaxFPS        int             `json:"max_fps"`
	Audio         audioStatus     `json:"audio"`
	Privacy       privacy.State   `json:"privacy"`
	PowerLock     powerlock.State `json:"power_lock"`
}

type vncStatus struct {
	Reachable bool         `json:"reachable"`
	Wayvnc    wayvnc.State `json:"wayvnc"`
}

type waylandStatus struct {
	Display       string `json:"display"`
	SocketPresent bool   `json:"socket_present"`
}

type outputRequest struct {
	Name string `json:"name"`
}

func (s *Server) handleOutput(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	if !s.originAllowed(r, false) {
		writeError(w, http.StatusForbidden, "origin_not_allowed")
		return
	}
	var request outputRequest
	if err := decodeJSON(w, r, &request, 1024); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if !config.ValidOutputName(request.Name) {
		writeError(w, http.StatusBadRequest, "invalid_output")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := s.wayvnc.SetOutput(ctx, request.Name); err != nil {
		s.logger.Warn("switch output failed", "output", request.Name, "error", err)
		writeError(w, http.StatusBadGateway, "output_switch_failed")
		return
	}
	outputs, err := s.wayvnc.Outputs(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, "output_refresh_failed")
		return
	}
	s.logger.Info("capture output changed", "output", request.Name, "client", s.clientIP(r))
	writeJSON(w, http.StatusOK, map[string]any{"outputs": outputs})
}

type privacyRequest struct {
	Enabled bool `json:"enabled"`
}

func (s *Server) handlePrivacy(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	if !s.originAllowed(r, false) {
		writeError(w, http.StatusForbidden, "origin_not_allowed")
		return
	}
	var request privacyRequest
	if err := decodeJSON(w, r, &request, 1024); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var err error
	if request.Enabled {
		err = s.privacy.Enable(ctx)
	} else {
		err = s.privacy.Disable(ctx)
	}
	if err != nil {
		s.logger.Warn("anti-peek mode change failed", "enabled", request.Enabled, "error", err)
		writeError(w, http.StatusBadGateway, "privacy_unavailable")
		return
	}
	state := s.privacy.State()
	s.logger.Info("anti-peek mode changed", "enabled", state.Enabled, "client", s.clientIP(r))
	writeJSON(w, http.StatusOK, state)
}

type powerLockRequest struct {
	Enabled bool `json:"enabled"`
}

func (s *Server) handlePowerLock(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	if !s.originAllowed(r, false) {
		writeError(w, http.StatusForbidden, "origin_not_allowed")
		return
	}
	var request powerLockRequest
	if err := decodeJSON(w, r, &request, 1024); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	if err := s.powerLock.SetEnabled(request.Enabled); err != nil {
		s.logger.Warn("power lock preference change failed", "enabled", request.Enabled, "error", err)
		writeError(w, http.StatusBadGateway, "power_lock_unavailable")
		return
	}
	if !request.Enabled {
		s.cancelPendingPowerLock()
	}
	state := s.powerLock.State()
	s.logger.Info("power lock preference changed", "enabled", state.Enabled, "client", s.clientIP(r))
	writeJSON(w, http.StatusOK, state)
}

// addClient registers a remote canvas session and cancels a pending Power Lock
// when the viewer reconnects within the grace period.
func (s *Server) addClient() int64 {
	s.powerLockMu.Lock()
	defer s.powerLockMu.Unlock()

	s.powerLockGeneration++
	if s.powerLockTimer != nil {
		s.powerLockTimer.Stop()
		s.powerLockTimer = nil
	}
	return s.activeClient.Add(1)
}

// removeClient unregisters a remote canvas session and schedules Power Lock
// when the last viewer leaves.
func (s *Server) removeClient() int64 {
	s.powerLockMu.Lock()
	defer s.powerLockMu.Unlock()

	remaining := s.activeClient.Add(-1)
	if remaining == 0 {
		s.schedulePowerLockLocked()
	}
	return remaining
}

func (s *Server) schedulePowerLockLocked() {
	if s.powerLock == nil || !s.powerLock.State().Enabled {
		return
	}
	if s.powerLockTimer != nil {
		s.powerLockTimer.Stop()
	}
	generation := s.powerLockGeneration
	delay := s.powerLockDelay
	s.powerLockTimer = time.AfterFunc(delay, func() {
		s.runPowerLock(generation)
	})
	s.logger.Info("power lock armed after the last remote session", "delay", delay)
}

func (s *Server) runPowerLock(generation uint64) {
	s.powerLockMu.Lock()
	if generation != s.powerLockGeneration {
		s.powerLockMu.Unlock()
		return
	}
	s.powerLockTimer = nil
	if s.activeClient.Load() != 0 {
		s.powerLockMu.Unlock()
		return
	}
	enabled := s.powerLock != nil && s.powerLock.State().Enabled
	s.powerLockMu.Unlock()
	if !enabled {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), powerLockTimeout)
	defer cancel()
	if err := s.powerLock.Lock(ctx); err != nil {
		s.logger.Warn("power lock failed", "error", err)
	}
}

func (s *Server) cancelPendingPowerLock() {
	s.powerLockMu.Lock()
	defer s.powerLockMu.Unlock()

	s.powerLockGeneration++
	if s.powerLockTimer != nil {
		s.powerLockTimer.Stop()
		s.powerLockTimer = nil
	}
}

// Close restores physical monitors when the process shuts down. Power Lock is
// intentionally not armed here: stopping the service is not a remote session
// ending, and a restart should not lock the desktop.
func (s *Server) Close() {
	s.cancelPendingPowerLock()
	if s.privacy != nil {
		s.privacy.Close()
	}
}

// restorePrivacyWhenIdle restores monitors after the last remote canvas closes.
// Anti-peek is a session-scoped control; leaving physical monitors dark with no
// viewer would be surprising and hard to undo without local input.
func (s *Server) restorePrivacyWhenIdle() {
	if s.activeClient.Load() != 0 {
		return
	}
	if !s.privacy.State().Enabled {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.privacy.Disable(ctx); err != nil {
		s.logger.Warn("failed to restore monitors after last session", "error", err)
	}
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clean := path.Clean("/" + r.URL.Path)
	relative := strings.TrimPrefix(clean, "/")
	if relative == "" || relative == "." {
		relative = "index.html"
	}
	if r.URL.Path != "/" && strings.HasSuffix(r.URL.Path, "/") {
		http.NotFound(w, r)
		return
	}

	info, err := fs.Stat(s.staticFS, relative)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	switch {
	case relative == "index.html", relative == "sw.js", relative == "manifest.webmanifest":
		w.Header().Set("Cache-Control", "no-cache")
	case strings.HasPrefix(relative, "assets/"):
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	case strings.HasPrefix(relative, "icons/"):
		w.Header().Set("Cache-Control", "public, max-age=86400")
	default:
		w.Header().Set("Cache-Control", "no-cache")
	}
	extension := path.Ext(relative)
	contentType := mime.TypeByExtension(extension)
	switch extension {
	case ".webmanifest":
		contentType = "application/manifest+json"
	case ".js":
		contentType = "text/javascript; charset=utf-8"
	}
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if relative == "sw.js" {
		w.Header().Set("Service-Worker-Allowed", "/")
	}

	http.ServeFileFS(w, r, s.staticFS, relative)
}

func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	if s.authenticated(r) {
		return true
	}
	writeError(w, http.StatusUnauthorized, "authentication_required")
	return false
}

func (s *Server) authenticated(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	return s.auth.Validate(cookie.Value)
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.requestIsHTTPS(r),
		SameSite: http.SameSiteStrictMode,
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.requestIsHTTPS(r),
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Unix(1, 0),
		MaxAge:   -1,
	})
}

func (s *Server) requestIsHTTPS(r *http.Request) bool {
	switch s.cfg.CookieSecure {
	case "true":
		return true
	case "false":
		return false
	}
	if r.TLS != nil {
		return true
	}
	return s.cfg.TrustProxy && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			first := strings.TrimSpace(strings.Split(forwarded, ",")[0])
			if first != "" && len(first) <= 128 {
				return first
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) originAllowed(r *http.Request, required bool) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return !required
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return strings.EqualFold(parsed.Host, r.Host)
}

func setSecurityHeaders(w http.ResponseWriter, secure bool) {
	w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self' ws: wss:; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; script-src 'self'; worker-src 'self'; manifest-src 'self'; font-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	if secure {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")
	}
}

func waylandSocket() (string, bool) {
	display := strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY"))
	if display == "" {
		return "", false
	}
	socketPath := display
	if !filepath.IsAbs(socketPath) {
		runtimeDir := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR"))
		if runtimeDir == "" {
			return display, false
		}
		socketPath = filepath.Join(runtimeDir, display)
	}
	info, err := os.Stat(socketPath)
	if err != nil {
		return display, false
	}
	return display, info.Mode()&os.ModeSocket != 0
}

func probeTCP(ctx context.Context, address string) bool {
	dialer := net.Dialer{Timeout: time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any, limit int64) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return errors.New("content type must be application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if value != nil {
		_ = json.NewEncoder(w).Encode(value)
	}
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
