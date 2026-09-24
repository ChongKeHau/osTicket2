package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/db/testutil"
)

func TestSeedData(t *testing.T) {
	ctx := context.Background()
	q := db.New(testutil.Tx(t))
	statuses, err := q.ListStatuses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 3 || statuses[0].State != db.TicketStateOpen || statuses[2].State != db.TicketStateClosed {
		t.Fatalf("statuses: %+v", statuses)
	}
	prios, err := q.ListPriorities(ctx)
	if err != nil || len(prios) != 4 || prios[0].Name != "low" {
		t.Fatalf("priorities: %+v %v", prios, err)
	}
	def, err := q.DefaultPriority(ctx)
	if err != nil || def.Name != "normal" {
		t.Fatalf("default priority: %+v %v", def, err)
	}
	ds, err := q.DefaultStatus(ctx)
	if err != nil || ds.State != db.TicketStateOpen {
		t.Fatalf("default status: %+v %v", ds, err)
	}
}

func TestWithTxRollsBackOnError(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	boom := errors.New("boom")
	err := db.WithTx(ctx, tx, func(q *db.Queries) error {
		if _, err := tx.Exec(ctx, `INSERT INTO department (name) VALUES ('Rollback Me')`); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected boom, got %v", err)
	}
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM department WHERE name = 'Rollback Me'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("row survived rollback")
	}
	err = db.WithTx(ctx, tx, func(q *db.Queries) error {
		_, err := tx.Exec(ctx, `INSERT INTO department (name) VALUES ('Keep Me')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM department WHERE name = 'Keep Me'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("committed savepoint missing: %d %v", n, err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO department (name) VALUES ('Keep Me')`); !db.IsUniqueViolation(err) {
		t.Fatalf("expected unique violation, got %v", err)
	}
}
