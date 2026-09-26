package client

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/mail"
	"github.com/jackc/pgx/v5"
)

// One-time token lifetimes.
const (
	confirmTTL = 24 * time.Hour
	resetTTL   = 24 * time.Hour
	signinTTL  = time.Hour
	accessTTL  = time.Hour
)

// Profile is the end user as the portal shows it.
type Profile struct {
	ID          int64  `json:"id"`
	Email       string `json:"email"`
	Name        string `json:"name"`
	Verified    bool   `json:"verified"`
	HasPassword bool   `json:"has_password"`
}

// Session is a portal sign-in result. TicketID is set for a guest session;
// Kind names the emailed token that opened it (confirm, signin, access,
// reset) and is empty for a password login or a refresh. Confirm and reset
// sessions carry no refresh token; they exist to set the password.
type Session struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	ExpiresIn    int     `json:"expires_in"`
	User         Profile `json:"user"`
	TicketID     *int64  `json:"ticket_id"`
	Kind         string  `json:"kind,omitempty"`
}

// RegisterInput starts an account. It carries no password: the person who
// follows the confirm link sets one, so an address cannot be claimed with a
// password its owner never chose.
type RegisterInput struct {
	Email string `json:"email" binding:"required,email,max=255"`
	Name  string `json:"name" binding:"required,max=128"`
}

type PasswordInput struct {
	Password        string `json:"password" binding:"required"`
	CurrentPassword string `json:"current_password"`
}

// Service is the portal identity service: accounts, emailed one-time links,
// guest access, sessions and the profile.
type Service struct {
	b          db.Beginner
	tokens     *Tokens
	refreshTTL time.Duration
	notifier   mail.Notifier
	baseURL    string
	siteName   string
	now        func() time.Time
	// run executes the account-mail work behind Register, RequestLink,
	// RequestReset and RequestAccess after the handler has already answered,
	// so response time never depends on whether an address or ticket exists.
	// Tests replace it with a synchronous call.
	run func(fn func(ctx context.Context))
}

// backgroundTimeout bounds one piece of background account-mail work.
const backgroundTimeout = 15 * time.Second

// runInBackground is the default Service.run: a goroutine with its own
// deadline, detached from the request, with panics logged rather than fatal.
func runInBackground(fn func(ctx context.Context)) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("portal background task panicked", "panic", r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), backgroundTimeout)
		defer cancel()
		fn(ctx)
	}()
}

// background schedules work through s.run and logs its failure. The log
// carries the operation name and error only: never the address or a token.
func (s *Service) background(op string, work func(ctx context.Context) error) {
	s.run(func(ctx context.Context) {
		if err := work(ctx); err != nil {
			slog.Error("portal account mail failed", "op", op, "err", err)
		}
	})
}

func NewService(b db.Beginner, tokens *Tokens, refreshTTL time.Duration, notifier mail.Notifier, baseURL, siteName string) *Service {
	return &Service{
		b: b, tokens: tokens, refreshTTL: refreshTTL, notifier: notifier,
		baseURL: strings.TrimRight(baseURL, "/"), siteName: siteName, now: time.Now,
		run: runInBackground,
	}
}

var (
	dummyHash     string
	dummyHashOnce sync.Once
)

// checkDummy pays one bcrypt compare so a login for an unknown or unusable
// account takes as long as one for a real account.
func checkDummy(pw string) {
	dummyHashOnce.Do(func() {
		h, err := auth.HashPassword("dummy-password-for-constant-time-compare")
		if err != nil {
			h = "$2a$10$CwTycUXWue0Thq9StjUM0uJ8Q4h2ffTNVJj7Wc0mm8VfM/y8vSFO6"
		}
		dummyHash = h
	})
	auth.CheckPassword(dummyHash, pw)
}

func (s *Service) link(raw string) string { return s.baseURL + "/portal/t/" + raw }

func profileOf(u db.EndUser) Profile {
	return Profile{
		ID: u.ID, Email: u.Email, Name: u.Name,
		Verified: u.EmailVerifiedAt != nil, HasPassword: u.PasswordHash != nil,
	}
}

// UpsertByEmail returns the end user for email (matched case-insensitively),
// creating one if needed. An existing user's name is filled in only when it
// was empty.
func (s *Service) UpsertByEmail(ctx context.Context, q *db.Queries, email, name string) (db.EndUser, error) {
	email = strings.TrimSpace(email)
	name = strings.TrimSpace(name)
	u, err := q.GetEndUserByEmail(ctx, email)
	if err == nil {
		if u.Name == "" && name != "" {
			return q.UpdateEndUserName(ctx, db.UpdateEndUserNameParams{ID: u.ID, Name: name})
		}
		return u, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.EndUser{}, err
	}
	return q.CreateEndUser(ctx, db.CreateEndUserParams{Email: email, Name: name})
}

func (s *Service) issueToken(ctx context.Context, q *db.Queries, u db.EndUser, kind db.ClientTokenKind, ticketID *int64, ttl time.Duration) (string, error) {
	raw, hash, err := NewOneTimeToken()
	if err != nil {
		return "", err
	}
	err = q.CreateClientToken(ctx, db.CreateClientTokenParams{
		EndUserID: u.ID, Kind: kind, TokenHash: hash, TicketID: ticketID, ExpiresAt: s.now().Add(ttl),
	})
	return raw, err
}

// sendLink mails a one-time link. The raw token leaves the service only here,
// inside Vars.Link.
func (s *Service) sendLink(ctx context.Context, q *db.Queries, u db.EndUser, key, raw string, ticketID *int64, vars mail.Vars) error {
	vars.Link = s.link(raw)
	vars.SiteName = s.siteName
	if vars.RequesterName == "" {
		vars.RequesterName = u.Name
	}
	vars.RequesterEmail = u.Email
	return s.notifier.Enqueue(ctx, q, mail.Notification{
		TemplateKey: key, TicketID: ticketID,
		To: []mail.Recipient{{Name: u.Name, Address: u.Email}}, Vars: vars,
	})
}

// mailToken issues a one-time token of kind for u and mails its link.
func (s *Service) mailToken(ctx context.Context, q *db.Queries, u db.EndUser, kind db.ClientTokenKind, ttl time.Duration, key string, ticketID *int64, vars mail.Vars) error {
	raw, err := s.issueToken(ctx, q, u, kind, ticketID, ttl)
	if err != nil {
		return err
	}
	return s.sendLink(ctx, q, u, key, raw, ticketID, vars)
}

// Register schedules the account start in the background and reports nothing:
// it creates (or reuses) the address's end user and mails a confirm link,
// whose session sets the password. An address that already has a password
// gets no mail, so the caller cannot tell the two cases apart.
func (s *Service) Register(in RegisterInput) {
	s.background("register", func(ctx context.Context) error { return s.register(ctx, in) })
}

func (s *Service) register(ctx context.Context, in RegisterInput) error {
	return db.WithTx(ctx, s.b, func(q *db.Queries) error {
		u, err := s.UpsertByEmail(ctx, q, in.Email, in.Name)
		if err != nil {
			return err
		}
		if u.PasswordHash != nil {
			return nil
		}
		return s.mailToken(ctx, q, u, db.ClientTokenKindConfirm, confirmTTL, "client_confirm", nil, mail.Vars{})
	})
}

// Login checks a password for a verified account. Every failure is the same
// ErrUnauthorized.
func (s *Service) Login(ctx context.Context, email, password string) (*Session, error) {
	var sess *Session
	err := db.WithTx(ctx, s.b, func(q *db.Queries) error {
		u, err := q.GetEndUserByEmail(ctx, strings.TrimSpace(email))
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err != nil || u.PasswordHash == nil || u.EmailVerifiedAt == nil {
			checkDummy(password)
			return apperr.ErrUnauthorized
		}
		if !auth.CheckPassword(*u.PasswordHash, password) {
			return apperr.ErrUnauthorized
		}
		sess, err = s.issue(ctx, q, u, nil, "")
		return err
	})
	return sess, err
}

// requestByEmail mails a one-time link to an existing address and silently
// does nothing for an unknown one.
func (s *Service) requestByEmail(ctx context.Context, email string, kind db.ClientTokenKind, ttl time.Duration, key string) error {
	return db.WithTx(ctx, s.b, func(q *db.Queries) error {
		u, err := q.GetEndUserByEmail(ctx, strings.TrimSpace(email))
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		return s.mailToken(ctx, q, u, kind, ttl, key, nil, mail.Vars{})
	})
}

// RequestLink schedules a sign-in link for a known address; unknown addresses get nothing.
func (s *Service) RequestLink(email string) {
	s.background("signin_link", func(ctx context.Context) error {
		return s.requestByEmail(ctx, email, db.ClientTokenKindSignin, signinTTL, "client_signin_link")
	})
}

// RequestReset schedules a password-reset link for a known address; unknown addresses get nothing.
func (s *Service) RequestReset(email string) {
	s.background("reset_link", func(ctx context.Context) error {
		return s.requestByEmail(ctx, email, db.ClientTokenKindReset, resetTTL, "client_reset")
	})
}

// RequestAccess schedules a guest access link for the ticket numbered number
// when email is its requester; otherwise nothing is sent.
func (s *Service) RequestAccess(email, number string) {
	s.background("access_link", func(ctx context.Context) error { return s.requestAccess(ctx, email, number) })
}

// requestAccess mails the access link and, when the ticket has no end user
// yet, claims it for the requester (atomically: a ticket that gained a user
// meanwhile keeps it).
func (s *Service) requestAccess(ctx context.Context, email, number string) error {
	email = strings.TrimSpace(email)
	return db.WithTx(ctx, s.b, func(q *db.Queries) error {
		id, err := q.GetTicketIDByNumberAndEmail(ctx, db.GetTicketIDByNumberAndEmailParams{Number: strings.TrimSpace(number), Lower: email})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		t, err := q.GetTicket(ctx, id)
		if err != nil {
			return err
		}
		u, err := s.UpsertByEmail(ctx, q, email, t.RequesterName)
		if err != nil {
			return err
		}
		if err := q.ClaimTicketUser(ctx, db.ClaimTicketUserParams{ID: id, UserID: &u.ID}); err != nil {
			return err
		}
		vars := mail.Vars{Number: t.Number, Subject: t.Subject, RequesterName: t.RequesterName}
		return s.mailToken(ctx, q, u, db.ClientTokenKindAccess, accessTTL, "client_access_link", &id, vars)
	})
}

// Exchange redeems an emailed one-time token. A confirm token verifies the
// address and, like a reset token, opens a password-setting session (pwr
// claim, no refresh token); an access token opens a guest session for its
// ticket. Unknown, used
// or expired tokens are ErrTokenInvalid.
func (s *Service) Exchange(ctx context.Context, raw string) (*Session, error) {
	var sess *Session
	err := db.WithTx(ctx, s.b, func(q *db.Queries) error {
		tok, err := q.ConsumeClientToken(ctx, HashToken(raw))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrTokenInvalid
		}
		if err != nil {
			return err
		}
		if !tok.ExpiresAt.After(s.now()) {
			return apperr.ErrTokenInvalid
		}
		if tok.Kind == db.ClientTokenKindConfirm {
			if err := q.MarkEndUserVerified(ctx, tok.EndUserID); err != nil {
				return err
			}
		}
		u, err := q.GetEndUser(ctx, tok.EndUserID)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrTokenInvalid
		}
		if err != nil {
			return err
		}
		var ticketID *int64
		if tok.Kind == db.ClientTokenKindAccess {
			ticketID = tok.TicketID
		}
		sess, err = s.issue(ctx, q, u, ticketID, tok.Kind)
		return err
	})
	return sess, err
}

// Refresh rotates a refresh token, keeping its ticket scope.
func (s *Service) Refresh(ctx context.Context, raw string) (*Session, error) {
	var sess *Session
	err := db.WithTx(ctx, s.b, func(q *db.Queries) error {
		rt, err := q.ConsumeClientRefreshToken(ctx, auth.HashRefreshToken(raw))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrUnauthorized
		}
		if err != nil {
			return err
		}
		if !rt.ExpiresAt.After(s.now()) {
			return apperr.ErrUnauthorized
		}
		u, err := q.GetEndUser(ctx, rt.EndUserID)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrUnauthorized
		}
		if err != nil {
			return err
		}
		sess, err = s.issue(ctx, q, u, rt.TicketID, "")
		return err
	})
	return sess, err
}

// Logout revokes raw when it belongs to userID; another user's token is left alone.
func (s *Service) Logout(ctx context.Context, userID int64, raw string) error {
	return db.New(s.b).RevokeClientRefreshToken(ctx, db.RevokeClientRefreshTokenParams{
		TokenHash: auth.HashRefreshToken(raw), EndUserID: userID,
	})
}

func (s *Service) Me(ctx context.Context, userID int64) (*Profile, error) {
	u, err := db.New(s.b).GetEndUser(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p := profileOf(u)
	return &p, nil
}

func (s *Service) UpdateName(ctx context.Context, userID int64, name string) (*Profile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, apperr.Validation("name", "required")
	}
	u, err := db.New(s.b).UpdateEndUserName(ctx, db.UpdateEndUserNameParams{ID: userID, Name: name})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p := profileOf(u)
	return &p, nil
}

// SetPassword sets a new password and marks the address verified. The current
// password is required unless the user has none yet or the session was opened
// from a reset link. Every refresh token the user holds is revoked, including
// the calling session's own, and the caller gets a fresh full session (access
// and refresh token, no pwr claim) to continue with.
func (s *Service) SetPassword(ctx context.Context, p Principal, in PasswordInput) (*Session, error) {
	var sess *Session
	err := db.WithTx(ctx, s.b, func(q *db.Queries) error {
		u, err := q.GetEndUser(ctx, p.UserID)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrNotFound
		}
		if err != nil {
			return err
		}
		if u.PasswordHash != nil && !p.PasswordReset {
			if in.CurrentPassword == "" {
				return apperr.Validation("current_password", "required")
			}
			if !auth.CheckPassword(*u.PasswordHash, in.CurrentPassword) {
				return apperr.Validation("current_password", "incorrect")
			}
		}
		hash, err := auth.HashPassword(in.Password)
		if err != nil {
			return err
		}
		if err := q.SetEndUserPassword(ctx, db.SetEndUserPasswordParams{ID: u.ID, PasswordHash: &hash}); err != nil {
			return err
		}
		if err := q.RevokeClientRefreshTokensForUser(ctx, u.ID); err != nil {
			return err
		}
		if u, err = q.GetEndUser(ctx, u.ID); err != nil {
			return err
		}
		sess, err = s.issue(ctx, q, u, nil, "")
		return err
	})
	if err != nil {
		return nil, err
	}
	return sess, nil
}

// LoadClient loads the principal for RequireUser; an unknown user is ErrUnauthorized.
func (s *Service) LoadClient(ctx context.Context, userID int64) (Principal, error) {
	u, err := db.New(s.b).GetEndUser(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, apperr.ErrUnauthorized
	}
	if err != nil {
		return Principal{}, err
	}
	return Principal{UserID: u.ID, Email: u.Email, Verified: u.EmailVerifiedAt != nil}, nil
}

// issue opens a session for u. A confirm or reset session gets an access token
// with the pwr claim and no refresh token; every other session gets a refresh token
// with the same ticket scope as its access token.
func (s *Service) issue(ctx context.Context, q *db.Queries, u db.EndUser, ticketID *int64, kind db.ClientTokenKind) (*Session, error) {
	reset := kind == db.ClientTokenKindReset || kind == db.ClientTokenKindConfirm
	access, _, err := s.tokens.IssueAccess(u.ID, ticketID, reset)
	if err != nil {
		return nil, err
	}
	sess := &Session{
		AccessToken: access, ExpiresIn: int(s.tokens.AccessTTL().Seconds()),
		User: profileOf(u), TicketID: ticketID, Kind: string(kind),
	}
	if reset {
		return sess, nil
	}
	raw, hash, err := auth.NewRefreshToken()
	if err != nil {
		return nil, err
	}
	if err := q.CreateClientRefreshToken(ctx, db.CreateClientRefreshTokenParams{
		TokenHash: hash, EndUserID: u.ID, TicketID: ticketID, ExpiresAt: s.now().Add(s.refreshTTL),
	}); err != nil {
		return nil, err
	}
	sess.RefreshToken = raw
	return sess, nil
}
