package db_test

import (
	"context"
	"testing"

	"github.com/grandpine/ticket-api/internal/db/testutil"
)

func TestPortalBackfillLinksTicketsByEmail(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	// The migration ran against a database seeded by V2__seed.sql, which already
	// creates a 'Support' department, so this is idempotent rather than a plain
	// insert (the shared test pool applies migrations once for the whole binary).
	_, err := tx.Exec(ctx, `INSERT INTO department (name, is_public) VALUES ('Support', true) ON CONFLICT (name) DO NOTHING`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `
	  INSERT INTO ticket (number, subject, dept_id, priority_id, status_id, requester_name, requester_email, source, extra)
	  SELECT '000101', 's', d.id, p.id, s.id, 'Pat', 'Pat@Example.test', 'web', '{}'::jsonb
	  FROM department d, (SELECT id FROM ticket_priority ORDER BY id LIMIT 1) p, (SELECT id FROM ticket_status ORDER BY id LIMIT 1) s
	  WHERE d.name = 'Support'`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `
	  INSERT INTO ticket (number, subject, dept_id, priority_id, status_id, requester_name, requester_email, source, extra)
	  SELECT '000102', 's', d.id, p.id, s.id, 'pat', 'pat@example.test', 'web', '{}'::jsonb
	  FROM department d, (SELECT id FROM ticket_priority ORDER BY id LIMIT 1) p, (SELECT id FROM ticket_status ORDER BY id LIMIT 1) s
	  WHERE d.name = 'Support'`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `
	  INSERT INTO end_user (email, name)
	  SELECT DISTINCT ON (lower(requester_email)) requester_email, requester_name FROM ticket
	  WHERE NOT EXISTS (SELECT 1 FROM end_user u WHERE lower(u.email) = lower(ticket.requester_email))
	  ORDER BY lower(requester_email), created_at DESC`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `UPDATE ticket t SET user_id = u.id FROM end_user u WHERE lower(u.email) = lower(t.requester_email)`)
	if err != nil {
		t.Fatal(err)
	}
	var users, linked int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM end_user WHERE lower(email) = 'pat@example.test'`).Scan(&users); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM ticket WHERE user_id IS NOT NULL AND number IN ('000101','000102')`).Scan(&linked); err != nil {
		t.Fatal(err)
	}
	if users != 1 || linked != 2 {
		t.Fatalf("users=%d linked=%d, want 1 and 2", users, linked)
	}
}
