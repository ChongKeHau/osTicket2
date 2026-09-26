package db_test

import (
	"context"
	"testing"

	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/db/testutil"
)

func TestEmailMigrationSeedsTemplatesAndEnum(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	q := db.New(tx)
	tpls, err := q.ListEmailTemplates(ctx)
	if err != nil || len(tpls) != 8 {
		t.Fatalf("templates = %d, %v", len(tpls), err)
	}
	keys := map[string]bool{}
	for _, tp := range tpls {
		keys[tp.Key] = true
	}
	for _, k := range []string{"ticket_autoresp", "ticket_reply", "assigned_alert", "message_alert"} {
		if !keys[k] {
			t.Fatalf("missing template %s", k)
		}
	}
	var ok bool
	if err := tx.QueryRow(ctx, "SELECT 'email'::ticket_source = 'email'").Scan(&ok); err != nil || !ok {
		t.Fatalf("ticket_source email: %v", err)
	}
	var n int64
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM email_outbox").Scan(&n); err != nil || n != 0 {
		t.Fatalf("outbox = %d, %v", n, err)
	}
}
