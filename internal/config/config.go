package config

import (
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultListenAddr    = ":8080"
	defaultVNCAddress    = "127.0.0.1:5900"
	defaultSessionTTL    = 12 * time.Hour
	defaultControlSocket = "/tmp/niridesk-wayvnc.sock"
	defaultAudioDevice   = "@DEFAULT_MONITOR@"
)

// Audio contains the settings used to capture the host's system audio.
type Audio struct {
	Enabled bool
	Binary  string
	Device  string
}

// Wayvnc contains the settings used to supervise the local wayvnc process.
type Wayvnc struct {
	Managed       bool
	Binary        string
	ControlBinary string
	ControlSocket string
	Output        string
	MaxFPS        int
	ShowCursor    bool
	DisableInput  bool
	LogLevel      string
}

// Config is the complete runtime configuration for niridesk.
type Config struct {
	ListenAddr             string
	VNCAddress             string
	Password               string
	SessionSecret          []byte
	SessionSecretGenerated bool
	SessionTTL             time.Duration
	TLSCertFile            string
	TLSKeyFile             string
	TrustProxy             bool
	CookieSecure           string
	MaxClients             int
	LogLevel               slog.Level
	Wayvnc                 Wayvnc
	Audio                  Audio
}

// Load reads configuration from the environment and validates it.
func Load() (Config, error) {
	cfg := Config{
		ListenAddr:   envOrDefault("LISTEN_ADDR", defaultListenAddr),
		VNCAddress:   envOrDefault("VNC_ADDR", defaultVNCAddress),
		CookieSecure: strings.ToLower(envOrDefault("COOKIE_SECURE", "auto")),
		MaxClients:   4,
		LogLevel:     slog.LevelInfo,
		Audio: Audio{
			Enabled: true,
			Binary:  envOrDefault("AUDIO_BINARY", "parec"),
			Device:  envOrDefault("AUDIO_DEVICE", defaultAudioDevice),
		},
		Wayvnc: Wayvnc{
			Managed:       true,
			Binary:        envOrDefault("WAYVNC_BIN", "wayvnc"),
			ControlBinary: envOrDefault("WAYVNCCTL_BIN", "wayvncctl"),
			ControlSocket: envOrDefault("WAYVNC_CONTROL_SOCKET", defaultControlSocket),
			Output:        strings.TrimSpace(os.Getenv("WAYVNC_OUTPUT")),
			MaxFPS:        30,
			ShowCursor:    true,
			LogLevel:      envOrDefault("WAYVNC_LOG_LEVEL", "info"),
		},
	}

	password, err := readSecret("REMOTE_PASSWORD")
	if err != nil {
		return Config{}, err
	}
	if password == "" {
		return Config{}, errors.New("REMOTE_PASSWORD or REMOTE_PASSWORD_FILE is required")
	}
	if len(password) < 8 {
		return Config{}, errors.New("REMOTE_PASSWORD must be at least 8 characters")
	}
	cfg.Password = password

	secret, generated, err := loadSessionSecret()
	if err != nil {
		return Config{}, err
	}
	cfg.SessionSecret = secret
	cfg.SessionSecretGenerated = generated

	if raw := os.Getenv("SESSION_TTL"); raw != "" {
		ttl, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("SESSION_TTL: %w", err)
		}
		if ttl < 5*time.Minute || ttl > 30*24*time.Hour {
			return Config{}, errors.New("SESSION_TTL must be between 5m and 720h")
		}
		cfg.SessionTTL = ttl
	} else {
		cfg.SessionTTL = defaultSessionTTL
	}

	if raw := os.Getenv("MAX_CLIENTS"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 32 {
			return Config{}, errors.New("MAX_CLIENTS must be an integer between 1 and 32")
		}
		cfg.MaxClients = value
	}

	if raw := os.Getenv("TRUST_PROXY"); raw != "" {
		value, err := parseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("TRUST_PROXY: %w", err)
		}
		cfg.TrustProxy = value
	}

	switch cfg.CookieSecure {
	case "auto", "true", "false":
	default:
		return Config{}, errors.New("COOKIE_SECURE must be auto, true, or false")
	}

	if raw := os.Getenv("LOG_LEVEL"); raw != "" {
		level, err := parseLogLevel(raw)
		if err != nil {
			return Config{}, err
		}
		cfg.LogLevel = level
	}

	cfg.TLSCertFile = strings.TrimSpace(os.Getenv("TLS_CERT_FILE"))
	cfg.TLSKeyFile = strings.TrimSpace(os.Getenv("TLS_KEY_FILE"))
	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		return Config{}, errors.New("TLS_CERT_FILE and TLS_KEY_FILE must be set together")
	}

	if raw := os.Getenv("AUDIO_ENABLED"); raw != "" {
		value, err := parseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("AUDIO_ENABLED: %w", err)
		}
		cfg.Audio.Enabled = value
	}
	if cfg.Audio.Enabled {
		if cfg.Audio.Binary == "" || cfg.Audio.Device == "" {
			return Config{}, errors.New("AUDIO_BINARY and AUDIO_DEVICE must not be empty")
		}
		if len(cfg.Audio.Device) > 256 || strings.ContainsRune(cfg.Audio.Device, '\x00') {
			return Config{}, errors.New("AUDIO_DEVICE is invalid")
		}
	}
	if raw := os.Getenv("WAYVNC_MANAGED"); raw != "" {
		value, err := parseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("WAYVNC_MANAGED: %w", err)
		}
		cfg.Wayvnc.Managed = value
	}
	if raw := os.Getenv("WAYVNC_MAX_FPS"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 120 {
			return Config{}, errors.New("WAYVNC_MAX_FPS must be an integer between 1 and 120")
		}
		cfg.Wayvnc.MaxFPS = value
	}
	if raw := os.Getenv("WAYVNC_SHOW_CURSOR"); raw != "" {
		value, err := parseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("WAYVNC_SHOW_CURSOR: %w", err)
		}
		cfg.Wayvnc.ShowCursor = value
	}
	if raw := os.Getenv("WAYVNC_DISABLE_INPUT"); raw != "" {
		value, err := parseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("WAYVNC_DISABLE_INPUT: %w", err)
		}
		cfg.Wayvnc.DisableInput = value
	}
	if cfg.Wayvnc.Output != "" && !ValidOutputName(cfg.Wayvnc.Output) {
		return Config{}, errors.New("WAYVNC_OUTPUT contains unsupported characters")
	}
	if cfg.Wayvnc.ControlSocket == "" || !strings.HasPrefix(cfg.Wayvnc.ControlSocket, "/") {
		return Config{}, errors.New("WAYVNC_CONTROL_SOCKET must be an absolute path")
	}
	if cfg.Wayvnc.Binary == "" || cfg.Wayvnc.ControlBinary == "" {
		return Config{}, errors.New("WAYVNC_BIN and WAYVNCCTL_BIN must not be empty")
	}
	if cfg.Wayvnc.LogLevel == "" {
		cfg.Wayvnc.LogLevel = "info"
	}

	if _, _, err := net.SplitHostPort(cfg.ListenAddr); err != nil {
		return Config{}, fmt.Errorf("LISTEN_ADDR: %w", err)
	}
	if _, _, err := net.SplitHostPort(cfg.VNCAddress); err != nil {
		return Config{}, fmt.Errorf("VNC_ADDR: %w", err)
	}

	return cfg, nil
}

func loadSessionSecret() ([]byte, bool, error) {
	value, err := readSecret("SESSION_SECRET")
	if err != nil {
		return nil, false, err
	}
	if value != "" {
		if len(value) < 16 {
			return nil, false, errors.New("SESSION_SECRET must be at least 16 characters")
		}
		return []byte(value), false, nil
	}

	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, false, fmt.Errorf("generate session secret: %w", err)
	}
	return secret, true, nil
}

func readSecret(name string) (string, error) {
	if path := os.Getenv(name + "_FILE"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read %s_FILE: %w", name, err)
		}
		return strings.TrimRight(string(data), "\r\n"), nil
	}
	return os.Getenv(name), nil
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func parseBool(raw string) (bool, error) {
	value, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, errors.New("expected true or false")
	}
	return value, nil
}

func parseLogLevel(raw string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, errors.New("LOG_LEVEL must be debug, info, warn, or error")
	}
}

// ValidOutputName reports whether a Wayland output name is safe to pass to wayvncctl.
func ValidOutputName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.', r == ':':
		default:
			return false
		}
	}
	return true
}
