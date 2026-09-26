// Package apperr defines the error kinds services return and handlers map to HTTP.
package apperr

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrForbidden       = errors.New("forbidden")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrConflict        = errors.New("conflict")
	ErrPayloadTooLarge = errors.New("payload too large")
	ErrRateLimited     = errors.New("rate limited")
	// ErrTokenInvalid is an emailed one-time token that is unknown, expired or used.
	ErrTokenInvalid = errors.New("token invalid")
	// ErrGuestSession is a guest (single-ticket) portal session on an account-only route.
	ErrGuestSession = errors.New("guest session")
)

// RateLimitedError is ErrRateLimited with the wait until the next attempt may succeed.
type RateLimitedError struct{ RetryAfter time.Duration }

func (e *RateLimitedError) Error() string { return "rate limited" }

// Is makes errors.Is(err, ErrRateLimited) hold for a *RateLimitedError.
func (e *RateLimitedError) Is(target error) bool { return target == ErrRateLimited }

// RateLimited builds a RateLimitedError.
func RateLimited(after time.Duration) error { return &RateLimitedError{RetryAfter: after} }

// ValidationError carries per-field messages for a 400 response.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s: %s", k, e.Fields[k]))
	}
	return "validation failed: " + strings.Join(parts, ", ")
}

// Validation builds a single-field ValidationError.
func Validation(field, msg string) *ValidationError {
	return &ValidationError{Fields: map[string]string{field: msg}}
}
