package auth

import (
	"github.com/grandpine/ticket-api/internal/apperr"
	"golang.org/x/crypto/bcrypt"
)

// bcryptCost is a variable so tests can lower it.
var bcryptCost = bcrypt.DefaultCost

const minPasswordLen = 8

func HashPassword(pw string) (string, error) {
	if len(pw) < minPasswordLen {
		return "", apperr.Validation("password", "must be at least 8 characters")
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
