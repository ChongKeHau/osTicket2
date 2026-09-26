// Package client holds the customer-portal identity: access tokens with the
// "client" audience, one-time email tokens, the request principal and its
// middleware, and the portal rate limiter.
package client

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/grandpine/ticket-api/internal/apperr"
)

// Audience is the aud claim every client access token carries; staff tokens
// have none, so neither parser accepts the other's tokens.
const Audience = "client"

// Claims are a client access token's claims. TicketID is set for a guest
// session scoped to one ticket; PasswordReset marks a session opened from a
// reset link, which may set a new password without the old one.
type Claims struct {
	TicketID      *int64 `json:"tid,omitempty"`
	PasswordReset bool   `json:"pwr,omitempty"`
	jwt.RegisteredClaims
}

// Tokens issues and verifies HS256 client access tokens.
type Tokens struct {
	secret    []byte
	accessTTL time.Duration
	now       func() time.Time
}

func NewTokens(secret string, accessTTL time.Duration) *Tokens {
	return &Tokens{secret: []byte(secret), accessTTL: accessTTL, now: time.Now}
}

func (t *Tokens) AccessTTL() time.Duration { return t.accessTTL }

func (t *Tokens) IssueAccess(userID int64, ticketID *int64, passwordReset bool) (string, time.Time, error) {
	now := t.now()
	exp := now.Add(t.accessTTL)
	jti := make([]byte, 16)
	if _, err := rand.Read(jti); err != nil {
		return "", time.Time{}, err
	}
	c := Claims{TicketID: ticketID, PasswordReset: passwordReset, RegisteredClaims: jwt.RegisteredClaims{
		Subject: strconv.FormatInt(userID, 10), Audience: jwt.ClaimStrings{Audience},
		IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(exp), ID: hex.EncodeToString(jti),
	}}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(t.secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return s, exp, nil
}

func (t *Tokens) ParseAccess(raw string) (Claims, error) {
	var c Claims
	_, err := jwt.ParseWithClaims(raw, &c, func(*jwt.Token) (any, error) { return t.secret, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithAudience(Audience), jwt.WithTimeFunc(t.now))
	if err != nil {
		return Claims{}, fmt.Errorf("%w: %v", apperr.ErrUnauthorized, err)
	}
	return c, nil
}

// NewOneTimeToken returns a raw 32-byte token (hex) and its sha256 hex, the only form stored.
func NewOneTimeToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	raw = hex.EncodeToString(b)
	return raw, HashToken(raw), nil
}

// HashToken is the storage form of a one-time token.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
