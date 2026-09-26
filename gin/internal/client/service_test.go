package client

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/db/testutil"
	"github.com/grandpine/ticket-api/internal/mail"
	"github.com/jackc/pgx/v5"
)

type recordingNotifier struct{ sent []mail.Notification }

func (r *recordingNotifier) Enqueue(_ context.Context, _ *db.Queries, n mail.Notification) error {
	r.sent = append(r.sent, n)
	return nil
}

func (r *recordingNotifier) last(t *testing.T) mail.Notification {
	t.Helper()
	if len(r.sent) == 0 {
		t.Fatal("no mail sent")
	}
	return r.sent[len(r.sent)-1]
}

func newSvc(t *testing.T) (*Service, *recordingNotifier, pgx.Tx) {
	tx := testutil.Tx(t)
	n := &recordingNotifier{}
	svc := NewService(tx, NewTokens(secret, 15*time.Minute), 24*time.Hour, n, "https://desk.test", "Desk")
	svc.run = func(fn func(ctx context.Context)) { fn(context.Background()) } // synchronous: the tx is not concurrency-safe
	return svc, n, tx
}

func TestDefaultRunIsDetachedWithDeadline(t *testing.T) {
	svc := NewService(nil, NewTokens(secret, time.Minute), time.Hour, mail.Disabled{}, "https://desk.test", "Desk")
	done := make(chan time.Duration, 1)
	svc.run(func(ctx context.Context) {
		dl, ok := ctx.Deadline()
		if !ok {
			done <- -1
			return
		}
		done <- time.Until(dl)
	})
	select {
	case left := <-done:
		if left <= 0 || left > backgroundTimeout {
			t.Fatalf("background deadline %v", left)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("background work never ran")
	}
	// A panicking task is recovered, not fatal to the process.
	ran := make(chan struct{})
	svc.run(func(context.Context) { defer close(ran); panic("boom") })
	<-ran
}

// rawFromLink extracts the raw token from https://desk.test/portal/t/<raw>.
func rawFromLink(t *testing.T, link string) string {
	t.Helper()
	const prefix = "https://desk.test/portal/t/"
	if !strings.HasPrefix(link, prefix) || len(link) == len(prefix) {
		t.Fatalf("link %q", link)
	}
	return link[len(prefix):]
}

func shift(svc *Service, d time.Duration) {
	svc.now = func() time.Time { return time.Now().Add(d) }
}

func TestRegisterConfirmLogin(t *testing.T) {
	svc, n, _ := newSvc(t)
	ctx := context.Background()
	svc.Register(RegisterInput{Email: "Pat@Example.test", Name: "Pat"})
	if len(n.sent) != 1 || n.sent[0].TemplateKey != "client_confirm" || n.sent[0].TicketID != nil {
		t.Fatalf("mail %+v", n.sent)
	}
	if to := n.sent[0].To; len(to) != 1 || to[0].Address != "Pat@Example.test" || to[0].Name != "Pat" {
		t.Fatalf("recipient %+v", to)
	}
	if v := n.sent[0].Vars; v.SiteName != "Desk" || v.RequesterName != "Pat" {
		t.Fatalf("vars %+v", v)
	}
	if _, err := svc.Login(ctx, "pat@example.test", "secret123"); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatalf("login before confirm: %v", err)
	}
	// The confirm link opens a password-setting session: pwr claim, no refresh token.
	s, err := svc.Exchange(ctx, rawFromLink(t, n.sent[0].Vars.Link))
	if err != nil || s.Kind != "confirm" || !s.User.Verified || s.User.HasPassword || s.AccessToken == "" || s.RefreshToken != "" || s.TicketID != nil {
		t.Fatalf("exchange %+v %v", s, err)
	}
	c, err := svc.tokens.ParseAccess(s.AccessToken)
	if err != nil || !c.PasswordReset || c.TicketID != nil {
		t.Fatalf("confirm claims %+v %v", c, err)
	}
	if _, err := svc.Exchange(ctx, rawFromLink(t, n.sent[0].Vars.Link)); !errors.Is(err, apperr.ErrTokenInvalid) {
		t.Fatalf("second use: %v", err)
	}
	pwr := Principal{UserID: s.User.ID, Email: s.User.Email, Verified: true, PasswordReset: true}
	var ve *apperr.ValidationError
	if err := svc.SetPassword(ctx, pwr, PasswordInput{Password: "short"}); !errors.As(err, &ve) || ve.Fields["password"] == "" {
		t.Fatalf("short password: %v", err)
	}
	if err := svc.SetPassword(ctx, pwr, PasswordInput{Password: "secret123"}); err != nil {
		t.Fatal(err)
	}
	s2, err := svc.Login(ctx, "PAT@example.test", "secret123")
	if err != nil || s2.User.ID != s.User.ID || !s2.User.HasPassword || !s2.User.Verified || s2.RefreshToken == "" || s2.ExpiresIn != 900 {
		t.Fatalf("login %+v %v", s2, err)
	}
	if _, err := svc.Login(ctx, "pat@example.test", "wrong"); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatal("wrong password accepted")
	}
	if _, err := svc.Login(ctx, "nobody@example.test", "secret123"); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatal("unknown address accepted")
	}
	// An address that already has a password gets no mail and no error.
	svc.Register(RegisterInput{Email: "pat@EXAMPLE.test", Name: "Pat"})
	if len(n.sent) != 1 {
		t.Fatalf("conflicting registration sent mail: %+v", n.sent)
	}
}

func TestUpsertByEmailIsCaseInsensitive(t *testing.T) {
	svc, _, tx := newSvc(t)
	ctx := context.Background()
	q := db.New(tx)
	a, err := svc.UpsertByEmail(ctx, q, "Case@Example.test", "")
	if err != nil || a.Name != "" {
		t.Fatalf("create %+v %v", a, err)
	}
	b, err := svc.UpsertByEmail(ctx, q, "case@EXAMPLE.test", "Casey")
	if err != nil || b.ID != a.ID || b.Name != "Casey" {
		t.Fatalf("fill empty name %+v %v", b, err)
	}
	c, err := svc.UpsertByEmail(ctx, q, "  CASE@example.TEST ", "Other")
	if err != nil || c.ID != a.ID || c.Name != "Casey" {
		t.Fatalf("keep existing name %+v %v", c, err)
	}
	if c.Email != "Case@Example.test" {
		t.Fatalf("stored email changed: %q", c.Email)
	}
}

func TestRegisterAttachesToAnonymousUser(t *testing.T) {
	svc, n, tx := newSvc(t)
	ctx := context.Background()
	anon, err := svc.UpsertByEmail(ctx, db.New(tx), "a@x.test", "A")
	if err != nil {
		t.Fatal(err)
	}
	if anon.PasswordHash != nil {
		t.Fatal("anonymous user has a password")
	}
	svc.Register(RegisterInput{Email: "A@X.test", Name: "Alice"})
	if len(n.sent) != 1 || n.sent[0].TemplateKey != "client_confirm" || n.sent[0].To[0].Address != "a@x.test" {
		t.Fatalf("mail %+v", n.sent)
	}
	// Registering stores no password: the address stays unclaimed until confirmed.
	p, err := svc.Me(ctx, anon.ID)
	if err != nil || p.ID != anon.ID || p.HasPassword || p.Verified {
		t.Fatalf("profile after register %+v %v", p, err)
	}
	s, err := svc.Exchange(ctx, rawFromLink(t, n.sent[0].Vars.Link))
	if err != nil || s.User.ID != anon.ID || s.Kind != "confirm" {
		t.Fatalf("exchange %+v %v", s, err)
	}
	if err := svc.SetPassword(ctx, Principal{UserID: anon.ID, PasswordReset: true}, PasswordInput{Password: "secret123"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Login(ctx, "a@x.test", "secret123"); err != nil {
		t.Fatalf("login: %v", err)
	}
	// An address that already has a password gets no mail and no error.
	svc.Register(RegisterInput{Email: "a@x.test", Name: "A"})
	if len(n.sent) != 1 {
		t.Fatalf("conflicting register sent mail: %+v", n.sent)
	}
}

func TestSigninLinkFlow(t *testing.T) {
	svc, n, tx := newSvc(t)
	ctx := context.Background()
	svc.RequestLink("ghost@x.test")
	if len(n.sent) != 0 {
		t.Fatalf("unknown address: %+v", n.sent)
	}
	u, err := svc.UpsertByEmail(ctx, db.New(tx), "link@x.test", "Lin")
	if err != nil {
		t.Fatal(err)
	}
	svc.RequestLink("LINK@x.test")
	m := n.last(t)
	if m.TemplateKey != "client_signin_link" || m.TicketID != nil || m.To[0].Address != "link@x.test" {
		t.Fatalf("mail %+v", m)
	}
	s, err := svc.Exchange(ctx, rawFromLink(t, m.Vars.Link))
	if err != nil || s.Kind != "signin" || s.User.ID != u.ID || s.RefreshToken == "" || s.TicketID != nil {
		t.Fatalf("exchange %+v %v", s, err)
	}
	c, err := svc.tokens.ParseAccess(s.AccessToken)
	if err != nil || c.Subject != strconv.FormatInt(u.ID, 10) || c.TicketID != nil || c.PasswordReset {
		t.Fatalf("claims %+v %v", c, err)
	}
	r, err := svc.Refresh(ctx, s.RefreshToken)
	if err != nil || r.RefreshToken == "" || r.RefreshToken == s.RefreshToken || r.User.ID != u.ID {
		t.Fatalf("refresh %+v %v", r, err)
	}
	if _, err := svc.Refresh(ctx, s.RefreshToken); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatalf("old refresh token reused: %v", err)
	}
	// Logout is scoped to the owner: another user id cannot revoke the token.
	if err := svc.Logout(ctx, u.ID+1000, r.RefreshToken); err != nil {
		t.Fatal(err)
	}
	r2, err := svc.Refresh(ctx, r.RefreshToken)
	if err != nil {
		t.Fatalf("token revoked by another user: %v", err)
	}
	if err := svc.Logout(ctx, u.ID, r2.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refresh(ctx, r2.RefreshToken); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatalf("refresh after logout: %v", err)
	}
}

func insertTicket(t *testing.T, tx pgx.Tx, number, name, email string) int64 {
	t.Helper()
	var id int64
	if err := tx.QueryRow(context.Background(), `INSERT INTO ticket (number, subject, status_id, dept_id, priority_id, requester_name, requester_email)
		VALUES ($1, 'Printer ' || $1, (SELECT id FROM ticket_status ORDER BY id LIMIT 1), (SELECT id FROM department ORDER BY id LIMIT 1),
		        (SELECT id FROM ticket_priority ORDER BY id LIMIT 1), $2, $3) RETURNING id`, number, name, email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestGuestSessionScopedToTicket(t *testing.T) {
	svc, n, tx := newSvc(t)
	ctx := context.Background()
	t1 := insertTicket(t, tx, "910001", "Pat Guest", "pat@guest.test")
	insertTicket(t, tx, "910002", "Pat Guest", "pat@guest.test")

	svc.RequestAccess("someone@else.test", "910001")
	if len(n.sent) != 0 {
		t.Fatalf("wrong email: %+v", n.sent)
	}
	svc.RequestAccess("pat@guest.test", "999999")
	if len(n.sent) != 0 {
		t.Fatalf("unknown number: %+v", n.sent)
	}
	svc.RequestAccess("PAT@guest.test", "910001")
	// The ticket had no user; the access request claims it for the requester.
	pt, err := db.New(tx).GetPortalTicket(ctx, t1)
	if err != nil || pt.UserID == nil {
		t.Fatalf("ticket user after access request %+v %v", pt, err)
	}
	m := n.last(t)
	if m.TemplateKey != "client_access_link" || m.TicketID == nil || *m.TicketID != t1 ||
		m.Vars.Number != "910001" || m.Vars.Subject != "Printer 910001" || m.Vars.RequesterName != "Pat Guest" {
		t.Fatalf("mail %+v", m)
	}
	s, err := svc.Exchange(ctx, rawFromLink(t, m.Vars.Link))
	if err != nil || s.Kind != "access" || s.TicketID == nil || *s.TicketID != t1 || s.RefreshToken == "" {
		t.Fatalf("exchange %+v %v", s, err)
	}
	if c, err := svc.tokens.ParseAccess(s.AccessToken); err != nil || c.TicketID == nil || *c.TicketID != t1 {
		t.Fatalf("access claims %+v %v", c, err)
	}
	if s.User.Name != "Pat Guest" || !strings.EqualFold(s.User.Email, "pat@guest.test") {
		t.Fatalf("user %+v", s.User)
	}
	if *pt.UserID != s.User.ID {
		t.Fatalf("ticket claimed by %d, session user %d", *pt.UserID, s.User.ID)
	}
	// A ticket that already has a user keeps it.
	other, err := svc.UpsertByEmail(ctx, db.New(tx), "owner@x.test", "Owner")
	if err != nil {
		t.Fatal(err)
	}
	t3 := insertTicket(t, tx, "910003", "Pat Guest", "pat@guest.test")
	if err := db.New(tx).SetTicketUser(ctx, db.SetTicketUserParams{ID: t3, UserID: &other.ID}); err != nil {
		t.Fatal(err)
	}
	svc.RequestAccess("pat@guest.test", "910003")
	if pt3, err := db.New(tx).GetPortalTicket(ctx, t3); err != nil || pt3.UserID == nil || *pt3.UserID != other.ID {
		t.Fatalf("existing ticket user overwritten %+v %v", pt3, err)
	}
	r, err := svc.Refresh(ctx, s.RefreshToken)
	if err != nil || r.TicketID == nil || *r.TicketID != t1 {
		t.Fatalf("refresh %+v %v", r, err)
	}
	if c, err := svc.tokens.ParseAccess(r.AccessToken); err != nil || c.TicketID == nil || *c.TicketID != t1 {
		t.Fatalf("refreshed claims %+v %v", c, err)
	}
}

func TestTokenExpiry(t *testing.T) {
	svc, n, _ := newSvc(t)
	ctx := context.Background()
	svc.Register(RegisterInput{Email: "exp@x.test", Name: "Exp"})
	confirm := rawFromLink(t, n.last(t).Vars.Link)
	svc.RequestLink("exp@x.test")
	signin := rawFromLink(t, n.last(t).Vars.Link)
	svc.RequestLink("exp@x.test")
	signinFresh := rawFromLink(t, n.last(t).Vars.Link)

	shift(svc, 2*time.Hour)
	if _, err := svc.Exchange(ctx, signin); !errors.Is(err, apperr.ErrTokenInvalid) {
		t.Fatalf("signin after 2h: %v", err)
	}
	shift(svc, 25*time.Hour)
	if _, err := svc.Exchange(ctx, confirm); !errors.Is(err, apperr.ErrTokenInvalid) {
		t.Fatalf("confirm after 25h: %v", err)
	}
	shift(svc, 59*time.Minute)
	s, err := svc.Exchange(ctx, signinFresh)
	if err != nil {
		t.Fatalf("signin within the hour: %v", err)
	}
	shift(svc, 59*time.Minute+25*time.Hour)
	if _, err := svc.Refresh(ctx, s.RefreshToken); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatalf("refresh after its ttl: %v", err)
	}
}

func TestResetFlow(t *testing.T) {
	svc, n, tx := newSvc(t)
	ctx := context.Background()
	svc.RequestReset("ghost@x.test")
	if len(n.sent) != 0 {
		t.Fatalf("unknown address: %+v", n.sent)
	}
	u, err := svc.UpsertByEmail(ctx, db.New(tx), "reset@x.test", "Rae")
	if err != nil {
		t.Fatal(err)
	}
	svc.RequestReset("reset@x.test")
	m := n.last(t)
	if m.TemplateKey != "client_reset" || m.TicketID != nil {
		t.Fatalf("mail %+v", m)
	}
	s, err := svc.Exchange(ctx, rawFromLink(t, m.Vars.Link))
	if err != nil || s.Kind != "reset" || s.RefreshToken != "" || s.TicketID != nil || s.User.Verified {
		t.Fatalf("exchange %+v %v", s, err)
	}
	c, err := svc.tokens.ParseAccess(s.AccessToken)
	if err != nil || !c.PasswordReset || c.TicketID != nil {
		t.Fatalf("claims %+v %v", c, err)
	}
	reset := Principal{UserID: u.ID, Email: u.Email, PasswordReset: true}
	if err := svc.SetPassword(ctx, reset, PasswordInput{Password: "newpass123"}); err != nil {
		t.Fatal(err)
	}
	if p, err := svc.Me(ctx, u.ID); err != nil || !p.Verified || !p.HasPassword {
		t.Fatalf("profile after reset %+v %v", p, err)
	}
	login, err := svc.Login(ctx, "reset@x.test", "newpass123")
	if err != nil {
		t.Fatalf("login after reset: %v", err)
	}

	normal := Principal{UserID: u.ID, Email: u.Email, Verified: true}
	var ve *apperr.ValidationError
	if err := svc.SetPassword(ctx, normal, PasswordInput{Password: "another123"}); !errors.As(err, &ve) || ve.Fields["current_password"] == "" {
		t.Fatalf("missing current password: %v", err)
	}
	if err := svc.SetPassword(ctx, normal, PasswordInput{Password: "another123", CurrentPassword: "wrong-one"}); !errors.As(err, &ve) || ve.Fields["current_password"] == "" {
		t.Fatalf("wrong current password: %v", err)
	}
	if err := svc.SetPassword(ctx, normal, PasswordInput{Password: "another123", CurrentPassword: "newpass123"}); err != nil {
		t.Fatal(err)
	}
	// A password change revokes every refresh token the user holds.
	if _, err := svc.Refresh(ctx, login.RefreshToken); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatalf("refresh after password change: %v", err)
	}
	if _, err := svc.Login(ctx, "reset@x.test", "newpass123"); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatal("old password still accepted")
	}
	if _, err := svc.Login(ctx, "reset@x.test", "another123"); err != nil {
		t.Fatalf("login with changed password: %v", err)
	}
}

func TestProfileUpdate(t *testing.T) {
	svc, _, tx := newSvc(t)
	ctx := context.Background()
	u, err := svc.UpsertByEmail(ctx, db.New(tx), "prof@x.test", "Old")
	if err != nil {
		t.Fatal(err)
	}
	p, err := svc.UpdateName(ctx, u.ID, "  New Name ")
	if err != nil || p.Name != "New Name" || p.ID != u.ID || p.HasPassword {
		t.Fatalf("update %+v %v", p, err)
	}
	var ve *apperr.ValidationError
	if _, err := svc.UpdateName(ctx, u.ID, "   "); !errors.As(err, &ve) || ve.Fields["name"] == "" {
		t.Fatalf("blank name: %v", err)
	}
	if _, err := svc.Me(ctx, u.ID+1000); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("missing user: %v", err)
	}
}

func TestAudienceSeparation(t *testing.T) {
	svc, n, _ := newSvc(t)
	ctx := context.Background()
	if _, err := svc.LoadClient(ctx, 987654321); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatalf("unknown user: %v", err)
	}
	svc.Register(RegisterInput{Email: "aud@x.test", Name: "Aud"})
	s, err := svc.Exchange(ctx, rawFromLink(t, n.last(t).Vars.Link))
	if err != nil {
		t.Fatal(err)
	}
	p, err := svc.LoadClient(ctx, s.User.ID)
	if err != nil || p.UserID != s.User.ID || p.Email != "aud@x.test" || !p.Verified || p.TicketID != nil {
		t.Fatalf("principal %+v %v", p, err)
	}
	if _, err := auth.NewTokens(secret, time.Minute).ParseAccess(s.AccessToken); err == nil {
		t.Fatal("staff parser accepted a portal session token")
	}
}
