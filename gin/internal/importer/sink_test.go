package importer

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	if _, err := tx.Exec(ctx, `INSERT INTO ticket (id, number, subject, status_id, dept_id, priority_id, requester_email)
		OVERRIDING SYSTEM VALUE VALUES (900, '000777', 's', (SELECT id FROM ticket_status LIMIT 1), (SELECT id FROM department LIMIT 1), (SELECT id FROM ticket_priority LIMIT 1), 'r@example.test')`); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetSequences(ctx); err != nil {
		t.Fatal(err)
	}
	// The ticket-number sequence follows the highest numeric ticket number.
	var next int64
	if err := tx.QueryRow(ctx, "SELECT nextval('ticket_number_seq')").Scan(&next); err != nil {
		t.Fatal(err)
	}
	if next != 778 {
		t.Fatalf("next ticket number = %d, want 778", next)
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

func TestBatcherFlushesEveryBatch(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	s := NewSink(tx, 2, false)
	err := s.Step(ctx, func(w *Writer) error {
		b := w.NewBatcher("department", []string{"id", "name"})
		count := func() int {
			var n int
			if err := w.QueryRow(ctx, "SELECT count(*) FROM department WHERE id BETWEEN 80 AND 84").Scan(&n); err != nil {
				t.Fatal(err)
			}
			return n
		}
		for i := int64(80); i < 84; i++ {
			if err := b.Add(ctx, []any{i, fmt.Sprintf("Dept %d", i)}); err != nil {
				return err
			}
		}
		// Two full batches were flushed before the fifth row arrives.
		if n := count(); n != 4 {
			t.Fatalf("rows before fifth Add = %d, want 4", n)
		}
		if err := b.Add(ctx, []any{int64(84), "Dept 84"}); err != nil {
			return err
		}
		if n := count(); n != 4 {
			t.Fatalf("rows after fifth Add = %d, want 4 (held until Flush)", n)
		}
		if err := b.Flush(ctx); err != nil {
			return err
		}
		if n := count(); n != 5 {
			t.Fatalf("rows after Flush = %d, want 5", n)
		}
		// Flushing an empty batcher is a no-op.
		return b.Flush(ctx)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestChunkSizeCapsParameters(t *testing.T) {
	cases := []struct{ batch, cols, want int }{
		{500, 3, 500},
		{1000000, 3, 21845},
		{100000, 18, 3640},
		{2, 65536, 1},
	}
	for _, c := range cases {
		if got := chunkSize(c.batch, c.cols); got != c.want {
			t.Errorf("chunkSize(%d, %d) = %d, want %d", c.batch, c.cols, got, c.want)
		}
	}
}

func TestSinkInsertHugeBatch(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	s := NewSink(tx, 1_000_000, false)
	if err := s.Step(ctx, func(w *Writer) error {
		return w.Insert(ctx, "department", []string{"id", "name", "is_public"}, [][]any{{int64(95), "Ninety-five", true}, {int64(96), "Ninety-six", false}})
	}); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM department WHERE id IN (95, 96)").Scan(&n); err != nil || n != 2 {
		t.Fatalf("count = %d, %v", n, err)
	}
}

func TestSinkInsertErrorNamesRows(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	s := NewSink(tx, 2, false)
	err := s.Step(ctx, func(w *Writer) error {
		return w.Insert(ctx, "department", []string{"id", "name"}, [][]any{
			{int64(53), "A"}, {int64(54), "B"}, {int64(55), "C"}, {int64(53), "Again"},
		})
	})
	if err == nil || !strings.Contains(err.Error(), "insert department rows 55-53") {
		t.Fatalf("err = %v, want it to name rows 55-53", err)
	}
}
