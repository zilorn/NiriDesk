package auth

import (
	"testing"
	"time"
)

func TestPasswordComparison(t *testing.T) {
	manager := New("correct horse battery", []byte("0123456789abcdef"), time.Hour)
	if !manager.CheckPassword("correct horse battery") {
		t.Fatal("expected matching password to succeed")
	}
	if manager.CheckPassword("wrong") {
		t.Fatal("expected wrong password to fail")
	}
	if manager.CheckPassword("correct horse battery ") {
		t.Fatal("expected trailing space to matter")
	}
}

func TestSessionIssueValidateAndExpiry(t *testing.T) {
	manager := New("password123", []byte("0123456789abcdef"), time.Hour)
	now := time.Unix(1_700_000_000, 0)
	manager.now = func() time.Time { return now }

	token, expires, err := manager.Issue()
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if !expires.Equal(now.Add(time.Hour)) {
		t.Fatalf("unexpected expiry: %s", expires)
	}
	if !manager.Validate(token) {
		t.Fatal("expected issued token to validate")
	}

	tampered := token[:len(token)-1] + "x"
	if manager.Validate(tampered) {
		t.Fatal("expected tampered token to fail")
	}

	now = now.Add(2 * time.Hour)
	if manager.Validate(token) {
		t.Fatal("expected expired token to fail")
	}
}

func TestInvalidTokenShapes(t *testing.T) {
	manager := New("password123", []byte("0123456789abcdef"), time.Hour)
	for _, token := range []string{"", ".", "a.b", "!!!.???", "abc"} {
		if manager.Validate(token) {
			t.Fatalf("expected invalid token %q to fail", token)
		}
	}
}

func TestLimiterBlocksAndRecovers(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	limiter := NewLimiter(3, 10*time.Minute, 2*time.Minute)
	limiter.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if allowed, _ := limiter.Allowed("client"); !allowed {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
		limiter.Failure("client")
	}
	allowed, retry := limiter.Allowed("client")
	if allowed {
		t.Fatal("expected client to be blocked")
	}
	if retry <= 0 || retry > 2*time.Minute {
		t.Fatalf("unexpected retry duration: %s", retry)
	}

	now = now.Add(2*time.Minute + time.Second)
	if allowed, _ := limiter.Allowed("client"); !allowed {
		t.Fatal("expected client to recover after the block")
	}

	limiter.Failure("client")
	limiter.Success("client")
	if allowed, _ := limiter.Allowed("client"); !allowed {
		t.Fatal("expected success to clear history")
	}
}

func TestLimiterSeparatesClients(t *testing.T) {
	limiter := NewLimiter(1, time.Minute, time.Minute)
	limiter.Failure("one")
	if allowed, _ := limiter.Allowed("one"); allowed {
		t.Fatal("one should be blocked")
	}
	if allowed, _ := limiter.Allowed("two"); !allowed {
		t.Fatal("two should not be blocked")
	}
}
