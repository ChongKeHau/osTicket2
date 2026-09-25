// Package topic manages help topics.
package topic

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/jackc/pgx/v5"
)

type Topic struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	DeptID     *int64    `json:"dept_id"`
	PriorityID *int64    `json:"priority_id"`
	IsActive   bool      `json:"is_active"`
	SortOrder  int32     `json:"sort_order"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type CreateInput struct {
	Name       string `json:"name" binding:"required,max=128"`
	DeptID     *int64 `json:"dept_id"`
	PriorityID *int64 `json:"priority_id"`
	IsActive   *bool  `json:"is_active"`
	SortOrder  *int32 `json:"sort_order"`
}

type UpdateInput struct {
	Name       *string `json:"name" binding:"omitempty,min=1,max=128"`
	DeptID     *int64  `json:"dept_id"`
	PriorityID *int64  `json:"priority_id"`
	IsActive   *bool   `json:"is_active"`
	SortOrder  *int32  `json:"sort_order"`
}

type Service interface {
	List(ctx context.Context) ([]Topic, error)
	Get(ctx context.Context, id int64) (*Topic, error)
	Create(ctx context.Context, in CreateInput) (*Topic, error)
	Update(ctx context.Context, id int64, in UpdateInput) (*Topic, error)
	Delete(ctx context.Context, id int64) error
}

type service struct{ db db.Beginner }

func NewService(b db.Beginner) Service { return &service{db: b} }

func fromRow(r db.HelpTopic) Topic {
	return Topic{ID: r.ID, Name: r.Name, DeptID: r.DeptID, PriorityID: r.PriorityID, IsActive: r.IsActive,
		SortOrder: r.SortOrder, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC()}
}

func (s *service) List(ctx context.Context) ([]Topic, error) {
	rows, err := db.New(s.db).ListTopics(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Topic, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromRow(r))
	}
	return out, nil
}

func (s *service) Get(ctx context.Context, id int64) (*Topic, error) {
	r, err := db.New(s.db).GetTopic(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("topic %d: %w", id, apperr.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	out := fromRow(r)
	return &out, nil
}

func (s *service) Create(ctx context.Context, in CreateInput) (*Topic, error) {
	q := db.New(s.db)
	if err := checkRefs(ctx, q, in.DeptID, in.PriorityID); err != nil {
		return nil, err
	}
	isActive, sortOrder := true, int32(0)
	if in.IsActive != nil {
		isActive = *in.IsActive
	}
	if in.SortOrder != nil {
		sortOrder = *in.SortOrder
	}
	r, err := q.CreateTopic(ctx, db.CreateTopicParams{Name: in.Name, DeptID: in.DeptID, PriorityID: in.PriorityID, IsActive: isActive, SortOrder: sortOrder})
	if err != nil {
		return nil, err
	}
	out := fromRow(r)
	return &out, nil
}

func (s *service) Update(ctx context.Context, id int64, in UpdateInput) (*Topic, error) {
	q := db.New(s.db)
	if err := checkRefs(ctx, q, in.DeptID, in.PriorityID); err != nil {
		return nil, err
	}
	r, err := q.UpdateTopic(ctx, db.UpdateTopicParams{ID: id, Name: in.Name, DeptID: in.DeptID, PriorityID: in.PriorityID, IsActive: in.IsActive, SortOrder: in.SortOrder})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("topic %d: %w", id, apperr.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	out := fromRow(r)
	return &out, nil
}

func (s *service) Delete(ctx context.Context, id int64) error {
	return db.WithTx(ctx, s.db, func(q *db.Queries) error {
		refs, err := q.CountTopicReferences(ctx, &id)
		if err != nil {
			return err
		}
		if refs > 0 {
			return fmt.Errorf("%w: topic is referenced by tickets", apperr.ErrConflict)
		}
		n, err := q.DeleteTopic(ctx, id)
		// Belt and braces beside the reference count above: a foreign key
		// violation (e.g. a reference added between the count and the
		// delete) maps to the same conflict instead of a raw 500.
		if db.IsForeignKeyViolation(err) {
			return fmt.Errorf("%w: topic is referenced by tickets", apperr.ErrConflict)
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("topic %d: %w", id, apperr.ErrNotFound)
		}
		return nil
	})
}

func checkRefs(ctx context.Context, q *db.Queries, deptID, priorityID *int64) error {
	fields := map[string]string{}
	if deptID != nil {
		if _, err := q.GetDepartment(ctx, *deptID); errors.Is(err, pgx.ErrNoRows) {
			fields["dept_id"] = "unknown department"
		} else if err != nil {
			return err
		}
	}
	if priorityID != nil {
		if _, err := q.GetPriority(ctx, *priorityID); errors.Is(err, pgx.ErrNoRows) {
			fields["priority_id"] = "unknown priority"
		} else if err != nil {
			return err
		}
	}
	if len(fields) > 0 {
		return &apperr.ValidationError{Fields: fields}
	}
	return nil
}
