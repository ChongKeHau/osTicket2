package auth

import (
	"sync"
	"time"
)

const (
	loginRateLimitMax    = 10
	loginRateLimitWindow = 60 * time.Second
	// loginRateLimitPruneAt bounds memory: once the map holds more than this
	// many keys, each check sweeps expired entries before doing its work.
	loginRateLimitPruneAt = 10_000
)

// loginRateLimiter is an in-memory fixed-window limiter keyed by
// "username|clientIP". It is process-local (not shared across replicas) and
// is meant to slow down brute-force login attempts, not to be a precise
// distributed rate limit.
type loginRateLimiter struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[string]*loginRateEntry
}

type loginRateEntry struct {
	count      int
	windowEnds time.Time
}

func newLoginRateLimiter() *loginRateLimiter {
	return &loginRateLimiter{now: time.Now, entries: make(map[string]*loginRateEntry)}
}

// allow reports whether an attempt for key may proceed, and if so reserves
// it: the check and the increment happen under the same lock, so a burst of
// concurrent calls for the same key can never together admit more than
// loginRateLimitMax. Every admitted call counts, whether the login that
// follows succeeds or fails; a successful login must call reset(key)
// afterward so only failures count towards the next window, per the
// package's "only failed logins count" contract.
func (l *loginRateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.pruneLocked(now)
	e, ok := l.entries[key]
	if !ok || now.After(e.windowEnds) {
		e = &loginRateEntry{windowEnds: now.Add(loginRateLimitWindow)}
		l.entries[key] = e
	}
	if e.count >= loginRateLimitMax {
		return false
	}
	e.count++
	return true
}

// reset clears key's counter, called after a successful login.
func (l *loginRateLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

// pruneLocked drops expired entries once the map grows large, so a
// long-running process doesn't leak memory for usernames/IPs that stop
// trying. Must be called with l.mu held.
func (l *loginRateLimiter) pruneLocked(now time.Time) {
	if len(l.entries) <= loginRateLimitPruneAt {
		return
	}
	for k, e := range l.entries {
		if now.After(e.windowEnds) {
			delete(l.entries, k)
		}
	}
}
