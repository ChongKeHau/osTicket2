package auth

import (
	"sync"

	"github.com/grandpine/ticket-api/internal/apperr"
	"golang.org/x/crypto/bcrypt"
)

// bcryptCost is a variable so tests can lower it.
var bcryptCost = bcrypt.DefaultCost

const (
	minPasswordLen = 8
	maxPasswordLen = 72
)

// dummyHash is a bcrypt hash of a fixed placeholder password. Login compares
// against it when the username doesn't exist, so an unknown username costs
// the same bcrypt compare as a real one and can't be timed apart. It's
// generated once, lazily on first use (not in a package init()), so it
// always reflects whatever bcryptCost is in effect by the time a caller
// actually needs it — including a bcryptCost a test lowers in its own init().
var (
	dummyHash     string
	dummyHashOnce sync.Once
)

func ensureDummyHash() string {
	dummyHashOnce.Do(func() {
		h, err := bcrypt.GenerateFromPassword([]byte("dummy-password-for-constant-time-compare"), bcryptCost)
		if err != nil {
			// GenerateFromPassword only fails for a pathological cost; fall back
			// to a hash that never matches so the timing property still holds.
			dummyHash = "$2a$10$CwTycUXWue0Thq9StjUM0uJ8Q4h2ffTNVJj7Wc0mm8VfM/y8vSFO6"
			return
		}
		dummyHash = string(h)
	})
	return dummyHash
}

func HashPassword(pw string) (string, error) {
	if len(pw) < minPasswordLen {
		return "", apperr.Validation("password", "must be at least 8 characters")
	}
	if len(pw) > maxPasswordLen {
		return "", apperr.Validation("password", "must be at most 72 bytes")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}
