// Package dept manages departments.
package dept

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/jackc/pgx/v5"
)

type Department struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	IsPublic  bool      `json:"is_public"`
	ManagerID *int64    `json:"manager_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateInput struct {
	Name      string `json:"name" binding:"required,max=128"`
	IsPublic  *bool  `json:"is_public"`
	ManagerID *int64 `json:"manager_id"`
}

type UpdateInput struct {
	Name      *string `json:"name" binding:"omitempty,min=1,max=128"`
	IsPublic  *bool   `json:"is_public"`
	ManagerID *int64  `json:"manager_id"`
}

type Service interface {
	List(ctx context.Context) ([]Department, error)
	Get(ctx context.Context, id int64) (*Department, error)
	Create(ctx context.Context, in CreateInput) (*Department, error)
	Update(ctx context.Context, id int64, in UpdateInput) (*Department, error)
	Delete(ctx context.Context, id int64) error
}

type service struct{ db db.Beginner }

func NewService(b db.Beginner) Service { return &service{db: b} }

func fromRow(d db.Department) Department {
	return Department{ID: d.ID, Name: d.Name, IsPublic: d.IsPublic, ManagerID: d.ManagerID,
		CreatedAt: d.CreatedAt.UTC(), UpdatedAt: d.UpdatedAt.UTC()}
}

func (s *service) List(ctx context.Context) ([]Department, error) {
	rows, err := db.New(s.db).ListDepartments(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Department, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromRow(r))
	}
	return out, nil
}

func (s *service) Get(ctx context.Context, id int64) (*Department, error) {
	d, err := db.New(s.db).GetDepartment(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("department %d: %w", id, apperr.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	out := fromRow(d)
	return &out, nil
}

func (s *service) Create(ctx context.Context, in CreateInput) (*Department, error) {
	var out Department
	err := db.WithTx(ctx, s.db, func(q *db.Queries) error {
		if err := checkManager(ctx, q, in.ManagerID); err != nil {
			return err
		}
		isPublic := true
		if in.IsPublic != nil {
			isPublic = *in.IsPublic
		}
		d, err := q.CreateDepartment(ctx, db.CreateDepartmentParams{Name: in.Name, IsPublic: isPublic, ManagerID: in.ManagerID})
		if db.IsUniqueViolation(err) {
			return fmt.Errorf("%w: department name already exists", apperr.ErrConflict)
		}
		if err != nil {
			return err
		}
		out = fromRow(d)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *service) Update(ctx context.Context, id int64, in UpdateInput) (*Department, error) {
	var out Department
	err := db.WithTx(ctx, s.db, func(q *db.Queries) error {
		if err := checkManager(ctx, q, in.ManagerID); err != nil {
			return err
		}
		d, err := q.UpdateDepartment(ctx, db.UpdateDepartmentParams{ID: id, Name: in.Name, IsPublic: in.IsPublic, ManagerID: in.ManagerID})
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("department %d: %w", id, apperr.ErrNotFound)
		}
		if db.IsUniqueViolation(err) {
			return fmt.Errorf("%w: department name already exists", apperr.ErrConflict)
		}
		if err != nil {
			return err
		}
		out = fromRow(d)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *service) Delete(ctx context.Context, id int64) error {
	return db.WithTx(ctx, s.db, func(q *db.Queries) error {
		refs, err := q.CountDepartmentReferences(ctx, id)
		if err != nil {
			return err
		}
		if refs > 0 {
			return fmt.Errorf("%w: department is referenced by tickets, staff or topics", apperr.ErrConflict)
		}
		n, err := q.DeleteDepartment(ctx, id)
		// Belt and braces beside the reference count above: a foreign key
		// violation (e.g. a reference added between the count and the
		// delete) maps to the same conflict instead of a raw 500.
		if db.IsForeignKeyViolation(err) {
			return fmt.Errorf("%w: department is referenced by tickets, staff or topics", apperr.ErrConflict)
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("department %d: %w", id, apperr.ErrNotFound)
		}
		return nil
	})
}

func checkManager(ctx context.Context, q *db.Queries, id *int64) error {
	if id == nil {
		return nil
	}
	_, err := q.GetStaff(ctx, *id)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.Validation("manager_id", "unknown staff")
	}
	return err
}
