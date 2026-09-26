package client

import (
	"strings"
	"time"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
)

// Limiter applies one fixed-window budget per address and another per client IP.
type Limiter struct{ byEmail, byIP *auth.RateLimiter }

func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{byEmail: auth.NewRateLimiter(max, window), byIP: auth.NewRateLimiter(max, window)}
}

// Check counts one attempt for the address and the IP and, when either is
// over its budget, reports the longest wait among the keys that are over.
func (l *Limiter) Check(email, ip string) error {
	e := "e|" + strings.ToLower(strings.TrimSpace(email))
	i := "i|" + ip
	okE, okI := l.byEmail.Allow(e), l.byIP.Allow(i)
	if okE && okI {
		return nil
	}
	var after time.Duration
	if !okE {
		after = l.byEmail.RetryAfter(e)
	}
	if a := l.byIP.RetryAfter(i); !okI && a > after {
		after = a
	}
	return apperr.RateLimited(after)
}
