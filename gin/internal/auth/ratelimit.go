package auth

import (
	"sync"
	"time"
)

const (
	loginRateLimitMax    = 10
	loginRateLimitWindow = 60 * time.Second
	// rateLimitPruneAt bounds memory: once the map holds more than this
	// many keys, each check sweeps expired entries before doing its work.
	rateLimitPruneAt = 10_000
)

// RateLimiter is an in-memory fixed-window limiter keyed by an arbitrary
// string (the staff login uses "username|clientIP"). It is process-local (not
// shared across replicas) and is meant to slow down brute-force attempts, not
// to be a precise distributed rate limit.
type RateLimiter struct {
	mu      sync.Mutex
	max     int
	window  time.Duration
	now     func() time.Time
	entries map[string]*rateEntry
}

type rateEntry struct {
	count      int
	windowEnds time.Time
}

// NewRateLimiter admits at most max attempts per key in each window.
func NewRateLimiter(max int, window time.Duration) *RateLimiter {
	return &RateLimiter{max: max, window: window, now: time.Now, entries: make(map[string]*rateEntry)}
}

func newLoginRateLimiter() *RateLimiter {
	return NewRateLimiter(loginRateLimitMax, loginRateLimitWindow)
}

// Allow reports whether an attempt for key may proceed, and if so reserves
// it: the check and the increment happen under the same lock, so a burst of
// concurrent calls for the same key can never together admit more than max.
// Every admitted call counts, whether the attempt that follows succeeds or
// fails; the login handler calls Reset(key) after a successful login so only
// failures count towards the next window.
func (l *RateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.pruneLocked(now)
	e, ok := l.entries[key]
	if !ok || now.After(e.windowEnds) {
		e = &rateEntry{windowEnds: now.Add(l.window)}
		l.entries[key] = e
	}
	if e.count >= l.max {
		return false
	}
	e.count++
	return true
}

// Reset clears key's counter.
func (l *RateLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

// RetryAfter is the time until key's current window ends, or 0 if the key
// has no live window.
func (l *RateLimiter) RetryAfter(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok {
		return 0
	}
	if d := e.windowEnds.Sub(l.now()); d > 0 {
		return d
	}
	return 0
}

// pruneLocked drops expired entries once the map grows large, so a
// long-running process doesn't leak memory for keys that stop trying.
// Must be called with l.mu held.
func (l *RateLimiter) pruneLocked(now time.Time) {
	if len(l.entries) <= rateLimitPruneAt {
		return
	}
	for k, e := range l.entries {
		if now.After(e.windowEnds) {
			delete(l.entries, k)
		}
	}
}
