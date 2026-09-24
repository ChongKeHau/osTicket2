// Package staff manages agent accounts.
package staff

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/jackc/pgx/v5"
)

type Staff struct {
	ID            int64     `json:"id"`
	Username      string    `json:"username"`
	Email         string    `json:"email"`
	FirstName     string    `json:"first_name"`
	LastName      string    `json:"last_name"`
	IsAdmin       bool      `json:"is_admin"`
	IsActive      bool      `json:"is_active"`
	PrimaryDeptID int64     `json:"primary_dept_id"`
	DepartmentIDs []int64   `json:"department_ids"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type CreateInput struct {
	Username      string  `json:"username" binding:"required,max=64"`
	Email         string  `json:"email" binding:"required,email,max=255"`
	Password      string  `json:"password" binding:"required"`
	FirstName     string  `json:"first_name" binding:"max=64"`
	LastName      string  `json:"last_name" binding:"max=64"`
	IsAdmin       bool    `json:"is_admin"`
	PrimaryDeptID int64   `json:"primary_dept_id" binding:"required"`
	DepartmentIDs []int64 `json:"department_ids"`
}

type UpdateInput struct {
	Email         *string  `json:"email" binding:"omitempty,email,max=255"`
	FirstName     *string  `json:"first_name" binding:"omitempty,max=64"`
	LastName      *string  `json:"last_name" binding:"omitempty,max=64"`
	IsAdmin       *bool    `json:"is_admin"`
	IsActive      *bool    `json:"is_active"`
	PrimaryDeptID *int64   `json:"primary_dept_id"`
	DepartmentIDs *[]int64 `json:"department_ids"`
}

type Service interface {
	List(ctx context.Context) ([]Staff, error)
	Get(ctx context.Context, id int64) (*Staff, error)
	Create(ctx context.Context, in CreateInput) (*Staff, error)
	Update(ctx context.Context, id int64, in UpdateInput) (*Staff, error)
	SetPassword(ctx context.Context, id int64, password string) error
}

type service struct{ db db.Beginner }

func NewService(b db.Beginner) Service { return &service{db: b} }

func (s *service) List(ctx context.Context) ([]Staff, error) {
	q := db.New(s.db)
	rows, err := q.ListStaff(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Staff, 0, len(rows))
	for _, r := range rows {
		st, err := build(ctx, q, r)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, nil
}

func (s *service) Get(ctx context.Context, id int64) (*Staff, error) {
	q := db.New(s.db)
	r, err := q.GetStaff(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("staff %d: %w", id, apperr.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	st, err := build(ctx, q, r)
	if err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *service) Create(ctx context.Context, in CreateInput) (*Staff, error) {
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	var out Staff
	err = db.WithTx(ctx, s.db, func(q *db.Queries) error {
		if err := checkDepts(ctx, q, &in.PrimaryDeptID, in.DepartmentIDs); err != nil {
			return err
		}
		r, err := q.CreateStaff(ctx, db.CreateStaffParams{
			Username: in.Username, Email: in.Email, PasswordHash: hash, FirstName: in.FirstName,
			LastName: in.LastName, IsAdmin: in.IsAdmin, IsActive: true, PrimaryDeptID: in.PrimaryDeptID,
		})
		if db.IsUniqueViolation(err) {
			return fmt.Errorf("%w: username or email already exists", apperr.ErrConflict)
		}
		if err != nil {
			return err
		}
		if err := replaceDepts(ctx, q, r.ID, in.DepartmentIDs); err != nil {
			return err
		}
		out, err = build(ctx, q, r)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *service) Update(ctx context.Context, id int64, in UpdateInput) (*Staff, error) {
	var out Staff
	err := db.WithTx(ctx, s.db, func(q *db.Queries) error {
		var extra []int64
		if in.DepartmentIDs != nil {
			extra = *in.DepartmentIDs
		}
		if err := checkDepts(ctx, q, in.PrimaryDeptID, extra); err != nil {
			return err
		}
		r, err := q.UpdateStaff(ctx, db.UpdateStaffParams{
			ID: id, Email: in.Email, FirstName: in.FirstName, LastName: in.LastName,
			IsAdmin: in.IsAdmin, IsActive: in.IsActive, PrimaryDeptID: in.PrimaryDeptID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("staff %d: %w", id, apperr.ErrNotFound)
		}
		if db.IsUniqueViolation(err) {
			return fmt.Errorf("%w: email already exists", apperr.ErrConflict)
		}
		if err != nil {
			return err
		}
		if in.DepartmentIDs != nil {
			if err := replaceDepts(ctx, q, id, *in.DepartmentIDs); err != nil {
				return err
			}
		}
		if in.IsActive != nil && !*in.IsActive {
			if err := q.RevokeStaffRefreshTokens(ctx, id); err != nil {
				return err
			}
		}
		out, err = build(ctx, q, r)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *service) SetPassword(ctx context.Context, id int64, password string) error {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	return db.WithTx(ctx, s.db, func(q *db.Queries) error {
		n, err := q.SetStaffPassword(ctx, db.SetStaffPasswordParams{ID: id, PasswordHash: hash})
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("staff %d: %w", id, apperr.ErrNotFound)
		}
		// A password reset must invalidate any refresh tokens issued before
		// it, so a stolen refresh token stops rotating once the password is
		// changed.
		return q.RevokeStaffRefreshTokens(ctx, id)
	})
}

func build(ctx context.Context, q *db.Queries, r db.Staff) (Staff, error) {
	extra, err := q.ListStaffDepartmentIDs(ctx, r.ID)
	if err != nil {
		return Staff{}, err
	}
	ids := []int64{r.PrimaryDeptID}
	for _, id := range extra {
		if id != r.PrimaryDeptID {
			ids = append(ids, id)
		}
	}
	return Staff{
		ID: r.ID, Username: r.Username, Email: r.Email, FirstName: r.FirstName, LastName: r.LastName,
		IsAdmin: r.IsAdmin, IsActive: r.IsActive, PrimaryDeptID: r.PrimaryDeptID, DepartmentIDs: ids,
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}, nil
}

func checkDepts(ctx context.Context, q *db.Queries, primary *int64, extra []int64) error {
	if primary != nil {
		if _, err := q.GetDepartment(ctx, *primary); errors.Is(err, pgx.ErrNoRows) {
			return apperr.Validation("primary_dept_id", "unknown department")
		} else if err != nil {
			return err
		}
	}
	for _, id := range extra {
		if _, err := q.GetDepartment(ctx, id); errors.Is(err, pgx.ErrNoRows) {
			return apperr.Validation("department_ids", fmt.Sprintf("unknown department %d", id))
		} else if err != nil {
			return err
		}
	}
	return nil
}

func replaceDepts(ctx context.Context, q *db.Queries, staffID int64, ids []int64) error {
	if err := q.DeleteStaffDepartments(ctx, staffID); err != nil {
		return err
	}
	for _, id := range ids {
		if err := q.AddStaffDepartment(ctx, db.AddStaffDepartmentParams{StaffID: staffID, DeptID: id}); err != nil {
			return err
		}
	}
	return nil
}
