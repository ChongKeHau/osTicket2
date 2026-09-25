package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/jackc/pgx/v5"
)

type StaffProfile struct {
	ID            int64   `json:"id"`
	Username      string  `json:"username"`
	Email         string  `json:"email"`
	FirstName     string  `json:"first_name"`
	LastName      string  `json:"last_name"`
	IsAdmin       bool    `json:"is_admin"`
	DepartmentIDs []int64 `json:"department_ids"`
}

type Session struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	ExpiresIn    int          `json:"expires_in"`
	Staff        StaffProfile `json:"staff"`
}

type Service struct {
	db         db.Beginner
	tokens     *Tokens
	refreshTTL time.Duration
	now        func() time.Time
}

func NewService(b db.Beginner, tokens *Tokens, refreshTTL time.Duration) *Service {
	return &Service{db: b, tokens: tokens, refreshTTL: refreshTTL, now: time.Now}
}

func (s *Service) Login(ctx context.Context, username, password string) (*Session, error) {
	q := db.New(s.db)
	st, err := q.GetStaffByUsername(ctx, username)
	if errors.Is(err, pgx.ErrNoRows) {
		// Always pay the bcrypt compare, even for an unknown username, so
		// response time doesn't reveal whether the username exists.
		CheckPassword(ensureDummyHash(), password)
		return nil, apperr.ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	ok := CheckPassword(st.PasswordHash, password)
	if !ok || !st.IsActive {
		return nil, apperr.ErrUnauthorized
	}
	return s.issue(ctx, q, st)
}

func (s *Service) Refresh(ctx context.Context, raw string) (*Session, error) {
	var sess *Session
	err := db.WithTx(ctx, s.db, func(q *db.Queries) error {
		rt, err := q.ConsumeRefreshToken(ctx, HashRefreshToken(raw))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrUnauthorized
		}
		if err != nil {
			return err
		}
		if !rt.ExpiresAt.After(s.now()) {
			return apperr.ErrUnauthorized
		}
		st, err := q.GetStaff(ctx, rt.StaffID)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrUnauthorized
		}
		if err != nil {
			return err
		}
		if !st.IsActive {
			return apperr.ErrUnauthorized
		}
		sess, err = s.issue(ctx, q, st)
		return err
	})
	return sess, err
}

// Logout revokes raw, scoped to staffID: it is a no-op (not an error) if raw
// belongs to a different staff id, so one caller can never revoke someone
// else's session.
func (s *Service) Logout(ctx context.Context, staffID int64, raw string) error {
	return db.New(s.db).RevokeRefreshToken(ctx, db.RevokeRefreshTokenParams{
		TokenHash: HashRefreshToken(raw), StaffID: staffID,
	})
}

func (s *Service) Me(ctx context.Context, staffID int64) (*StaffProfile, error) {
	q := db.New(s.db)
	st, err := q.GetStaff(ctx, staffID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p, err := s.profile(ctx, q, st)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Service) LoadPrincipal(ctx context.Context, staffID int64) (Principal, error) {
	q := db.New(s.db)
	st, err := q.GetStaff(ctx, staffID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, apperr.ErrUnauthorized
	}
	if err != nil {
		return Principal{}, err
	}
	if !st.IsActive {
		return Principal{}, apperr.ErrUnauthorized
	}
	ids, err := deptIDs(ctx, q, st)
	if err != nil {
		return Principal{}, err
	}
	return Principal{StaffID: st.ID, IsAdmin: st.IsAdmin, DeptIDs: ids}, nil
}

func (s *Service) issue(ctx context.Context, q *db.Queries, st db.Staff) (*Session, error) {
	access, _, err := s.tokens.IssueAccess(st.ID, st.IsAdmin)
	if err != nil {
		return nil, err
	}
	raw, hash, err := NewRefreshToken()
	if err != nil {
		return nil, err
	}
	if err := q.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
		TokenHash: hash, StaffID: st.ID, ExpiresAt: s.now().Add(s.refreshTTL),
	}); err != nil {
		return nil, err
	}
	profile, err := s.profile(ctx, q, st)
	if err != nil {
		return nil, err
	}
	return &Session{
		AccessToken: access, RefreshToken: raw,
		ExpiresIn: int(s.tokens.AccessTTL().Seconds()), Staff: profile,
	}, nil
}

func (s *Service) profile(ctx context.Context, q *db.Queries, st db.Staff) (StaffProfile, error) {
	ids, err := deptIDs(ctx, q, st)
	if err != nil {
		return StaffProfile{}, err
	}
	return StaffProfile{
		ID: st.ID, Username: st.Username, Email: st.Email, FirstName: st.FirstName,
		LastName: st.LastName, IsAdmin: st.IsAdmin, DepartmentIDs: ids,
	}, nil
}

// deptIDs returns the primary department followed by extra memberships, deduplicated.
func deptIDs(ctx context.Context, q *db.Queries, st db.Staff) ([]int64, error) {
	extra, err := q.ListStaffDepartmentIDs(ctx, st.ID)
	if err != nil {
		return nil, fmt.Errorf("staff departments: %w", err)
	}
	ids := []int64{st.PrimaryDeptID}
	for _, id := range extra {
		if id != st.PrimaryDeptID {
			ids = append(ids, id)
		}
	}
	return ids, nil
}
