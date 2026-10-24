package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Manager issues and validates signed, stateless session tokens.
type Manager struct {
	password []byte
	secret   []byte
	ttl      time.Duration
	now      func() time.Time
}

// New creates a session manager. The password is compared in constant time.
func New(password string, secret []byte, ttl time.Duration) *Manager {
	return &Manager{
		password: []byte(password),
		secret:   append([]byte(nil), secret...),
		ttl:      ttl,
		now:      time.Now,
	}
}

// CheckPassword compares a candidate password without early exit on content.
func (m *Manager) CheckPassword(candidate string) bool {
	got := []byte(candidate)
	want := m.password
	if len(got) != len(want) {
		// Keep the comparison cost independent of the provided length.
		dummy := make([]byte, len(got))
		subtle.ConstantTimeCompare(got, dummy)
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

// Issue returns a signed session token containing its expiration time.
func (m *Manager) Issue() (string, time.Time, error) {
	expires := m.now().Add(m.ttl)
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", time.Time{}, fmt.Errorf("generate session nonce: %w", err)
	}
	payload := strconv.FormatInt(expires.Unix(), 10) + "." + base64.RawURLEncoding.EncodeToString(nonce)
	return encode(payload) + "." + encodeBytes(m.sign([]byte(payload))), expires, nil
}

// Validate reports whether token is authentic and unexpired.
func (m *Manager) Validate(token string) bool {
	if token == "" {
		return false
	}
	dot := strings.LastIndexByte(token, '.')
	if dot <= 0 || dot == len(token)-1 {
		return false
	}
	payload, err := decode(token[:dot])
	if err != nil {
		return false
	}
	signature, err := decodeBytes(token[dot+1:])
	if err != nil {
		return false
	}
	if !hmac.Equal(signature, m.sign([]byte(payload))) {
		return false
	}
	parts := strings.SplitN(payload, ".", 2)
	if len(parts) != 2 {
		return false
	}
	expires, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return false
	}
	return m.now().Unix() < expires
}

func (m *Manager) sign(payload []byte) []byte {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write(payload)
	return mac.Sum(nil)
}

func encode(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func encodeBytes(value []byte) string {
	return base64.RawURLEncoding.EncodeToString(value)
}

func decode(value string) (string, error) {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeBytes(value string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(value)
}

// Limiter tracks failed login attempts per client and temporarily blocks
// clients that exceed the failure budget.
type Limiter struct {
	mu         sync.Mutex
	attempts   map[string]*attempt
	max        int
	window     time.Duration
	block      time.Duration
	now        func() time.Time
	operations int
}

type attempt struct {
	failures     int
	first        time.Time
	blockedUntil time.Time
}

// NewLimiter creates a failed-login limiter.
func NewLimiter(max int, window, block time.Duration) *Limiter {
	return &Limiter{
		attempts: make(map[string]*attempt),
		max:      max,
		window:   window,
		block:    block,
		now:      time.Now,
	}
}

// Allowed reports whether a login attempt may proceed.
func (l *Limiter) Allowed(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.gcLocked()

	entry, ok := l.attempts[key]
	if !ok {
		return true, 0
	}
	now := l.now()
	if now.Before(entry.blockedUntil) {
		return false, entry.blockedUntil.Sub(now)
	}
	if now.Sub(entry.first) > l.window {
		delete(l.attempts, key)
	}
	return true, 0
}

// Failure records a failed login attempt.
func (l *Limiter) Failure(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	entry, ok := l.attempts[key]
	if !ok || now.Sub(entry.first) > l.window {
		entry = &attempt{first: now}
		l.attempts[key] = entry
	}
	entry.failures++
	if entry.failures >= l.max {
		entry.blockedUntil = now.Add(l.block)
	}
}

// Success clears the failure history for a client.
func (l *Limiter) Success(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}

func (l *Limiter) gcLocked() {
	l.operations++
	if l.operations%256 != 0 && len(l.attempts) < 4096 {
		return
	}
	now := l.now()
	for key, entry := range l.attempts {
		if now.After(entry.blockedUntil) && now.Sub(entry.first) > l.window {
			delete(l.attempts, key)
		}
	}
}
