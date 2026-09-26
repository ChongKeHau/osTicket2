package ticket

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/grandpine/ticket-api/internal/config"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/mail"
)

func mailFixture(t *testing.T) *fx {
	t.Helper()
	f := newFixture(t)
	cfg := config.MailConfig{Enabled: true, FromAddress: "desk@example.test", Domain: "example.test", BaseURL: "https://desk.example.test", SiteName: "Desk"}
	f.svc = NewService(f.tx, WithNotifier(mail.NewNotifier(cfg, mail.NewRenderer(time.Minute))))
	return f
}

// outbox claims and drains the pending outbox rows (marking each sent, as a
// real sender would) so repeated calls in one test only see newly queued mail.
func outbox(t *testing.T, f *fx) []db.EmailOutbox {
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

func TestCreateEnqueuesAutoresponse(t *testing.T) {
	f := mailFixture(t)
	tk, err := f.svc.Create(f.ctx, f.admin, CreateInput{Subject: "Printer", Message: "Smoke", RequesterName: "Pat", RequesterEmail: "pat@example.test", DeptID: &f.support.ID})
	if err != nil {
		t.Fatal(err)
	}
	rows := outbox(t, f)
	if len(rows) != 1 || rows[0].TemplateKey != "ticket_autoresp" || rows[0].ToAddress != "pat@example.test" || rows[0].TicketID == nil || *rows[0].TicketID != tk.ID || !rows[0].AutoSubmitted {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[0].Subject != "[#"+tk.Number+"] Printer" {
		t.Fatalf("subject = %q", rows[0].Subject)
	}
	// A staff-created ticket's created event carries no "via" key.
	events, err := f.svc.Events(f.ctx, f.admin, tk.ID)
	if err != nil || len(events) != 1 || events[0].Kind != "created" {
		t.Fatalf("events = %+v, %v", events, err)
	}
	var data map[string]any
	if err := json.Unmarshal(events[0].Data, &data); err != nil {
		t.Fatal(err)
	}
	if _, ok := data["via"]; ok {
		t.Fatalf("staff create must not set via: %+v", data)
	}
}

// TestOmittedFormatMailsAsHTML: an omitted format is stored as html, so the
// mail must render it as HTML too rather than escaping the markup.
func TestOmittedFormatMailsAsHTML(t *testing.T) {
	f := mailFixture(t)
	tk, err := f.svc.Create(f.ctx, f.admin, CreateInput{Subject: "Printer", Message: "<p>Smoke <b>here</b></p>", RequesterEmail: "pat@example.test", DeptID: &f.support.ID})
	if err != nil {
		t.Fatal(err)
	}
	rows := outbox(t, f)
	if len(rows) != 1 || !strings.Contains(rows[0].BodyHtml, "<b>here</b>") || strings.Contains(rows[0].BodyHtml, "&lt;p&gt;") {
		t.Fatalf("autoresp html = %+v", rows)
	}
	if _, err := f.svc.Reply(f.ctx, f.agent, tk.ID, ReplyInput{Body: "<p>On <i>it</i></p>"}); err != nil {
		t.Fatal(err)
	}
	rows = outbox(t, f)
	if len(rows) != 1 || !strings.Contains(rows[0].BodyHtml, "<p>On <i>it</i></p>") || strings.Contains(rows[0].BodyHtml, "&lt;p&gt;") {
		t.Fatalf("reply html = %+v", rows)
	}
	if strings.Contains(rows[0].BodyText, "<p>") {
		t.Fatalf("reply text part keeps markup: %q", rows[0].BodyText)
	}
}

func TestReplyAndAssignEnqueue(t *testing.T) {
	f := mailFixture(t)
	tk, err := f.svc.Create(f.ctx, f.admin, CreateInput{Subject: "Printer", Message: "Smoke", RequesterEmail: "pat@example.test", DeptID: &f.support.ID})
	if err != nil {
		t.Fatal(err)
	}
	_ = outbox(t, f) // consume the autoresponse
	if _, err := f.svc.Reply(f.ctx, f.agent, tk.ID, ReplyInput{Body: "On it", Format: "text"}); err != nil {
		t.Fatal(err)
	}
	rows := outbox(t, f)
	if len(rows) != 1 || rows[0].TemplateKey != "ticket_reply" || rows[0].ToAddress != "pat@example.test" || rows[0].EntryID == nil {
		t.Fatalf("reply rows = %+v", rows)
	}
	// Self-assignment sends nothing; assigning someone else alerts them.
	agentID, otherID, adminID := f.staff["agent"].ID, f.staff["other"].ID, f.staff["admin"].ID
	if _, err := f.svc.Assign(f.ctx, f.agent, tk.ID, &agentID); err != nil {
		t.Fatal(err)
	}
	if rows := outbox(t, f); len(rows) != 0 {
		t.Fatalf("self-assign rows = %+v", rows)
	}
	if _, err := f.svc.Assign(f.ctx, f.admin, tk.ID, &otherID); err == nil {
		t.Fatal("other cannot see support: expected validation error")
	}
	// The acting principal (agent) is not the new assignee (admin), so this
	// is a real reassignment, not a self-assignment: it must alert admin.
	if _, err := f.svc.Assign(f.ctx, f.agent, tk.ID, &adminID); err != nil {
		t.Fatal(err)
	}
	rows = outbox(t, f)
	if len(rows) != 1 || rows[0].TemplateKey != "assigned_alert" || rows[0].ToAddress != "admin@x.test" {
		t.Fatalf("assign rows = %+v", rows)
	}
	// Notes never send.
	if _, err := f.svc.Note(f.ctx, f.agent, tk.ID, NoteInput{Body: "internal", Format: "text"}); err != nil {
		t.Fatal(err)
	}
	if rows := outbox(t, f); len(rows) != 0 {
		t.Fatalf("note rows = %+v", rows)
	}
}

func TestCreateExternalAndAppendMessage(t *testing.T) {
	f := mailFixture(t)
	tk, err := f.svc.CreateExternal(f.ctx, ExternalCreateInput{Subject: "By mail", Body: "<p>Hi</p>", Format: "html", RequesterName: "Pat", RequesterEmail: "Pat@Example.test", DeptID: f.support.ID, AutoSubmitted: true})
	if err != nil {
		t.Fatal(err)
	}
	if tk.Source != "email" || tk.RequesterEmail != "Pat@Example.test" {
		t.Fatalf("ticket = %+v", tk)
	}
	if rows := outbox(t, f); len(rows) != 0 {
		t.Fatalf("auto-submitted create must not autorespond: %+v", rows)
	}
	events, err := f.svc.Events(f.ctx, f.admin, tk.ID)
	if err != nil || len(events) != 1 || events[0].Kind != "created" || events[0].Staff != nil {
		t.Fatalf("events = %+v, %v", events, err)
	}
	var data map[string]any
	if err := json.Unmarshal(events[0].Data, &data); err != nil {
		t.Fatal(err)
	}
	if data["via"] != "email" {
		t.Fatalf("external create must set via=email: %+v", data)
	}
	// Unassigned: every active staff member of the department is alerted (admin and agent, not other).
	entry, err := f.svc.AppendMessage(f.ctx, tk.ID, MessageInput{Poster: "Pat", Body: "More info", Format: "text"})
	if err != nil {
		t.Fatal(err)
	}
	if entry.Type != "message" || entry.StaffID != nil || entry.Poster != "Pat" {
		t.Fatalf("entry = %+v", entry)
	}
	rows := outbox(t, f)
	to := map[string]bool{}
	for _, r := range rows {
		if r.TemplateKey != "message_alert" {
			t.Fatalf("row = %+v", r)
		}
		to[r.ToAddress] = true
	}
	if len(rows) != 2 || !to["admin@x.test"] || !to["agent@x.test"] {
		t.Fatalf("recipients = %v", to)
	}
	got, _ := f.svc.Get(f.ctx, f.admin, tk.ID)
	if got.IsAnswered {
		t.Fatal("append must mark the ticket unanswered")
	}
	// Assigned: only the assignee is alerted.
	agentID := f.staff["agent"].ID
	if _, err := f.svc.Assign(f.ctx, f.admin, tk.ID, &agentID); err != nil {
		t.Fatal(err)
	}
	_ = outbox(t, f)
	if _, err := f.svc.AppendMessage(f.ctx, tk.ID, MessageInput{Poster: "Pat", Body: "Again", Format: "text"}); err != nil {
		t.Fatal(err)
	}
	rows = outbox(t, f)
	if len(rows) != 1 || rows[0].ToAddress != "agent@x.test" {
		t.Fatalf("assigned alert = %+v", rows)
	}
	if _, err := f.svc.AppendMessage(f.ctx, 999999, MessageInput{Poster: "x", Body: "y", Format: "text"}); err == nil {
		t.Fatal("unknown ticket must fail")
	}
}

func TestAppendMessageSkipsInactiveAssignee(t *testing.T) {
	f := mailFixture(t)
	tk, err := f.svc.CreateExternal(f.ctx, ExternalCreateInput{Subject: "By mail", Body: "Hi", Format: "text", RequesterName: "Pat", RequesterEmail: "pat@example.test", DeptID: f.support.ID, AutoSubmitted: true})
	if err != nil {
		t.Fatal(err)
	}
	agentID := f.staff["agent"].ID
	if _, err := f.svc.Assign(f.ctx, f.admin, tk.ID, &agentID); err != nil {
		t.Fatal(err)
	}
	_ = outbox(t, f) // consume the assigned_alert
	if _, err := f.tx.Exec(f.ctx, `UPDATE staff SET is_active = false WHERE id = $1`, agentID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.AppendMessage(f.ctx, tk.ID, MessageInput{Poster: "Pat", Body: "Again", Format: "text"}); err != nil {
		t.Fatal(err)
	}
	rows := outbox(t, f)
	if len(rows) != 1 || rows[0].ToAddress != "admin@x.test" || rows[0].TemplateKey != "message_alert" {
		t.Fatalf("inactive-assignee fallback rows = %+v", rows)
	}
}

func TestDisabledNotifierIsDefault(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.Create(context.Background(), f.admin, CreateInput{Subject: "S", Message: "M", RequesterEmail: "p@x.test", DeptID: &f.support.ID}); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := f.tx.QueryRow(f.ctx, "SELECT count(*) FROM email_outbox").Scan(&n); err != nil || n != 0 {
		t.Fatalf("outbox = %d, %v", n, err)
	}
}
