package web

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gorilla/websocket"
	"github.com/zilorn/niridesk/internal/audio"
	"github.com/zilorn/niridesk/internal/config"
	"github.com/zilorn/niridesk/internal/powerlock"
	"github.com/zilorn/niridesk/internal/privacy"
	"github.com/zilorn/niridesk/internal/wayvnc"
)

type fakeController struct {
	mu      sync.Mutex
	outputs []wayvnc.Output
	setErr  error
}

func (f *fakeController) State(context.Context) wayvnc.State {
	return wayvnc.State{Managed: true, Running: true, Version: "0.9.1", NeatVNC: "0.9.1"}
}

func (f *fakeController) Outputs(context.Context) ([]wayvnc.Output, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]wayvnc.Output(nil), f.outputs...), nil
}

func (f *fakeController) SetOutput(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.setErr != nil {
		return f.setErr
	}
	for index := range f.outputs {
		f.outputs[index].Captured = f.outputs[index].Name == name
	}
	return nil
}

func testConfig(vncAddress string) config.Config {
	return config.Config{
		ListenAddr:    "127.0.0.1:0",
		VNCAddress:    vncAddress,
		Password:      "test-password",
		SessionSecret: []byte("0123456789abcdef0123456789abcdef"),
		SessionTTL:    time.Hour,
		CookieSecure:  "false",
		MaxClients:    2,
		Wayvnc: config.Wayvnc{
			Managed: true,
			MaxFPS:  30,
		},
		Audio: config.Audio{Enabled: false},
	}
}

type fakePrivacy struct {
	mu    sync.Mutex
	state privacy.State
}

func (f *fakePrivacy) State() privacy.State {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state
}

func (f *fakePrivacy) Enable(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state.Enabled = true
	f.state.LastError = ""
	return nil
}

func (f *fakePrivacy) Disable(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state.Enabled = false
	f.state.LastError = ""
	return nil
}

func (f *fakePrivacy) Close() {
	_ = f.Disable(context.Background())
}

func (f *fakePrivacy) enabled() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state.Enabled
}

type fakePowerLock struct {
	mu      sync.Mutex
	state   powerlock.State
	setErr  error
	lockErr error
	calls   int
}

func (f *fakePowerLock) State() powerlock.State {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state
}

func (f *fakePowerLock) SetEnabled(enabled bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if enabled && !f.state.Supported {
		f.state.LastError = "niri IPC is not available"
		return errors.New("niri IPC is not available")
	}
	if f.setErr != nil {
		f.state.LastError = f.setErr.Error()
		return f.setErr
	}
	f.state.Enabled = enabled
	f.state.LastError = ""
	return nil
}

func (f *fakePowerLock) Lock(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.state.Enabled {
		return nil
	}
	f.calls++
	return f.lockErr
}

func (f *fakePowerLock) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakePowerLock) enabled() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state.Enabled
}

func testHandler(t *testing.T, vncAddress string) (*Server, *fakeController) {
	t.Helper()
	controller := &fakeController{outputs: []wayvnc.Output{
		{Name: "DP-1", Description: "Test Display", Width: 1920, Height: 1080, Captured: true},
		{Name: "HDMI-A-1", Description: "Second Display", Width: 2560, Height: 1440},
	}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	assets := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<!doctype html><title>NiriDesk test</title>")},
	}
	server := newServer(testConfig(vncAddress), controller, BuildInfo{Version: "test", Commit: "abcdef1"}, logger, assets)
	server.privacy = &fakePrivacy{state: privacy.State{Supported: true}}
	server.powerLock = &fakePowerLock{state: powerlock.State{Supported: true}}
	server.powerLockDelay = 20 * time.Millisecond
	return server, controller
}

func loginClient(t *testing.T, ts *httptest.Server, password string) (*http.Client, *http.Response) {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	return client, postLogin(t, client, ts.URL, password)
}

func postLogin(t *testing.T, client *http.Client, baseURL, password string) *http.Response {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"password": password})
	response, err := client.Post(baseURL+"/api/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	return response
}

func TestStaticAndSecurityHeaders(t *testing.T) {
	server, _ := testHandler(t, "127.0.0.1:1")
	ts := httptest.NewServer(server)
	defer ts.Close()

	response, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", response.StatusCode)
	}
	if !strings.Contains(string(body), "NiriDesk") {
		t.Fatal("index page did not contain the product name")
	}
	if !strings.Contains(response.Header.Get("Content-Security-Policy"), "default-src 'self'") {
		t.Fatal("missing content security policy")
	}
	if response.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing nosniff header")
	}

	missing, err := http.Get(ts.URL + "/does-not-exist.js")
	if err != nil {
		t.Fatal(err)
	}
	defer missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("expected missing asset to return 404, got %d", missing.StatusCode)
	}
}

func TestLoginAndProtectedStatus(t *testing.T) {
	server, _ := testHandler(t, "127.0.0.1:1")
	ts := httptest.NewServer(server)
	defer ts.Close()

	unauthorized, err := http.Get(ts.URL + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthorized.StatusCode)
	}

	client, response := loginClient(t, ts, "test-password")
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("login failed: %d", response.StatusCode)
	}
	cookies := response.Cookies()
	if len(cookies) == 0 || cookies[0].Name != sessionCookie || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("unexpected session cookie: %+v", cookies)
	}

	statusHTTP, err := client.Get(ts.URL + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	defer statusHTTP.Body.Close()
	if statusHTTP.StatusCode != http.StatusOK {
		t.Fatalf("status failed: %d", statusHTTP.StatusCode)
	}
	var status statusResponse
	if err := json.NewDecoder(statusHTTP.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if len(status.Outputs) != 2 || status.Outputs[0].Name != "DP-1" {
		t.Fatalf("unexpected outputs: %+v", status.Outputs)
	}
	if status.MaxFPS != 30 || status.MaxClients != 2 {
		t.Fatalf("unexpected status config: %+v", status)
	}
}

func TestOutputSwitch(t *testing.T) {
	server, controller := testHandler(t, "127.0.0.1:1")
	ts := httptest.NewServer(server)
	defer ts.Close()

	client, response := loginClient(t, ts, "test-password")
	response.Body.Close()

	body := strings.NewReader(`{"name":"HDMI-A-1"}`)
	request, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/output", body)
	request.Header.Set("Content-Type", "application/json")
	result, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Body.Close()
	if result.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(result.Body)
		t.Fatalf("switch failed: %d %s", result.StatusCode, raw)
	}

	outputs, _ := controller.Outputs(context.Background())
	if outputs[1].Captured != true || outputs[0].Captured != false {
		t.Fatalf("capture did not move: %+v", outputs)
	}
}

func TestLoginRateLimit(t *testing.T) {
	server, _ := testHandler(t, "127.0.0.1:1")
	ts := httptest.NewServer(server)
	defer ts.Close()

	client, _ := loginClient(t, ts, "wrong-password")
	for i := 0; i < 4; i++ {
		response := postLogin(t, client, ts.URL, "wrong-password")
		response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+2, response.StatusCode)
		}
	}
	blocked := postLogin(t, client, ts.URL, "wrong-password")
	blocked.Body.Close()
	if blocked.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after failure budget, got %d", blocked.StatusCode)
	}

	valid := postLogin(t, client, ts.URL, "test-password")
	valid.Body.Close()
	if valid.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected block to apply to valid passwords too, got %d", valid.StatusCode)
	}
}

func TestWebSocketProxy(t *testing.T) {
	vncAddress := startFakeVNCServer(t)
	server, _ := testHandler(t, vncAddress)
	ts := httptest.NewServer(server)
	defer ts.Close()

	client, response := loginClient(t, ts, "test-password")
	response.Body.Close()
	cookies := client.Jar.Cookies(mustURL(t, ts.URL))
	cookieHeader := ""
	for index, cookie := range cookies {
		if index > 0 {
			cookieHeader += "; "
		}
		cookieHeader += cookie.Name + "=" + cookie.Value
	}

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/websocket"
	header := http.Header{}
	header.Set("Cookie", cookieHeader)
	header.Set("Origin", ts.URL)
	conn, upgradeResponse, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		if upgradeResponse != nil {
			raw, _ := io.ReadAll(upgradeResponse.Body)
			t.Fatalf("websocket dial: %v (%d %s)", err, upgradeResponse.StatusCode, raw)
		}
		t.Fatalf("websocket dial: %v", err)
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	messageType, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if messageType != websocket.BinaryMessage || string(payload) != "RFB 003.008\n" {
		t.Fatalf("unexpected banner: %d %q", messageType, payload)
	}

	if err := conn.WriteMessage(websocket.BinaryMessage, []byte("ping")); err != nil {
		t.Fatal(err)
	}
	messageType, payload, err = conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if messageType != websocket.BinaryMessage || string(payload) != "ping" {
		t.Fatalf("unexpected echo: %d %q", messageType, payload)
	}
}

func TestWebSocketRequiresAuthAndOrigin(t *testing.T) {
	server, _ := testHandler(t, "127.0.0.1:1")
	ts := httptest.NewServer(server)
	defer ts.Close()
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/websocket"

	conn, response, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		conn.Close()
		t.Fatal("expected unauthenticated websocket to fail")
	}
	if response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %+v", response)
	}

	client, login := loginClient(t, ts, "test-password")
	login.Body.Close()
	cookies := client.Jar.Cookies(mustURL(t, ts.URL))
	header := http.Header{}
	header.Set("Cookie", cookies[0].Name+"="+cookies[0].Value)
	conn, response, err = websocket.DefaultDialer.Dial(wsURL, header)
	if err == nil {
		conn.Close()
		t.Fatal("expected websocket without Origin to fail")
	}
	if response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %+v", response)
	}
}

func TestAudioWebSocketStreamsFrames(t *testing.T) {
	server, _ := testHandler(t, "127.0.0.1:1")
	capture := filepath.Join(t.TempDir(), "parec")
	if err := os.WriteFile(capture, []byte("#!/bin/sh\nhead -c 7680 /dev/zero\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	server.cfg.Audio = config.Audio{Enabled: true, Binary: capture, Device: "@DEFAULT_MONITOR@"}
	ts := httptest.NewServer(server)
	defer ts.Close()

	client, response := loginClient(t, ts, "test-password")
	response.Body.Close()
	cookies := client.Jar.Cookies(mustURL(t, ts.URL))
	header := http.Header{}
	header.Set("Cookie", cookies[0].Name+"="+cookies[0].Value)
	header.Set("Origin", ts.URL)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/audio"
	conn, upgradeResponse, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		if upgradeResponse != nil {
			raw, _ := io.ReadAll(upgradeResponse.Body)
			t.Fatalf("audio dial: %v (%d %s)", err, upgradeResponse.StatusCode, raw)
		}
		t.Fatalf("audio dial: %v", err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	messageType, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if messageType != websocket.BinaryMessage {
		t.Fatalf("unexpected audio message type: %d", messageType)
	}
	if len(payload) != audioFrameHeaderSize+audio.FrameBytes {
		t.Fatalf("unexpected audio frame length: %d", len(payload))
	}
	if string(payload[:4]) != string(audioFrameMagic[:]) || payload[4] != audioProtocolVersion || payload[5] != audioFormatS16LE {
		t.Fatalf("unexpected audio header: %x", payload[:6])
	}
	if binary.LittleEndian.Uint32(payload[8:12]) != audio.SampleRate || binary.LittleEndian.Uint16(payload[6:8]) != audio.Channels {
		t.Fatal("unexpected audio format")
	}
	if binary.LittleEndian.Uint64(payload[12:20]) == 0 {
		t.Fatal("expected a capture timestamp")
	}
}

func TestAudioWebSocketRequiresAuthAndOrigin(t *testing.T) {
	server, _ := testHandler(t, "127.0.0.1:1")
	server.cfg.Audio = config.Audio{Enabled: true, Binary: "/bin/true", Device: "@DEFAULT_MONITOR@"}
	ts := httptest.NewServer(server)
	defer ts.Close()
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/audio"

	conn, response, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		conn.Close()
		t.Fatal("expected unauthenticated audio websocket to fail")
	}
	if response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %+v", response)
	}

	client, login := loginClient(t, ts, "test-password")
	login.Body.Close()
	cookies := client.Jar.Cookies(mustURL(t, ts.URL))
	header := http.Header{}
	header.Set("Cookie", cookies[0].Name+"="+cookies[0].Value)
	conn, response, err = websocket.DefaultDialer.Dial(wsURL, header)
	if err == nil {
		conn.Close()
		t.Fatal("expected audio websocket without Origin to fail")
	}
	if response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %+v", response)
	}
}

func TestPrivacyModeToggleAndAutoRestore(t *testing.T) {
	vncAddress := startFakeVNCServer(t)
	server, _ := testHandler(t, vncAddress)
	privacy := server.privacy.(*fakePrivacy)
	ts := httptest.NewServer(server)
	defer ts.Close()

	client, response := loginClient(t, ts, "test-password")
	response.Body.Close()
	if privacy.enabled() {
		t.Fatal("anti-peek should start disabled")
	}

	body := strings.NewReader(`{"enabled":true}`)
	request, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/privacy", body)
	request.Header.Set("Content-Type", "application/json")
	result, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Body.Close()
	if result.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(result.Body)
		t.Fatalf("privacy enable failed: %d %s", result.StatusCode, raw)
	}
	if !privacy.enabled() {
		t.Fatal("expected anti-peek mode to be enabled")
	}

	cookies := client.Jar.Cookies(mustURL(t, ts.URL))
	header := http.Header{}
	header.Set("Cookie", cookies[0].Name+"="+cookies[0].Value)
	header.Set("Origin", ts.URL)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/websocket"
	conn, upgradeResponse, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		if upgradeResponse != nil {
			raw, _ := io.ReadAll(upgradeResponse.Body)
			t.Fatalf("websocket dial: %v (%d %s)", err, upgradeResponse.StatusCode, raw)
		}
		t.Fatalf("websocket dial: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()

	deadline := time.Now().Add(2 * time.Second)
	for privacy.enabled() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if privacy.enabled() {
		t.Fatal("expected anti-peek mode to restore after the last session")
	}
}

func TestPowerLockToggleAndStatus(t *testing.T) {
	server, _ := testHandler(t, "127.0.0.1:1")
	powerLock := server.powerLock.(*fakePowerLock)
	ts := httptest.NewServer(server)
	defer ts.Close()

	unauthorized, err := http.Post(ts.URL+"/api/power-lock", "application/json", strings.NewReader(`{"enabled":true}`))
	if err != nil {
		t.Fatal(err)
	}
	unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthorized.StatusCode)
	}

	client, response := loginClient(t, ts, "test-password")
	response.Body.Close()

	statusHTTP, err := client.Get(ts.URL + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	var status statusResponse
	if err := json.NewDecoder(statusHTTP.Body).Decode(&status); err != nil {
		statusHTTP.Body.Close()
		t.Fatal(err)
	}
	statusHTTP.Body.Close()
	if !status.PowerLock.Supported || status.PowerLock.Enabled {
		t.Fatalf("unexpected initial power lock state: %+v", status.PowerLock)
	}

	setPowerLock(t, client, ts.URL, true)
	if !powerLock.enabled() {
		t.Fatal("expected Power Lock to be enabled")
	}

	setPowerLock(t, client, ts.URL, false)
	if powerLock.enabled() {
		t.Fatal("expected Power Lock to be disabled")
	}
}

func TestPowerLockAfterLastSession(t *testing.T) {
	vncAddress := startFakeVNCServer(t)
	server, _ := testHandler(t, vncAddress)
	powerLock := server.powerLock.(*fakePowerLock)
	ts := httptest.NewServer(server)
	defer ts.Close()

	client, response := loginClient(t, ts, "test-password")
	response.Body.Close()
	setPowerLock(t, client, ts.URL, true)

	conn := dialRemoteWebSocket(t, ts, client)
	_ = conn.Close()
	waitFor(t, 2*time.Second, func() bool { return powerLock.callCount() == 1 })

	time.Sleep(60 * time.Millisecond)
	if calls := powerLock.callCount(); calls != 1 {
		t.Fatalf("expected exactly one lock call, got %d", calls)
	}
}

func TestPowerLockReconnectCancelsTimer(t *testing.T) {
	vncAddress := startFakeVNCServer(t)
	server, _ := testHandler(t, vncAddress)
	server.powerLockDelay = 300 * time.Millisecond
	powerLock := server.powerLock.(*fakePowerLock)
	ts := httptest.NewServer(server)
	defer ts.Close()

	client, response := loginClient(t, ts, "test-password")
	response.Body.Close()
	setPowerLock(t, client, ts.URL, true)

	first := dialRemoteWebSocket(t, ts, client)
	_ = first.Close()
	waitFor(t, time.Second, func() bool { return server.activeClient.Load() == 0 })

	second := dialRemoteWebSocket(t, ts, client)
	time.Sleep(450 * time.Millisecond)
	if calls := powerLock.callCount(); calls != 0 {
		t.Fatalf("reconnect should cancel Power Lock, got %d calls", calls)
	}

	_ = second.Close()
	waitFor(t, 2*time.Second, func() bool { return powerLock.callCount() == 1 })
}

func TestPowerLockDisabledDoesNotLock(t *testing.T) {
	vncAddress := startFakeVNCServer(t)
	server, _ := testHandler(t, vncAddress)
	powerLock := server.powerLock.(*fakePowerLock)
	ts := httptest.NewServer(server)
	defer ts.Close()

	client, response := loginClient(t, ts, "test-password")
	response.Body.Close()

	conn := dialRemoteWebSocket(t, ts, client)
	_ = conn.Close()
	waitFor(t, time.Second, func() bool { return server.activeClient.Load() == 0 })
	time.Sleep(80 * time.Millisecond)
	if calls := powerLock.callCount(); calls != 0 {
		t.Fatalf("disabled Power Lock should not lock, got %d calls", calls)
	}
}

func TestPowerLockDisableCancelsTimer(t *testing.T) {
	vncAddress := startFakeVNCServer(t)
	server, _ := testHandler(t, vncAddress)
	server.powerLockDelay = 200 * time.Millisecond
	powerLock := server.powerLock.(*fakePowerLock)
	ts := httptest.NewServer(server)
	defer ts.Close()

	client, response := loginClient(t, ts, "test-password")
	response.Body.Close()
	setPowerLock(t, client, ts.URL, true)

	conn := dialRemoteWebSocket(t, ts, client)
	_ = conn.Close()
	waitFor(t, time.Second, func() bool { return server.activeClient.Load() == 0 })

	setPowerLock(t, client, ts.URL, false)
	time.Sleep(300 * time.Millisecond)
	if calls := powerLock.callCount(); calls != 0 {
		t.Fatalf("disabling Power Lock should cancel the timer, got %d calls", calls)
	}
}

func TestPowerLockUnsupported(t *testing.T) {
	server, _ := testHandler(t, "127.0.0.1:1")
	server.powerLock.(*fakePowerLock).state.Supported = false
	ts := httptest.NewServer(server)
	defer ts.Close()

	client, response := loginClient(t, ts, "test-password")
	response.Body.Close()

	body := strings.NewReader(`{"enabled":true}`)
	request, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/power-lock", body)
	request.Header.Set("Content-Type", "application/json")
	result, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Body.Close()
	if result.StatusCode != http.StatusBadGateway {
		raw, _ := io.ReadAll(result.Body)
		t.Fatalf("expected 502, got %d %s", result.StatusCode, raw)
	}
}

func setPowerLock(t *testing.T, client *http.Client, baseURL string, enabled bool) {
	t.Helper()
	payload := `{"enabled":false}`
	if enabled {
		payload = `{"enabled":true}`
	}
	request, _ := http.NewRequest(http.MethodPost, baseURL+"/api/power-lock", strings.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(response.Body)
		t.Fatalf("power lock update failed: %d %s", response.StatusCode, raw)
	}
}

func dialRemoteWebSocket(t *testing.T, ts *httptest.Server, client *http.Client) *websocket.Conn {
	t.Helper()
	cookies := client.Jar.Cookies(mustURL(t, ts.URL))
	if len(cookies) == 0 {
		t.Fatal("missing session cookie")
	}
	header := http.Header{}
	header.Set("Cookie", cookies[0].Name+"="+cookies[0].Value)
	header.Set("Origin", ts.URL)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/websocket"
	conn, upgradeResponse, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		if upgradeResponse != nil {
			raw, _ := io.ReadAll(upgradeResponse.Body)
			t.Fatalf("websocket dial: %v (%d %s)", err, upgradeResponse.StatusCode, raw)
		}
		t.Fatalf("websocket dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatal(err)
	}
	return conn
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition was not met before the deadline")
}

func startFakeVNCServer(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = conn.Write([]byte("RFB 003.008\n"))
				buffer := make([]byte, 4096)
				for {
					n, err := conn.Read(buffer)
					if n > 0 {
						_, _ = conn.Write(buffer[:n])
					}
					if err != nil {
						return
					}
				}
			}()
		}
	}()
	return listener.Addr().String()
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
