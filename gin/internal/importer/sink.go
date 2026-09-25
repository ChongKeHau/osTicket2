package importer

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/grandpine/ticket-api/internal/db"
	"github.com/jackc/pgx/v5"
)

// ErrTargetNotEmpty is returned by Preflight when the target already has data.
var ErrTargetNotEmpty = errors.New("target database is not empty")

// identityTables are reset by ResetSequences after ids were inserted explicitly.
var identityTables = []string{
	"department", "staff", "refresh_token", "ticket_priority", "ticket_status",
	"help_topic", "ticket", "thread_entry", "ticket_event", "file",
}

// Sink writes to the target Postgres database, one transaction per Step.
// In dry-run mode the steps run as savepoints inside one outer transaction
// that Close rolls back, so nothing is committed but later steps still see
// earlier rows.
type Sink struct {
	db     db.Beginner
	batch  int
	dryRun bool
	outer  pgx.Tx
}

// NewSink wraps a pool or transaction. batch is the number of rows per INSERT.
func NewSink(b db.Beginner, batch int, dryRun bool) *Sink {
	if batch < 1 {
		batch = 1
	}
	return &Sink{db: b, batch: batch, dryRun: dryRun}
}

// Writer is the per-step handle. It is only valid inside Step.
type Writer struct {
	tx    pgx.Tx
	batch int
}

// Step runs fn in a transaction and commits when fn returns nil. In dry-run
// mode the transaction is a savepoint of the outer transaction instead.
func (s *Sink) Step(ctx context.Context, fn func(w *Writer) error) error {
	var parent db.Beginner = s.db
	if s.dryRun {
		if s.outer == nil {
			outer, err := s.db.Begin(ctx)
			if err != nil {
				return fmt.Errorf("begin dry run: %w", err)
			}
			s.outer = outer
		}
		parent = s.outer
	}
	tx, err := parent.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	if err := fn(&Writer{tx: tx, batch: s.batch}); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// Close rolls back the dry-run outer transaction. It is a no-op otherwise.
func (s *Sink) Close(ctx context.Context) error {
	if s.outer == nil {
		return nil
	}
	err := s.outer.Rollback(ctx)
	s.outer = nil
	return err
}

// Insert writes rows with explicit ids in batches.
func (w *Writer) Insert(ctx context.Context, table string, cols []string, rows [][]any) error {
	for start := 0; start < len(rows); start += w.batch {
		end := start + w.batch
		if end > len(rows) {
			end = len(rows)
		}
		chunk := rows[start:end]
		var sb strings.Builder
		fmt.Fprintf(&sb, "INSERT INTO %s (%s) OVERRIDING SYSTEM VALUE VALUES ", table, strings.Join(cols, ", "))
		args := make([]any, 0, len(chunk)*len(cols))
		for i, row := range chunk {
			if len(row) != len(cols) {
				return fmt.Errorf("insert %s: row %d has %d values for %d columns", table, start+i, len(row), len(cols))
			}
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString("(")
			for j := range row {
				if j > 0 {
					sb.WriteString(", ")
				}
				fmt.Fprintf(&sb, "$%d", len(args)+1)
				args = append(args, row[j])
			}
			sb.WriteString(")")
		}
		if _, err := w.tx.Exec(ctx, sb.String(), args...); err != nil {
			return fmt.Errorf("insert %s: %w", table, err)
		}
	}
	return nil
}

func (w *Writer) Exec(ctx context.Context, sql string, args ...any) error {
	_, err := w.tx.Exec(ctx, sql, args...)
	return err
}

func (w *Writer) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return w.tx.QueryRow(ctx, sql, args...)
}

func (w *Writer) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return w.tx.Query(ctx, sql, args...)
}

// Preflight checks that the target holds only the seed rows.
func (s *Sink) Preflight(ctx context.Context) error {
	checks := []struct {
		table string
		want  int
	}{
		{"staff", 0}, {"ticket", 0}, {"thread_entry", 0}, {"file", 0}, {"department", 1}, {"help_topic", 1},
	}
	for _, c := range checks {
		var n int
		if err := s.db.QueryRow(ctx, "SELECT count(*) FROM "+c.table).Scan(&n); err != nil {
			return fmt.Errorf("preflight %s: %w", c.table, err)
		}
		if n != c.want {
			return fmt.Errorf("%w: %s has %d rows (expected %d)", ErrTargetNotEmpty, c.table, n, c.want)
		}
	}
	return nil
}

// ResetSequences moves every identity sequence past the highest inserted id.
//
// setval is not transactional: even inside a transaction that later rolls
// back, the sequence advance survives. A test that calls this (directly or
// via Run) therefore permanently moves the shared test database's identity
// sequences forward, regardless of the rollback. Tests must never assert an
// absolute generated id after a call that reaches here (assert relative to
// the highest id seen instead) and must not run with t.Parallel() against
// the shared database, since concurrent tests would race on the same
// sequences.
func (s *Sink) ResetSequences(ctx context.Context) error {
	if s.dryRun {
		return nil
	}
	for _, t := range identityTables {
		q := fmt.Sprintf("SELECT setval(pg_get_serial_sequence('%s', 'id'), COALESCE(MAX(id), 1), MAX(id) IS NOT NULL) FROM %s", t, t)
		if _, err := s.db.Exec(ctx, q); err != nil {
			return fmt.Errorf("reset sequence %s: %w", t, err)
		}
	}
	return nil
}
