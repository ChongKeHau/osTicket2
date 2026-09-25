package mail

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/grandpine/ticket-api/internal/config"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/db/testutil"
)

func TestMessageIDRoundTrip(t *testing.T) {
	id, err := NewMessageID(42, "example.test")
	if err != nil || !strings.HasPrefix(id, "<ticket-42-") || !strings.HasSuffix(id, "@example.test>") {
		t.Fatalf("id = %q, %v", id, err)
	}
	if n, ok := TicketIDFromMessageID(id); !ok || n != 42 {
		t.Fatalf("parse = %d %v", n, ok)
	}
	if _, ok := TicketIDFromMessageID("<abc@example.test>"); ok {
		t.Fatal("foreign id must not parse")
	}
	if _, ok := TicketIDFromMessageID("<ticket-x-1@example.test>"); ok {
		t.Fatal("non-numeric must not parse")
	}
}

func testCfg() config.MailConfig {
	return config.MailConfig{Enabled: true, FromName: "Desk", FromAddress: "desk@example.test", Domain: "example.test", BaseURL: "https://desk.example.test", SiteName: "Desk"}
}

func newTicket(t *testing.T, q *db.Queries) int64 {
	t.Helper()
	ctx := context.Background()
	dept, err := q.FirstDepartment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	st, err := q.DefaultStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pr, err := q.DefaultPriority(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id, err := q.CreateTicket(ctx, db.CreateTicketParams{Number: "700001", Subject: "Hello", StatusID: st.ID, DeptID: dept.ID, PriorityID: pr.ID, RequesterName: "Pat", RequesterEmail: "pat@example.test", Source: db.TicketSourceWeb, Extra: []byte("{}")})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestEnqueueWritesOutboxRowsAndThreads(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	q := db.New(tx)
	tid := newTicket(t, q)
	n := NewNotifier(testCfg(), NewRenderer(time.Minute))
	v := Vars{Number: "700001", Subject: "Hello", RequesterName: "Pat", RequesterEmail: "pat@example.test"}
	v.Message, v.MessageHTML = BodyVars("first", "text")
	err := n.Enqueue(ctx, q, Notification{TemplateKey: "ticket_autoresp", TicketID: tid, To: []Recipient{{Name: "Pat", Address: "pat@example.test"}}, Vars: v, AutoSubmitted: true})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := q.ClaimOutbox(ctx, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %+v, %v", rows, err)
	}
	r := rows[0]
	if r.ToAddress != "pat@example.test" || r.Subject != "[#700001] Hello" || !r.AutoSubmitted || r.InReplyTo != nil || !strings.Contains(r.BodyText, "https://desk.example.test/tickets/") || !strings.Contains(r.BodyHtml, "Desk") {
		t.Fatalf("row = %+v", r)
	}
	// Once that one is sent, the next mail to the same recipient threads onto it.
	if err := q.MarkOutboxSent(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	if err := n.Enqueue(ctx, q, Notification{TemplateKey: "ticket_reply", TicketID: tid, To: []Recipient{{Address: "pat@example.test"}}, Vars: v}); err != nil {
		t.Fatal(err)
	}
	rows, _ = q.ClaimOutbox(ctx, 10)
	if len(rows) != 1 || rows[0].InReplyTo == nil || *rows[0].InReplyTo != r.MessageID || rows[0].AutoSubmitted {
		t.Fatalf("threaded row = %+v", rows)
	}
}

func TestEnqueueSkipsBlankRecipientsAndBadTemplate(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	q := db.New(tx)
	tid := newTicket(t, q)
	n := NewNotifier(testCfg(), NewRenderer(time.Minute))
	if err := n.Enqueue(ctx, q, Notification{TemplateKey: "ticket_reply", TicketID: tid, To: []Recipient{{Address: ""}}, Vars: SampleVars()}); err != nil {
		t.Fatal(err)
	}
	if err := n.Enqueue(ctx, q, Notification{TemplateKey: "does_not_exist", TicketID: tid, To: []Recipient{{Address: "a@b.test"}}, Vars: SampleVars()}); err != nil {
		t.Fatalf("render failures are logged, not returned: %v", err)
	}
	var cnt int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM email_outbox").Scan(&cnt); err != nil || cnt != 0 {
		t.Fatalf("outbox = %d, %v", cnt, err)
	}
	if err := (Disabled{}).Enqueue(ctx, q, Notification{}); err != nil {
		t.Fatal(err)
	}
}
