package auth

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRateLimiterWindowRetryAfterAndReset(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	l := NewRateLimiter(2, time.Minute)
	l.now = func() time.Time { return now }
	if l.RetryAfter("k") != 0 {
		t.Fatal("unknown key must report no wait")
	}
	if !l.Allow("k") || !l.Allow("k") || l.Allow("k") {
		t.Fatal("want two admitted then refused")
	}
	now = now.Add(20 * time.Second)
	if got := l.RetryAfter("k"); got != 40*time.Second {
		t.Fatalf("retry after %v, want 40s", got)
	}
	l.Reset("k")
	if !l.Allow("k") {
		t.Fatal("reset must clear the counter")
	}
	now = now.Add(2 * time.Minute)
	if l.RetryAfter("k") != 0 {
		t.Fatal("expired window must report no wait")
	}
	if !l.Allow("k") || !l.Allow("k") {
		t.Fatal("a new window must admit again")
	}
}

// TestLoginRateLimiterAllowIsAtomic proves Allow() reserves its slot under
// the same lock as the check: a burst of concurrent calls for one key can
// never together admit more than loginRateLimitMax, even though the caller
// (the login handler) only calls Login -- and could fail or reset -- well
// after Allow() returns.
func TestLoginRateLimiterAllowIsAtomic(t *testing.T) {
	l := newLoginRateLimiter()
	const n = 15
	var admitted int32
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if l.Allow("k") {
				atomic.AddInt32(&admitted, 1)
			}
		}()
	}
	wg.Wait()
	if admitted > loginRateLimitMax {
		t.Fatalf("admitted %d of %d concurrent attempts, want at most %d", admitted, n, loginRateLimitMax)
	}
	if admitted == 0 {
		t.Fatal("expected at least one concurrent attempt to be admitted")
	}
}
