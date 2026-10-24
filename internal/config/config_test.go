package config

import (
	"os"
	"path/filepath"
	"testing"
)

func clearConfigEnv(t *testing.T) {
	t.Helper()
	names := []string{
		"LISTEN_ADDR", "VNC_ADDR", "REMOTE_PASSWORD", "REMOTE_PASSWORD_FILE",
		"SESSION_SECRET", "SESSION_SECRET_FILE", "SESSION_TTL", "TLS_CERT_FILE",
		"TLS_KEY_FILE", "TRUST_PROXY", "COOKIE_SECURE", "MAX_CLIENTS", "LOG_LEVEL",
		"WAYVNC_MANAGED", "WAYVNC_BIN", "WAYVNCCTL_BIN", "WAYVNC_CONTROL_SOCKET",
		"WAYVNC_OUTPUT", "WAYVNC_MAX_FPS", "WAYVNC_SHOW_CURSOR", "WAYVNC_DISABLE_INPUT",
		"WAYVNC_LOG_LEVEL",
	}
	for _, name := range names {
		t.Setenv(name, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("REMOTE_PASSWORD", "long-enough-password")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ListenAddr != ":8080" {
		t.Fatalf("unexpected listen address: %s", cfg.ListenAddr)
	}
	if cfg.VNCAddress != "127.0.0.1:5900" {
		t.Fatalf("unexpected VNC address: %s", cfg.VNCAddress)
	}
	if !cfg.Wayvnc.Managed || cfg.Wayvnc.MaxFPS != 30 {
		t.Fatalf("unexpected wayvnc defaults: %+v", cfg.Wayvnc)
	}
	if !cfg.SessionSecretGenerated || len(cfg.SessionSecret) != 32 {
		t.Fatal("expected an ephemeral 32-byte session secret")
	}
	if cfg.SessionTTL == 0 {
		t.Fatal("expected a session TTL")
	}
}

func TestLoadRequiresStrongPassword(t *testing.T) {
	clearConfigEnv(t)
	if _, err := Load(); err == nil {
		t.Fatal("expected missing password to fail")
	}

	t.Setenv("REMOTE_PASSWORD", "short")
	if _, err := Load(); err == nil {
		t.Fatal("expected short password to fail")
	}
}

func TestLoadPasswordAndSecretFromFiles(t *testing.T) {
	clearConfigEnv(t)
	dir := t.TempDir()
	passwordFile := filepath.Join(dir, "password")
	secretFile := filepath.Join(dir, "secret")
	if err := os.WriteFile(passwordFile, []byte("from-a-secret-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secretFile, []byte("0123456789abcdef0123456789abcdef\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REMOTE_PASSWORD_FILE", passwordFile)
	t.Setenv("SESSION_SECRET_FILE", secretFile)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Password != "from-a-secret-file" {
		t.Fatalf("unexpected password: %q", cfg.Password)
	}
	if cfg.SessionSecretGenerated {
		t.Fatal("did not expect a generated secret")
	}
}

func TestLoadValidation(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("REMOTE_PASSWORD", "long-enough-password")
	t.Setenv("TLS_CERT_FILE", "/tmp/cert.pem")
	if _, err := Load(); err == nil {
		t.Fatal("expected an unpaired TLS file to fail")
	}

	clearConfigEnv(t)
	t.Setenv("REMOTE_PASSWORD", "long-enough-password")
	t.Setenv("WAYVNC_OUTPUT", "bad output;rm")
	if _, err := Load(); err == nil {
		t.Fatal("expected an invalid output name to fail")
	}

	clearConfigEnv(t)
	t.Setenv("REMOTE_PASSWORD", "long-enough-password")
	t.Setenv("WAYVNC_MAX_FPS", "500")
	if _, err := Load(); err == nil {
		t.Fatal("expected an invalid FPS limit to fail")
	}

	clearConfigEnv(t)
	t.Setenv("REMOTE_PASSWORD", "long-enough-password")
	t.Setenv("COOKIE_SECURE", "sometimes")
	if _, err := Load(); err == nil {
		t.Fatal("expected an invalid cookie mode to fail")
	}
}
