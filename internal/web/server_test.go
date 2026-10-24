package web

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gorilla/websocket"
	"github.com/zilorn/niridesk/internal/config"
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
	}
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
