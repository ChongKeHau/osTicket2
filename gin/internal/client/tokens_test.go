package client

import (
	"testing"
	"time"

	"github.com/grandpine/ticket-api/internal/auth"
)

const secret = "0123456789abcdef0123456789abcdef"

func TestClientTokenRoundTripAndAudience(t *testing.T) {
	ct := NewTokens(secret, 15*time.Minute)
	raw, exp, err := ct.IssueAccess(42, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if time.Until(exp) < 14*time.Minute {
		t.Fatalf("exp too soon: %v", exp)
	}
	c, err := ct.ParseAccess(raw)
	if err != nil {
		t.Fatal(err)
	}
	if c.Subject != "42" || c.TicketID != nil || c.PasswordReset {
		t.Fatalf("claims %+v", c)
	}
	tid := int64(7)
	g, _, _ := ct.IssueAccess(42, &tid, false)
	gc, err := ct.ParseAccess(g)
	if err != nil || gc.TicketID == nil || *gc.TicketID != 7 {
		t.Fatalf("guest claims %+v %v", gc, err)
	}
	pr, _, _ := ct.IssueAccess(42, nil, true)
	if pc, err := ct.ParseAccess(pr); err != nil || !pc.PasswordReset {
		t.Fatalf("password-reset claims %+v %v", pc, err)
	}

	// A staff token is not a client token and vice versa.
	st := auth.NewTokens(secret, 15*time.Minute)
	staffRaw, _, _ := st.IssueAccess(1, true)
	if _, err := ct.ParseAccess(staffRaw); err == nil {
		t.Fatal("client parser accepted a staff token")
	}
	if _, err := st.ParseAccess(raw); err == nil {
		t.Fatal("staff parser accepted a client token")
	}
}

func TestClientTokenRejectsExpiredAndForeignSecret(t *testing.T) {
	ct := NewTokens(secret, 15*time.Minute)
	old := NewTokens(secret, 15*time.Minute)
	old.now = func() time.Time { return time.Now().Add(-time.Hour) }
	raw, _, _ := old.IssueAccess(1, nil, false)
	if _, err := ct.ParseAccess(raw); err == nil {
		t.Fatal("accepted an expired token")
	}
	other, _, _ := NewTokens("xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", time.Minute).IssueAccess(1, nil, false)
	if _, err := ct.ParseAccess(other); err == nil {
		t.Fatal("accepted a token signed with another secret")
	}
	if ct.AccessTTL() != 15*time.Minute {
		t.Fatalf("ttl %v", ct.AccessTTL())
	}
}

func TestOneTimeTokenHash(t *testing.T) {
	raw, hash, err := NewOneTimeToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 64 || HashToken(raw) != hash || len(hash) != 64 {
		t.Fatalf("raw %q hash %q", raw, hash)
	}
	raw2, _, _ := NewOneTimeToken()
	if raw2 == raw {
		t.Fatal("tokens must differ")
	}
}
