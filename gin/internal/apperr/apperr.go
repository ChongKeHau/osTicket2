// Package apperr defines the error kinds services return and handlers map to HTTP.
package apperr

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrForbidden       = errors.New("forbidden")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrConflict        = errors.New("conflict")
	ErrPayloadTooLarge = errors.New("payload too large")
)

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
