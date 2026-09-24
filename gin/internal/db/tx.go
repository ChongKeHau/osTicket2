package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Beginner is satisfied by *pgxpool.Pool and by pgx.Tx (nested Begin creates a
// savepoint), which lets tests wrap a service in a rolled-back transaction.
type Beginner interface {
	DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// DB exposes the underlying connection for tests and raw statements.
func (q *Queries) DB() DBTX { return q.db }

// WithTx runs fn inside a transaction and commits if fn returns nil.
func WithTx(ctx context.Context, b Beginner, fn func(q *Queries) error) error {
	tx, err := b.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	if err := fn(New(tx)); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
