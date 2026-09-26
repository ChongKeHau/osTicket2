package client

import (
	"errors"
	"testing"
	"time"

	"github.com/grandpine/ticket-api/internal/apperr"
)

func TestLimiterChecksEmailAndIP(t *testing.T) {
	l := NewLimiter(2, time.Minute)
	if err := l.Check("a@x.test", "1.1.1.1"); err != nil {
		t.Fatal(err)
	}
	if err := l.Check("a@x.test", "2.2.2.2"); err != nil {
		t.Fatal(err)
	}
	err := l.Check("A@X.TEST", "3.3.3.3") // same email, case-insensitive, third hit
	var rl *apperr.RateLimitedError
	if !errors.As(err, &rl) || rl.RetryAfter <= 0 {
		t.Fatalf("want rate limited, got %v", err)
	}
	if !errors.Is(err, apperr.ErrRateLimited) {
		t.Fatal("RateLimitedError must match ErrRateLimited")
	}
	if err := l.Check("b@x.test", "1.1.1.1"); err != nil {
		t.Fatal(err)
	} // ip has 2 hits now
	if err := l.Check("c@x.test", "1.1.1.1"); err == nil {
		t.Fatal("ip over limit")
	}
}
