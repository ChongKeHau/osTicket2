package auth

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

type Claims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// Tokens issues and verifies HS256 access tokens.
type Tokens struct {
	secret    []byte
	accessTTL time.Duration
	now       func() time.Time
}

func NewTokens(secret string, accessTTL time.Duration) *Tokens {
	return &Tokens{secret: []byte(secret), accessTTL: accessTTL, now: time.Now}
}

func (t *Tokens) AccessTTL() time.Duration { return t.accessTTL }

func (t *Tokens) IssueAccess(staffID int64, isAdmin bool) (string, time.Time, error) {
	role := "agent"
	if isAdmin {
		role = "admin"
	}
	now := t.now()
	exp := now.Add(t.accessTTL)
	claims := Claims{
		Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(staffID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
			ID:        randomHex(16),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, exp, nil
}

func (t *Tokens) ParseAccess(raw string) (Claims, error) {
	var claims Claims
	_, err := jwt.ParseWithClaims(raw, &claims, func(tok *jwt.Token) (any, error) {
		if tok.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method %v", tok.Header["alg"])
		}
		return t.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if err != nil {
		return Claims{}, fmt.Errorf("%w: %v", apperr.ErrUnauthorized, err)
	}
	// Staff tokens carry no audience; any token that names one (the customer
	// portal's carry "client") must never authenticate a staff request.
	if len(claims.Audience) > 0 {
		return Claims{}, fmt.Errorf("%w: token has an audience", apperr.ErrUnauthorized)
	}
	return claims, nil
}

// NewRefreshToken returns a random opaque token and its storage hash.
func NewRefreshToken() (raw, hash string, err error) {
	raw = randomHex(32)
	if raw == "" {
		return "", "", fmt.Errorf("random source unavailable")
	}
	return raw, HashRefreshToken(raw), nil
}

func HashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}
