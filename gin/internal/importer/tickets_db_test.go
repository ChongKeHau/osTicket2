package importer

import (
	"context"
	"testing"
	"time"

	"github.com/grandpine/ticket-api/internal/db/testutil"
)

func TestImportTickets(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	src := openTestSource(t)
	sink := NewSink(tx, 2, false)
	lk, rep := NewLookup(), NewReport()
	runReferenceSteps(t, sink, src, lk, rep)
	if err := sink.Step(ctx, func(w *Writer) error { return importTickets(ctx, src, w, lk, rep) }); err != nil {
		t.Fatal(err)
	}
	c := rep.Counter(EntityTickets)
	if c.Read != 5 || c.Written != 4 || c.Skipped != 1 || c.Reasons[ReasonDeletedStatus] != 1 {
		t.Fatalf("ticket counter = %+v", c)
	}
	if _, ok := lk.Tickets[4]; ok {
		t.Fatal("deleted ticket must not be mapped")
	}
	var number, subject, email, source string
	var priority, status, dept int64
	var assignee *int64
	var due *time.Time
	q := "SELECT number, subject, priority_id, status_id, dept_id, assigned_staff_id, requester_email, source, due_at FROM ticket WHERE id = $1"
	if err := tx.QueryRow(ctx, q, lk.Tickets[1]).Scan(&number, &subject, &priority, &status, &dept, &assignee, &email, &source, &due); err != nil {
		t.Fatal(err)
	}
	if number != "100001" || subject != "Printer on fire" || priority != lk.Priorities[3] || status != lk.Statuses[1] || dept != lk.DefaultDept || assignee == nil || *assignee != lk.Staff[1] || email != "pat@example.test" || source != "web" || due == nil {
		t.Fatalf("ticket 1 = %s %s %d %d %d %v %s %s %v", number, subject, priority, status, dept, assignee, email, source, due)
	}
	// Ticket 2: Resolved, no user_email_id → default email, Email source → other, priority from answer.
	if err := tx.QueryRow(ctx, q, lk.Tickets[2]).Scan(&number, &subject, &priority, &status, &dept, &assignee, &email, &source, &due); err != nil {
		t.Fatal(err)
	}
	if status != lk.Statuses[2] || email != "fallback@example.test" || source != "other" || assignee != nil || priority != lk.Priorities[2] {
		t.Fatalf("ticket 2 = %d %s %s %v %d", status, email, source, assignee, priority)
	}
	// Ticket 3: archived → closed status id 4, empty number → 000003, empty subject, no email row → placeholder, phone.
	if err := tx.QueryRow(ctx, q, lk.Tickets[3]).Scan(&number, &subject, &priority, &status, &dept, &assignee, &email, &source, &due); err != nil {
		t.Fatal(err)
	}
	if number != "000003" || subject != "(no subject)" || email != "unknown-3@imported.invalid" || source != "phone" || status != lk.Statuses[4] || priority != lk.DefaultPriority {
		t.Fatalf("ticket 3 = %s %s %s %s %d %d", number, subject, email, source, status, priority)
	}
	// Ticket 5: duplicate number, unknown dept and staff → seed dept, unassigned; topic 3 has no priority → seed normal.
	if err := tx.QueryRow(ctx, q, lk.Tickets[5]).Scan(&number, &subject, &priority, &status, &dept, &assignee, &email, &source, &due); err != nil {
		t.Fatal(err)
	}
	if number != "100001-5" || dept != lk.DefaultDept || assignee != nil || priority != lk.DefaultPriority || status != lk.Statuses[6] || source != "api" {
		t.Fatalf("ticket 5 = %s %d %v %d %d %s", number, dept, assignee, priority, status, source)
	}
	var extra map[string]any
	if err := tx.QueryRow(ctx, "SELECT extra FROM ticket WHERE id = $1", lk.Tickets[3]).Scan(&extra); err != nil || extra["source_extra"] != "ext 12" || extra["source"] != "Phone" {
		t.Fatalf("extra = %v, %v", extra, err)
	}
}
