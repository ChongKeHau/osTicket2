package importer

import (
	"context"
	"errors"
	"testing"

	"github.com/grandpine/ticket-api/internal/db/testutil"
)

func TestSinkInsertPreservesIDsInBatches(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	s := NewSink(tx, 2, false)
	err := s.Step(ctx, func(w *Writer) error {
		return w.Insert(ctx, "department", []string{"id", "name", "is_public"}, [][]any{
			{int64(50), "Fifty", true}, {int64(51), "Fifty-one", false}, {int64(52), "Fifty-two", true},
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM department WHERE id IN (50,51,52)").Scan(&n); err != nil || n != 3 {
		t.Fatalf("count = %d, %v", n, err)
	}
	var name string
	if err := tx.QueryRow(ctx, "SELECT name FROM department WHERE id = 51").Scan(&name); err != nil || name != "Fifty-one" {
		t.Fatalf("name = %q, %v", name, err)
	}
}

func TestSinkStepRollsBackOnError(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	s := NewSink(tx, 500, false)
	boom := errors.New("boom")
	err := s.Step(ctx, func(w *Writer) error {
		if err := w.Insert(ctx, "department", []string{"id", "name"}, [][]any{{int64(60), "Sixty"}}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	var n int
	_ = tx.QueryRow(ctx, "SELECT count(*) FROM department WHERE id = 60").Scan(&n)
	if n != 0 {
		t.Fatal("row should have been rolled back")
	}
}

func TestSinkDryRunWritesNothing(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	s := NewSink(tx, 500, true)
	if err := s.Step(ctx, func(w *Writer) error {
		return w.Insert(ctx, "department", []string{"id", "name"}, [][]any{{int64(70), "Seventy"}})
	}); err != nil {
		t.Fatal(err)
	}
	// A later step sees the earlier step's row, so foreign keys resolve during a dry run.
	if err := s.Step(ctx, func(w *Writer) error {
		var n int
		if err := w.QueryRow(ctx, "SELECT count(*) FROM department WHERE id = 70").Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			return errors.New("row from the previous step not visible")
		}
		return w.Insert(ctx, "staff", []string{"id", "username", "email", "password_hash", "primary_dept_id"}, [][]any{{int64(70), "dry", "dry@example.test", "h", int64(70)}})
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = tx.QueryRow(ctx, "SELECT count(*) FROM department WHERE id = 70").Scan(&n)
	if n != 0 {
		t.Fatal("dry run must not commit")
	}
	_ = tx.QueryRow(ctx, "SELECT count(*) FROM staff WHERE id = 70").Scan(&n)
	if n != 0 {
		t.Fatal("dry run must not commit staff either")
	}
}

func TestPreflightRefusesNonEmpty(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	s := NewSink(tx, 500, false)
	if err := s.Preflight(ctx); err != nil {
		t.Fatalf("fresh seed database should pass: %v", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO staff (username, email, password_hash, primary_dept_id) VALUES ('x','x@example.test','h',(SELECT id FROM department LIMIT 1))"); err != nil {
		t.Fatal(err)
	}
	err := s.Preflight(ctx)
	if !errors.Is(err, ErrTargetNotEmpty) || err.Error() == ErrTargetNotEmpty.Error() {
		t.Fatalf("want wrapped ErrTargetNotEmpty naming the table, got %v", err)
	}
}

func TestPreflightRefusesExtraDepartment(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	if _, err := tx.Exec(ctx, "INSERT INTO department (name) VALUES ('Extra')"); err != nil {
		t.Fatal(err)
	}
	if err := NewSink(tx, 500, false).Preflight(ctx); !errors.Is(err, ErrTargetNotEmpty) {
		t.Fatalf("got %v", err)
	}
}

// TestResetSequences calls ResetSequences itself, after inserting a row with
// a known id in the same transaction, so the expected next id (901) holds
// regardless of what earlier tests left the shared sequences at; it does not
// depend on run ordering with other tests.
func TestResetSequences(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	s := NewSink(tx, 500, false)
	if err := s.Step(ctx, func(w *Writer) error {
		return w.Insert(ctx, "department", []string{"id", "name"}, [][]any{{int64(900), "Nine hundred"}})
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetSequences(ctx); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := tx.QueryRow(ctx, "INSERT INTO department (name) VALUES ('Next') RETURNING id").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if id != 901 {
		t.Fatalf("next id = %d, want 901", id)
	}
	// An empty table starts at 1 after the reset.
	if err := tx.QueryRow(ctx, "INSERT INTO file (key, name, mime, size, sha256) VALUES ('abcdef', 'a', 'text/plain', 1, 'x') RETURNING id").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if id != 1 {
		t.Fatalf("file id = %d, want 1", id)
	}
}
