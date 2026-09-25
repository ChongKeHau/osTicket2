package importer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/grandpine/ticket-api/internal/attachment"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/db/testutil"
)

func TestRunEndToEnd(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	filesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(filesDir, "fskey2"), []byte("PNG!"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := attachment.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Run(ctx, Options{MySQLDSN: mysqlDSN(t), Prefix: fixturePrefix, FilesDir: filesDir, Batch: 2}, tx, store)
	if err != nil {
		t.Fatalf("run: %v\n%s", err, rep)
	}
	t.Log("\n" + rep.String())
	if rep.Counter(EntityTickets).Written != 4 || rep.Counter(EntityEntries).Written != 8 || rep.Counter(EntityFiles).Written != 2 {
		t.Fatalf("counts:\n%s", rep)
	}
	// Events: 10 read; written created(1), assigned(2), transferred(3), unassigned(5), edited(6), closed(7) = 6; skipped viewed, annulled, deleted-ticket, released.
	ev := rep.Counter(EntityEvents)
	if ev.Written != 6 || ev.Skipped != 4 {
		t.Fatalf("events = %+v", ev)
	}
	var kind string
	var data map[string]any
	if err := tx.QueryRow(ctx, "SELECT kind, data FROM ticket_event WHERE id = 5").Scan(&kind, &data); err != nil || kind != "unassigned" {
		t.Fatalf("event 5 = %s %v, %v", kind, data, err)
	}
	if err := tx.QueryRow(ctx, "SELECT kind, data FROM ticket_event WHERE id = 6").Scan(&kind, &data); err != nil || kind != "edited" || data["raw"] != "not json" {
		t.Fatalf("event 6 = %s %v, %v", kind, data, err)
	}
	// last_response_at: ticket 1's newest response is entry 5 at 12:30; ticket 3 has none.
	var last *time.Time
	if err := tx.QueryRow(ctx, "SELECT last_response_at FROM ticket WHERE id = 1").Scan(&last); err != nil || last == nil || !last.Equal(time.Date(2020, 1, 10, 12, 30, 0, 0, time.UTC)) {
		t.Fatalf("ticket 1 last response = %v, %v", last, err)
	}
	if err := tx.QueryRow(ctx, "SELECT last_response_at FROM ticket WHERE id = 3").Scan(&last); err != nil || last != nil {
		t.Fatalf("ticket 3 last response = %v, %v", last, err)
	}
	// Round trip through the API's own queries.
	q := db.New(tx)
	tk, err := q.GetTicket(ctx, 1)
	if err != nil || tk.Number != "100001" || tk.Subject != "Printer on fire" {
		t.Fatalf("GetTicket = %+v, %v", tk, err)
	}
	st, err := q.GetStaffByUsername(ctx, "admin")
	if err != nil || !auth.CheckPassword(st.PasswordHash, "agentpass1") {
		t.Fatalf("staff login = %+v, %v", st, err)
	}
	// Sequences were reset: a new ticket gets an id above the imported ones.
	var id int64
	if err := tx.QueryRow(ctx, `INSERT INTO ticket (number, subject, status_id, dept_id, priority_id, requester_email)
		VALUES ('900000', 'new', (SELECT id FROM ticket_status LIMIT 1), (SELECT id FROM department LIMIT 1), (SELECT id FROM ticket_priority LIMIT 1), 'n@example.test') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if id <= 5 {
		t.Fatalf("new ticket id = %d, want above the imported ids", id)
	}
	if !rep.NeedsAttention() {
		t.Fatal("two attachments were skipped, so the run needs attention")
	}
}

func TestRunDryRun(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	rep, err := Run(ctx, Options{MySQLDSN: mysqlDSN(t), Prefix: fixturePrefix, DryRun: true}, tx, nil)
	if err != nil {
		t.Fatalf("dry run: %v\n%s", err, rep)
	}
	if rep.Counter(EntityTickets).Read != 5 || rep.Counter(EntityTickets).Written != 4 {
		t.Fatalf("dry-run counts:\n%s", rep)
	}
	var n int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM ticket").Scan(&n); err != nil || n != 0 {
		t.Fatalf("dry run wrote %d tickets", n)
	}
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM staff").Scan(&n); err != nil || n != 0 {
		t.Fatalf("dry run wrote %d staff", n)
	}
}

func TestRunRefusesNonEmptyTarget(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	if _, err := tx.Exec(ctx, "INSERT INTO department (name) VALUES ('Extra')"); err != nil {
		t.Fatal(err)
	}
	rep, err := Run(ctx, Options{MySQLDSN: mysqlDSN(t), Prefix: fixturePrefix}, tx, nil)
	if !IsPreflight(err) || rep == nil {
		t.Fatalf("err = %v", err)
	}
}
