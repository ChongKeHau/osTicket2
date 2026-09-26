package inbound

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/grandpine/ticket-api/internal/attachment"
	"github.com/grandpine/ticket-api/internal/config"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/db/testutil"
	"github.com/grandpine/ticket-api/internal/mail"
	"github.com/jackc/pgx/v5"
)

type procFx struct {
	ctx  context.Context
	tx   pgx.Tx
	q    *db.Queries
	proc *Processor
	dept int64
}

func newProcFixture(t *testing.T) *procFx {
	t.Helper()
	tx := testutil.Tx(t)
	ctx := context.Background()
	q := db.New(tx)
	dept, err := q.FirstDepartment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ann", "bob"} {
		if _, err := q.CreateStaff(ctx, db.CreateStaffParams{Username: name, Email: name + "@x.test", PasswordHash: "h", FirstName: name, LastName: "Agent", IsActive: true, PrimaryDeptID: dept.ID}); err != nil {
			t.Fatal(err)
		}
	}
	store, err := attachment.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.MailConfig{FromAddress: "desk@example.test", Domain: "example.test", BaseURL: "https://desk.example.test", SiteName: "Desk"}
	n := mail.NewNotifier(cfg, mail.NewRenderer(time.Minute))
	proc := NewProcessor(tx, store, n, Options{OwnAddress: "desk@example.test", DefaultDeptID: dept.ID, MaxAttachment: 10, AllowedMIME: []string{"image/png", "application/pdf"}})
	return &procFx{ctx: ctx, tx: tx, q: q, proc: proc, dept: dept.ID}
}

// outbox claims and drains the pending outbox rows (marking each sent, as a
// real sender would) so repeated calls in one test only see newly queued mail.
func (f *procFx) outbox(t *testing.T) []db.EmailOutbox {
	t.Helper()
	rows, err := f.q.ClaimOutbox(f.ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if err := f.q.MarkOutboxSent(f.ctx, r.ID); err != nil {
			t.Fatal(err)
		}
	}
	return rows
}

func replyRaw(ticketID int64, from, subject, body string) []byte {
	return []byte(fmt.Sprintf("From: %s\r\nTo: desk@example.test\r\nSubject: %s\r\nMessage-ID: <r-%d-%s@example.test>\r\nIn-Reply-To: <ticket-%d-0123456789ab@example.test>\r\nContent-Type: text/plain\r\n\r\n%s\r\n",
		from, subject, ticketID, strings.ReplaceAll(from, "@", "_"), ticketID, body))
}

func TestProcessCreatesTicketAndDedupes(t *testing.T) {
	f := newProcFixture(t)
	out, err := f.proc.Process(f.ctx, fixture(t, "plain.eml"))
	if err != nil || out.Outcome != db.InboundOutcomeCreated || out.TicketID == nil {
		t.Fatalf("out = %+v, %v", out, err)
	}
	row, err := f.q.GetTicket(f.ctx, *out.TicketID)
	if err != nil || row.Subject != "Printer on fire" || row.RequesterEmail != "pat@example.test" || row.RequesterName != "Pat Requester" || row.Source != db.TicketSourceEmail {
		t.Fatalf("ticket = %+v, %v", row, err)
	}
	rows := f.outbox(t)
	if len(rows) != 1 || rows[0].TemplateKey != "ticket_autoresp" || rows[0].ToAddress != "pat@example.test" {
		t.Fatalf("autoresp = %+v", rows)
	}
	again, err := f.proc.Process(f.ctx, fixture(t, "plain.eml"))
	if err != nil || again.Outcome != db.InboundOutcomeCreated || *again.TicketID != *out.TicketID {
		t.Fatalf("dedupe = %+v, %v", again, err)
	}
	var n int
	_ = f.tx.QueryRow(f.ctx, "SELECT count(*) FROM ticket WHERE requester_email = 'pat@example.test'").Scan(&n)
	if n != 1 {
		t.Fatalf("tickets = %d", n)
	}
}

// A ticket opened by mail is linked to the end user for the sender's address,
// so the sender sees it in the portal.
func TestProcessCreatedTicketLinksEndUser(t *testing.T) {
	f := newProcFixture(t)
	out, err := f.proc.Process(f.ctx, fixture(t, "plain.eml"))
	if err != nil || out.TicketID == nil {
		t.Fatalf("out = %+v, %v", out, err)
	}
	var email *string
	if err := f.tx.QueryRow(f.ctx, `SELECT u.email FROM ticket t LEFT JOIN end_user u ON u.id = t.user_id WHERE t.id = $1`, *out.TicketID).Scan(&email); err != nil {
		t.Fatal(err)
	}
	if email == nil || !strings.EqualFold(*email, "pat@example.test") {
		t.Fatalf("owner = %v", email)
	}
}

func TestProcessReplyCaseInsensitiveRequester(t *testing.T) {
	f := newProcFixture(t)
	out, _ := f.proc.Process(f.ctx, fixture(t, "plain.eml"))
	_ = f.outbox(t)
	rep, err := f.proc.Process(f.ctx, replyRaw(*out.TicketID, "PAT@Example.test", "Re: [#x] whatever", "Still burning."))
	if err != nil || rep.Outcome != db.InboundOutcomeReplied || rep.TicketID == nil || *rep.TicketID != *out.TicketID || rep.EntryID == nil {
		t.Fatalf("reply = %+v, %v", rep, err)
	}
	entries, err := f.q.ListThreadEntries(f.ctx, db.ListThreadEntriesParams{TicketID: *out.TicketID, After: 0, PageLimit: 10})
	if err != nil || len(entries) != 2 || entries[1].Type != db.ThreadEntryTypeMessage || entries[1].StaffID != nil || !strings.Contains(entries[1].Body, "Still burning") {
		t.Fatalf("entries = %+v, %v", entries, err)
	}
	rows := f.outbox(t)
	to := map[string]bool{}
	for _, r := range rows {
		to[r.ToAddress] = true
		if r.TemplateKey != "message_alert" {
			t.Fatalf("row = %+v", r)
		}
	}
	if len(rows) != 2 || !to["ann@x.test"] || !to["bob@x.test"] {
		t.Fatalf("alerts = %v", to)
	}
	tk, _ := f.q.GetTicket(f.ctx, *out.TicketID)
	if tk.IsAnswered {
		t.Fatal("reply must mark unanswered")
	}
}

func TestProcessReplyFromStrangerOpensNewTicket(t *testing.T) {
	f := newProcFixture(t)
	out, _ := f.proc.Process(f.ctx, fixture(t, "plain.eml"))
	_ = f.outbox(t)
	rep, err := f.proc.Process(f.ctx, replyRaw(*out.TicketID, "else@example.test", "Re: [#100001] Printer on fire", "Not the requester"))
	if err != nil || rep.Outcome != db.InboundOutcomeCreated || *rep.TicketID == *out.TicketID {
		t.Fatalf("stranger = %+v, %v", rep, err)
	}
	row, _ := f.q.GetTicket(f.ctx, *rep.TicketID)
	if row.Subject != "Printer on fire" || row.RequesterEmail != "else@example.test" {
		t.Fatalf("new ticket = %+v", row)
	}
}

func TestProcessMatchesBySubjectTag(t *testing.T) {
	f := newProcFixture(t)
	out, _ := f.proc.Process(f.ctx, fixture(t, "plain.eml"))
	_ = f.outbox(t)
	tk, _ := f.q.GetTicket(f.ctx, *out.TicketID)
	raw := []byte("From: pat@example.test\r\nTo: desk@example.test\r\nSubject: Re: [#" + tk.Number + "] Printer on fire\r\nMessage-ID: <tag-x@example.test>\r\nContent-Type: text/plain\r\n\r\nvia tag\r\n")
	rep, err := f.proc.Process(f.ctx, raw)
	if err != nil || rep.Outcome != db.InboundOutcomeReplied || *rep.TicketID != *out.TicketID {
		t.Fatalf("tag reply = %+v, %v", rep, err)
	}
}

func TestProcessAutoSubmittedAndBounceIgnored(t *testing.T) {
	f := newProcFixture(t)
	for _, name := range []string{"autoreply.eml", "bounce.eml"} {
		out, err := f.proc.Process(f.ctx, fixture(t, name))
		if err != nil || out.Outcome != db.InboundOutcomeIgnored || out.Reason == "" || out.TicketID != nil {
			t.Fatalf("%s = %+v, %v", name, out, err)
		}
	}
	var n int
	_ = f.tx.QueryRow(f.ctx, "SELECT count(*) FROM ticket").Scan(&n)
	if n != 0 {
		t.Fatalf("tickets = %d", n)
	}
	if rows := f.outbox(t); len(rows) != 0 {
		t.Fatalf("outbox = %+v", rows)
	}
	rec, err := f.q.GetInboundByMessageID(f.ctx, "<auto-1@example.test>")
	if err != nil || rec.Outcome != db.InboundOutcomeIgnored || !strings.Contains(rec.Reason, "auto-submitted") {
		t.Fatalf("record = %+v, %v", rec, err)
	}
}

func TestProcessAttachmentsAndHTML(t *testing.T) {
	f := newProcFixture(t)
	out, err := f.proc.Process(f.ctx, fixture(t, "attachment.eml"))
	if err != nil || out.Outcome != db.InboundOutcomeCreated {
		t.Fatalf("out = %+v, %v", out, err)
	}
	var files int
	_ = f.tx.QueryRow(f.ctx, "SELECT count(*) FROM attachment a JOIN thread_entry e ON e.id = a.thread_entry_id WHERE e.ticket_id = $1", *out.TicketID).Scan(&files)
	if files != 1 {
		t.Fatalf("attached files = %d", files)
	}
	var name string
	_ = f.tx.QueryRow(f.ctx, "SELECT f.name FROM file f JOIN attachment a ON a.file_id = f.id JOIN thread_entry e ON e.id = a.thread_entry_id WHERE e.ticket_id = $1", *out.TicketID).Scan(&name)
	if name != "pixel.png" {
		t.Fatalf("file = %q", name)
	}
	if !strings.Contains(out.Reason, "big.pdf") {
		t.Fatalf("oversized attachment must be reported: %q", out.Reason)
	}
	h, err := f.proc.Process(f.ctx, fixture(t, "html_only.eml"))
	if err != nil || h.Outcome != db.InboundOutcomeCreated {
		t.Fatalf("html = %+v, %v", h, err)
	}
	entries, _ := f.q.ListThreadEntries(f.ctx, db.ListThreadEntriesParams{TicketID: *h.TicketID, After: 0, PageLimit: 10})
	if len(entries) != 1 || entries[0].Format != db.BodyFormatHtml || strings.Contains(entries[0].Body, "<script") || !strings.Contains(entries[0].Body, "<b>desk</b>") {
		t.Fatalf("html entry = %+v", entries)
	}
	tk, _ := f.q.GetTicket(f.ctx, *h.TicketID)
	if tk.Subject != "Café machine" {
		t.Fatalf("subject = %q", tk.Subject)
	}
}

// TestProcessStoresUndeclaredCharsetAndNUL runs a latin-1 body with no charset
// parameter and a NUL byte through real Postgres, which rejects both raw.
func TestProcessStoresUndeclaredCharsetAndNUL(t *testing.T) {
	f := newProcFixture(t)
	raw := "From: =?utf-8?q?P=E9t?= <nul@example.test>\r\nTo: desk@example.test\r\nSubject: Caf\xe9\x00 down\r\nMessage-ID: <nul-1@example.test>\r\nContent-Type: text/plain\r\n\r\ncaf\xe9\x00 machine\r\n"
	out, err := f.proc.Process(f.ctx, []byte(raw))
	if err != nil || out.Outcome != db.InboundOutcomeCreated || out.TicketID == nil {
		t.Fatalf("out = %+v, %v", out, err)
	}
	tk, err := f.q.GetTicket(f.ctx, *out.TicketID)
	if err != nil || tk.Subject != "Caf� down" {
		t.Fatalf("ticket = %+v, %v", tk, err)
	}
}

func TestProcessUnparseableIsRecorded(t *testing.T) {
	f := newProcFixture(t)
	out, err := f.proc.Process(f.ctx, []byte("garbage without headers"))
	if err != nil || out.Outcome != db.InboundOutcomeIgnored || !strings.Contains(out.Reason, "unparseable") {
		t.Fatalf("out = %+v, %v", out, err)
	}
	var n int
	_ = f.tx.QueryRow(f.ctx, "SELECT count(*) FROM inbound_message").Scan(&n)
	if n != 1 {
		t.Fatalf("records = %d", n)
	}
	// Redelivery (commit succeeded, STORE \Seen did not) must dedupe, not
	// hit the unique key.
	again, err := f.proc.Process(f.ctx, []byte("garbage without headers"))
	if err != nil || again.Outcome != db.InboundOutcomeIgnored {
		t.Fatalf("again = %+v, %v", again, err)
	}
	_ = f.tx.QueryRow(f.ctx, "SELECT count(*) FROM inbound_message").Scan(&n)
	if n != 1 {
		t.Fatalf("records after redelivery = %d", n)
	}
}

func TestProcessIgnoredIsDeduped(t *testing.T) {
	f := newProcFixture(t)
	for i := 0; i < 2; i++ {
		out, err := f.proc.Process(f.ctx, fixture(t, "autoreply.eml"))
		if err != nil || out.Outcome != db.InboundOutcomeIgnored {
			t.Fatalf("pass %d: out = %+v, %v", i, out, err)
		}
	}
	var n int
	_ = f.tx.QueryRow(f.ctx, "SELECT count(*) FROM inbound_message").Scan(&n)
	if n != 1 {
		t.Fatalf("records = %d", n)
	}
}
