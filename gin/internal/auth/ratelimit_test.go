package auth

import (
	"sync"
	"sync/atomic"
	"testing"
)

// TestLoginRateLimiterAllowIsAtomic proves allow() reserves its slot under
// the same lock as the check: a burst of concurrent calls for one key can
// never together admit more than loginRateLimitMax, even though the caller
// (the login handler) only calls Login -- and could fail or reset -- well
// after allow() returns.
func TestLoginRateLimiterAllowIsAtomic(t *testing.T) {
	l := newLoginRateLimiter()
	const n = 15
	var admitted int32
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if l.allow("k") {
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
