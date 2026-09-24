package auth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/grandpine/ticket-api/internal/apperr"
)

const secret = "0123456789abcdef0123456789abcdef"

func TestAccessTokenRoundTrip(t *testing.T) {
	tk := NewTokens(secret, 15*time.Minute)
	raw, exp, err := tk.IssueAccess(42, true)
	if err != nil {
		t.Fatal(err)
	}
	if time.Until(exp) < 14*time.Minute {
		t.Fatalf("expiry too soon: %v", exp)
	}
	claims, err := tk.ParseAccess(raw)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "42" || claims.Role != "admin" || claims.ID == "" {
		t.Fatalf("claims: %+v", claims)
	}
	if _, _, err := NewTokens(secret, time.Minute).IssueAccess(7, false); err != nil {
		t.Fatal(err)
	}
}

func TestAccessTokenRejections(t *testing.T) {
	tk := NewTokens(secret, 15*time.Minute)
	raw, _, _ := tk.IssueAccess(1, false)

	expired := NewTokens(secret, 15*time.Minute)
	expired.now = func() time.Time { return time.Now().Add(-time.Hour) }
	old, _, _ := expired.IssueAccess(1, false)

	other, _, _ := NewTokens(strings.Repeat("x", 32), 15*time.Minute).IssueAccess(1, false)

	none := jwt.NewWithClaims(jwt.SigningMethodNone, Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "1"}})
	noneRaw, err := none.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}

	// alg: none but otherwise well-formed (valid exp), isolating the HS256
	// algorithm restriction from the "no exp" case above.
	noneWithExp := jwt.NewWithClaims(jwt.SigningMethodNone, Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "1", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
	})
	noneWithExpRaw, err := noneWithExp.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}

	// HS512, signed with the correct secret and a valid exp/sub: rejected
	// purely because it isn't HS256, not because the signature is wrong.
	hs512 := jwt.NewWithClaims(jwt.SigningMethodHS512, Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "1", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
	})
	hs512Raw, err := hs512.SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}

	for name, tok := range map[string]string{
		"expired":           old,
		"wrong key":         other,
		"alg none":          noneRaw,
		"alg none with exp": noneWithExpRaw,
		"alg HS512":         hs512Raw,
		"garbage":           "not.a.jwt",
		"tampered":          raw[:len(raw)-3] + "abc",
	} {
		if _, err := tk.ParseAccess(tok); !errors.Is(err, apperr.ErrUnauthorized) {
			t.Errorf("%s: expected ErrUnauthorized, got %v", name, err)
		}
	}
}

func TestRefreshTokenHash(t *testing.T) {
	raw, hash, err := NewRefreshToken()
	if err != nil || len(raw) != 64 || hash != HashRefreshToken(raw) {
		t.Fatalf("raw=%q hash=%q err=%v", raw, hash, err)
	}
	raw2, _, _ := NewRefreshToken()
	if raw2 == raw {
		t.Fatal("refresh tokens must be random")
	}
}

func TestPassword(t *testing.T) {
	h, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(h, "correct horse") || CheckPassword(h, "wrong") {
		t.Fatal("password check wrong")
	}
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("short password must be rejected")
	}
	var ve *apperr.ValidationError
	long := strings.Repeat("a", 73)
	if _, err := HashPassword(long); !errors.As(err, &ve) || ve.Fields["password"] == "" {
		t.Fatalf("password over 72 bytes must be a validation error, not a 500: %v", err)
	}
	maxLen := strings.Repeat("a", 72)
	if _, err := HashPassword(maxLen); err != nil {
		t.Fatalf("password of exactly 72 bytes must be accepted: %v", err)
	}
}
