package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/attachment"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/httpx"
	"github.com/grandpine/ticket-api/internal/ticket"
	"github.com/jackc/pgx/v5"
)

type portalFx struct {
	ctx     context.Context
	tx      pgx.Tx
	q       *db.Queries
	n       *recordingNotifier
	ident   *Service
	files   attachment.Service
	svc     *PortalService
	tickets ticket.Service // the staff side, for notes and status changes
	staff   auth.Principal
	dept    db.Department
}

func newPortal(t *testing.T, openMax int) *portalFx {
	t.Helper()
	ident, n, tx := newSvc(t)
	ctx := context.Background()
	q := db.New(tx)
	store, err := attachment.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	files := attachment.NewService(tx, store, 1<<20, []string{"text/plain"})
	dept, err := q.FirstDepartment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	st, err := q.CreateStaff(ctx, db.CreateStaffParams{
		Username: "alex", Email: "alex@desk.test", PasswordHash: "h", FirstName: "Alex", LastName: "Agent",
		IsAdmin: true, IsActive: true, PrimaryDeptID: dept.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &portalFx{
		ctx: ctx, tx: tx, q: q, n: n, ident: ident, files: files, dept: dept,
		svc:     NewPortalService(tx, n, files, ident, NewLimiter(openMax, time.Hour), "Desk"),
		tickets: ticket.NewService(tx, ticket.WithNotifier(n)),
		staff:   auth.Principal{StaffID: st.ID, IsAdmin: true, DeptIDs: []int64{dept.ID}},
	}
}

// open files a ticket anonymously from a unique IP, so the limiter never trips.
func (f *portalFx) open(t *testing.T, email, subject string) *Opened {
	t.Helper()
	out, err := f.svc.OpenTicket(f.ctx, nil, "ip-"+email+subject, OpenInput{
		Name: "Pat", Email: email, Subject: subject, Message: "help", DeptID: &f.dept.ID,
	})
	if err != nil {
		t.Fatalf("open %s: %v", subject, err)
	}
	return out
}

func (f *portalFx) principal(t *testing.T, email string) Principal {
	t.Helper()
	u, err := f.q.GetEndUserByEmail(f.ctx, email)
	if err != nil {
		t.Fatalf("end user %s: %v", email, err)
	}
	p, err := f.ident.LoadClient(f.ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (f *portalFx) upload(t *testing.T, name, body string) *attachment.File {
	t.Helper()
	file, err := f.files.UploadAnonymous(f.ctx, name, "text/plain", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func (f *portalFx) int(t *testing.T, sql string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := f.tx.QueryRow(f.ctx, sql, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return n
}

// lastEvent returns the kind and data of the ticket's newest event.
func (f *portalFx) lastEvent(t *testing.T, ticketID int64) (string, map[string]any) {
	t.Helper()
	var kind string
	var raw []byte
	if err := f.tx.QueryRow(f.ctx, `SELECT kind::text, data FROM ticket_event WHERE ticket_id = $1 ORDER BY id DESC LIMIT 1`, ticketID).Scan(&kind, &raw); err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	return kind, data
}

func (f *portalFx) closeByStaff(t *testing.T, id int64) {
	t.Helper()
	st, err := f.q.FirstStatusInState(f.ctx, db.TicketStateClosed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.tickets.SetStatus(f.ctx, f.staff, id, st.ID); err != nil {
		t.Fatal(err)
	}
}

func userIDOf(data map[string]any) int64 {
	v, _ := data["user_id"].(float64)
	return int64(v)
}

func TestReferenceListsPublicDepartmentsAndActiveTopics(t *testing.T) {
	f := newPortal(t, 100)
	if _, err := f.q.CreateDepartment(f.ctx, db.CreateDepartmentParams{Name: "Internal", IsPublic: false}); err != nil {
		t.Fatal(err)
	}
	ref, err := f.svc.Reference(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ref.SiteName != "Desk" || len(ref.Departments) != 1 || ref.Departments[0].Name != "Support" || len(ref.Topics) != 1 || ref.Topics[0].Name != "General Inquiry" {
		t.Fatalf("reference %+v", ref)
	}
}

func TestOpenTicketAnonymousCreatesUserAndAutoresponse(t *testing.T) {
	f := newPortal(t, 100)
	f.n.sent = nil
	out, err := f.svc.OpenTicket(f.ctx, nil, "203.0.113.1", OpenInput{
		Name: "Pat Doe", Email: "Pat@Example.test", Subject: "Printer", Message: "It jams", DeptID: &f.dept.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.ID == 0 || out.Number == "" {
		t.Fatalf("opened %+v", out)
	}
	u, err := f.q.GetEndUserByEmail(f.ctx, "pat@example.test")
	if err != nil || u.Name != "Pat Doe" {
		t.Fatalf("end user %+v %v", u, err)
	}
	var userID *int64
	var source string
	if err := f.tx.QueryRow(f.ctx, `SELECT user_id, source::text FROM ticket WHERE id = $1`, out.ID).Scan(&userID, &source); err != nil {
		t.Fatal(err)
	}
	if userID == nil || *userID != u.ID || source != "web" {
		t.Fatalf("ticket user %v source %q", userID, source)
	}
	if n := f.int(t, `SELECT count(*) FROM thread_entry WHERE ticket_id = $1 AND user_id = $2`, out.ID, u.ID); n != 1 {
		t.Fatalf("first message user_id rows %d", n)
	}
	var via string
	if err := f.tx.QueryRow(f.ctx, `SELECT data->>'via' FROM ticket_event WHERE ticket_id = $1 AND kind = 'created'`, out.ID).Scan(&via); err != nil || via != "portal" {
		t.Fatalf("created via %q %v", via, err)
	}
	var auto []string
	for _, m := range f.n.sent {
		if m.TemplateKey == "ticket_autoresp" {
			auto = append(auto, m.To[0].Address)
			if m.TicketID == nil || *m.TicketID != out.ID {
				t.Fatalf("autoresponse ticket id %v", m.TicketID)
			}
		}
	}
	if len(auto) != 1 || !strings.EqualFold(auto[0], "pat@example.test") {
		t.Fatalf("autoresponses %v (all %+v)", auto, f.n.sent)
	}

	// A second anonymous ticket for the same address reuses the end user.
	f.open(t, "pat@example.test", "Again")
	if n := f.int(t, `SELECT count(*) FROM end_user WHERE lower(email) = 'pat@example.test'`); n != 1 {
		t.Fatalf("end users %d", n)
	}
}

func TestOpenTicketValidation(t *testing.T) {
	f := newPortal(t, 100)
	var ve *apperr.ValidationError
	if _, err := f.svc.OpenTicket(f.ctx, nil, "ip", OpenInput{Subject: "S", Message: "M", DeptID: &f.dept.ID}); !errors.As(err, &ve) || ve.Fields["email"] != "required" {
		t.Fatalf("missing email: %v", err)
	}
	hidden, err := f.q.CreateDepartment(f.ctx, db.CreateDepartmentParams{Name: "Internal", IsPublic: false})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.OpenTicket(f.ctx, nil, "ip", OpenInput{Email: "a@x.test", Subject: "S", Message: "M", DeptID: &hidden.ID}); !errors.As(err, &ve) || ve.Fields["dept_id"] == "" {
		t.Fatalf("non-public department: %v", err)
	}
	// A topic supplies the department when none is given.
	var topicID int64
	if err := f.tx.QueryRow(f.ctx, `SELECT id FROM help_topic WHERE is_active LIMIT 1`).Scan(&topicID); err != nil {
		t.Fatal(err)
	}
	out, err := f.svc.OpenTicket(f.ctx, nil, "ip2", OpenInput{Email: "a@x.test", Subject: "S", Message: "M", TopicID: &topicID})
	if err != nil {
		t.Fatalf("topic only: %v", err)
	}
	if got := f.int(t, `SELECT topic_id FROM ticket WHERE id = $1`, out.ID); got != topicID {
		t.Fatalf("topic %d", got)
	}
	if _, err := f.svc.OpenTicket(f.ctx, nil, "ip3", OpenInput{Email: "a@x.test", Subject: "S", Message: "M"}); !errors.As(err, &ve) || ve.Fields["dept_id"] == "" {
		t.Fatalf("no department: %v", err)
	}
	// A staff upload that is not yet attached is not attachable from the portal.
	staffFile, err := f.files.Upload(f.ctx, f.staff, "s.txt", "text/plain", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.OpenTicket(f.ctx, nil, "ip4", OpenInput{Email: "a@x.test", Subject: "S", Message: "M", DeptID: &f.dept.ID, FileIDs: []int64{staffFile.ID}}); !errors.As(err, &ve) || ve.Fields["file_ids"] == "" {
		t.Fatalf("staff file: %v", err)
	}
}

func TestPortalAttachRequiresFileToken(t *testing.T) {
	f := newPortal(t, 100)
	file := f.upload(t, "a.txt", "a")
	other := f.upload(t, "b.txt", "b")
	staffFile, err := f.files.Upload(f.ctx, f.staff, "s.txt", "text/plain", strings.NewReader("s"))
	if err != nil {
		t.Fatal(err)
	}
	open := func(ids []int64, tokens []string) error {
		_, err := f.svc.OpenTicket(f.ctx, nil, "ip-"+strconv.Itoa(len(tokens)), OpenInput{
			Email: "ann@x.test", Subject: "S", Message: "M", DeptID: &f.dept.ID, FileIDs: ids, FileTokens: tokens,
		})
		return err
	}
	rejected := func(name string, err error) {
		t.Helper()
		var ve *apperr.ValidationError
		if !errors.As(err, &ve) || ve.Fields["file_ids"] != "unknown or already used file" {
			t.Fatalf("%s: %v", name, err)
		}
	}
	rejected("missing tokens", open([]int64{file.ID}, nil))
	rejected("wrong token", open([]int64{file.ID}, []string{other.Token}))
	rejected("empty token", open([]int64{file.ID}, []string{""}))
	rejected("length mismatch", open([]int64{file.ID, other.ID}, []string{file.Token}))
	rejected("staff upload", open([]int64{staffFile.ID}, []string{""}))
	rejected("unknown file", open([]int64{999999}, []string{file.Token}))
	if n := f.int(t, `SELECT count(*) FROM ticket WHERE requester_email = 'ann@x.test'`); n != 0 {
		t.Fatalf("rejected opens created %d tickets", n)
	}

	// A matching pair attaches; the same pair cannot be used again.
	if err := open([]int64{file.ID}, []string{file.Token}); err != nil {
		t.Fatal(err)
	}
	if n := f.int(t, `SELECT count(*) FROM attachment WHERE file_id = $1`, file.ID); n != 1 {
		t.Fatalf("attachments %d", n)
	}
	rejected("reuse", open([]int64{file.ID}, []string{file.Token}))
	var id int64
	if err := f.tx.QueryRow(f.ctx, `SELECT id FROM ticket WHERE requester_email = 'ann@x.test'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	ann := f.principal(t, "ann@x.test")
	var ve *apperr.ValidationError
	if err := f.svc.Reply(f.ctx, ann, id, ReplyInput{Body: "x", FileIDs: []int64{file.ID}, FileTokens: []string{file.Token}}); !errors.As(err, &ve) {
		t.Fatalf("reply reuse: %v", err)
	}
	if err := f.svc.Reply(f.ctx, ann, id, ReplyInput{Body: "x", FileIDs: []int64{other.ID}, FileTokens: []string{file.Token}}); !errors.As(err, &ve) {
		t.Fatalf("reply wrong token: %v", err)
	}
	if err := f.svc.Reply(f.ctx, ann, id, ReplyInput{Body: "x", FileIDs: []int64{other.ID}, FileTokens: []string{other.Token}}); err != nil {
		t.Fatalf("reply with token: %v", err)
	}
}

func TestOpenTicketSignedInUsesSessionEmail(t *testing.T) {
	f := newPortal(t, 1)
	f.open(t, "sam@x.test", "First")
	p := f.principal(t, "sam@x.test")
	// The body's address is ignored, and signed-in callers are not rate-limited.
	for i := 0; i < 3; i++ {
		out, err := f.svc.OpenTicket(f.ctx, &p, "198.51.100.7", OpenInput{Email: "mallory@x.test", Subject: "Mine", Message: "m", DeptID: &f.dept.ID})
		if err != nil {
			t.Fatal(err)
		}
		var email string
		var userID int64
		if err := f.tx.QueryRow(f.ctx, `SELECT requester_email, user_id FROM ticket WHERE id = $1`, out.ID).Scan(&email, &userID); err != nil {
			t.Fatal(err)
		}
		if email != "sam@x.test" || userID != p.UserID {
			t.Fatalf("requester %q user %d", email, userID)
		}
	}
	if n := f.int(t, `SELECT count(*) FROM end_user WHERE email = 'mallory@x.test'`); n != 0 {
		t.Fatalf("body address created a user")
	}
}

func TestListTicketsOwnOnly(t *testing.T) {
	f := newPortal(t, 100)
	a1 := f.open(t, "ann@x.test", "A1")
	a2 := f.open(t, "ann@x.test", "A2")
	a3 := f.open(t, "ann@x.test", "A3")
	f.open(t, "bob@x.test", "B1")
	f.closeByStaff(t, a1.ID)
	ann := f.principal(t, "ann@x.test")

	all, err := f.svc.ListTickets(f.ctx, ann, "", httpx.Page{Page: 1, PageSize: 25})
	if err != nil {
		t.Fatal(err)
	}
	if all.Total != 3 || len(all.Items) != 3 {
		t.Fatalf("all %+v", all)
	}
	for _, r := range all.Items {
		if !strings.HasPrefix(r.Subject, "A") || r.Department != "Support" || r.Status.Name == "" {
			t.Fatalf("row %+v", r)
		}
	}
	open, err := f.svc.ListTickets(f.ctx, ann, "open", httpx.Page{Page: 1, PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if open.Total != 2 || len(open.Items) != 1 || open.PageSize != 1 {
		t.Fatalf("open page 1 %+v", open)
	}
	page2, err := f.svc.ListTickets(f.ctx, ann, "open", httpx.Page{Page: 2, PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.Items) != 1 || page2.Items[0].ID == open.Items[0].ID {
		t.Fatalf("open page 2 %+v", page2)
	}
	got := map[int64]bool{open.Items[0].ID: true, page2.Items[0].ID: true}
	if !got[a2.ID] || !got[a3.ID] {
		t.Fatalf("open ids %v", got)
	}
	closed, err := f.svc.ListTickets(f.ctx, ann, "closed", httpx.Page{Page: 1, PageSize: 25})
	if err != nil {
		t.Fatal(err)
	}
	if closed.Total != 1 || closed.Items[0].ID != a1.ID || closed.Items[0].State != "closed" || closed.Items[0].ClosedAt == nil {
		t.Fatalf("closed %+v", closed)
	}
	var ve *apperr.ValidationError
	if _, err := f.svc.ListTickets(f.ctx, ann, "bogus", httpx.Page{Page: 1, PageSize: 25}); !errors.As(err, &ve) || ve.Fields["state"] == "" {
		t.Fatalf("bad state: %v", err)
	}
}

func TestGuestCannotListTickets(t *testing.T) {
	f := newPortal(t, 100)
	a := f.open(t, "ann@x.test", "A1")
	guest := f.principal(t, "ann@x.test")
	guest.TicketID = &a.ID
	if _, err := f.svc.ListTickets(f.ctx, guest, "", httpx.Page{Page: 1, PageSize: 25}); !errors.Is(err, apperr.ErrGuestSession) {
		t.Fatalf("guest list: %v", err)
	}
	if _, err := f.svc.Close(f.ctx, guest, a.ID); !errors.Is(err, apperr.ErrGuestSession) {
		t.Fatalf("guest close: %v", err)
	}
}

func TestGuestCanViewAndReplyOwnTicketOnly(t *testing.T) {
	f := newPortal(t, 100)
	a1 := f.open(t, "ann@x.test", "A1")
	a2 := f.open(t, "ann@x.test", "A2")
	b1 := f.open(t, "bob@x.test", "B1")
	guest := f.principal(t, "ann@x.test")
	guest.TicketID = &a1.ID

	v, err := f.svc.GetTicket(f.ctx, guest, a1.ID)
	if err != nil || v.ID != a1.ID {
		t.Fatalf("own ticket %+v %v", v, err)
	}
	if err := f.svc.Reply(f.ctx, guest, a1.ID, ReplyInput{Body: "more"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{a2.ID, b1.ID, 999999} {
		if _, err := f.svc.GetTicket(f.ctx, guest, id); !errors.Is(err, apperr.ErrNotFound) {
			t.Fatalf("guest get %d: %v", id, err)
		}
		if err := f.svc.Reply(f.ctx, guest, id, ReplyInput{Body: "x"}); !errors.Is(err, apperr.ErrNotFound) {
			t.Fatalf("guest reply %d: %v", id, err)
		}
	}
	// An account session sees every own ticket but never another user's.
	ann := f.principal(t, "ann@x.test")
	if _, err := f.svc.GetTicket(f.ctx, ann, a2.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.GetTicket(f.ctx, ann, b1.ID); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("other user's ticket: %v", err)
	}
}

func TestGetTicketHidesNotes(t *testing.T) {
	f := newPortal(t, 100)
	custFile := f.upload(t, "log.txt", "log")
	out, err := f.svc.OpenTicket(f.ctx, nil, "ip", OpenInput{
		Name: "Ann", Email: "ann@x.test", Subject: "S", Message: "first", DeptID: &f.dept.ID, FileIDs: []int64{custFile.ID}, FileTokens: []string{custFile.Token},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.tickets.Note(f.ctx, f.staff, out.ID, ticket.NoteInput{Body: "internal only"}); err != nil {
		t.Fatal(err)
	}
	staffFile, err := f.files.Upload(f.ctx, f.staff, "fix.txt", "text/plain", strings.NewReader("fix"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.tickets.Reply(f.ctx, f.staff, out.ID, ticket.ReplyInput{Body: "try this", FileIDs: []int64{staffFile.ID}}); err != nil {
		t.Fatal(err)
	}
	v, err := f.svc.GetTicket(f.ctx, f.principal(t, "ann@x.test"), out.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Entries) != 2 {
		t.Fatalf("entries %+v", v.Entries)
	}
	m, r := v.Entries[0], v.Entries[1]
	if m.Type != "message" || m.Poster != "Ann" || m.Body != "first" || len(m.Attachments) != 1 || m.Attachments[0].Name != "log.txt" {
		t.Fatalf("message %+v", m)
	}
	if r.Type != "response" || r.Poster != "Alex Agent" || r.Body != "try this" || len(r.Attachments) != 1 || r.Attachments[0].FileID != staffFile.ID {
		t.Fatalf("response %+v", r)
	}
	for _, e := range v.Entries {
		if strings.Contains(e.Body, "internal") {
			t.Fatalf("note leaked: %+v", e)
		}
	}
	if v.Number != out.Number || v.Status.Name == "" || v.State != "open" || v.Department != "Support" || v.Topic != nil {
		t.Fatalf("view %+v", v.TicketRow)
	}
}

func TestReplyAppendsMessageWithUser(t *testing.T) {
	f := newPortal(t, 100)
	out := f.open(t, "ann@x.test", "S")
	if _, err := f.tickets.Reply(f.ctx, f.staff, out.ID, ticket.ReplyInput{Body: "answer"}); err != nil {
		t.Fatal(err)
	}
	ann := f.principal(t, "ann@x.test")
	file := f.upload(t, "shot.txt", "s")
	f.n.sent = nil
	if err := f.svc.Reply(f.ctx, ann, out.ID, ReplyInput{Body: "thanks", FileIDs: []int64{file.ID}, FileTokens: []string{file.Token}}); err != nil {
		t.Fatal(err)
	}
	var entryID, userID int64
	if err := f.tx.QueryRow(f.ctx, `SELECT id, user_id FROM thread_entry WHERE ticket_id = $1 AND body = 'thanks' AND type = 'message'`, out.ID).Scan(&entryID, &userID); err != nil {
		t.Fatal(err)
	}
	if userID != ann.UserID {
		t.Fatalf("entry user %d", userID)
	}
	if n := f.int(t, `SELECT count(*) FROM attachment WHERE thread_entry_id = $1 AND file_id = $2`, entryID, file.ID); n != 1 {
		t.Fatalf("attachment rows %d", n)
	}
	var answered bool
	if err := f.tx.QueryRow(f.ctx, `SELECT is_answered FROM ticket WHERE id = $1`, out.ID).Scan(&answered); err != nil || answered {
		t.Fatalf("answered %v %v", answered, err)
	}
	if len(f.n.sent) != 1 || f.n.sent[0].TemplateKey != "message_alert" {
		t.Fatalf("mail %+v", f.n.sent)
	}
}

func TestReplyReopensClosedTicket(t *testing.T) {
	f := newPortal(t, 100)
	out := f.open(t, "ann@x.test", "S")
	f.closeByStaff(t, out.ID)
	ann := f.principal(t, "ann@x.test")
	if err := f.svc.Reply(f.ctx, ann, out.ID, ReplyInput{Body: "still broken"}); err != nil {
		t.Fatal(err)
	}
	v, err := f.svc.GetTicket(f.ctx, ann, out.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.State != "open" || v.ClosedAt != nil {
		t.Fatalf("after reply %+v", v.TicketRow)
	}
	var eventID, entryID int64
	var raw []byte
	var staffID *int64
	if err := f.tx.QueryRow(f.ctx, `SELECT id, data, staff_id FROM ticket_event WHERE ticket_id = $1 AND kind IN ('reopened', 'status_changed') ORDER BY id DESC LIMIT 1`, out.ID).Scan(&eventID, &raw, &staffID); err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	_ = json.Unmarshal(raw, &data)
	if userIDOf(data) != ann.UserID || staffID != nil {
		t.Fatalf("reopen event data %s staff %v", raw, staffID)
	}
	if err := f.tx.QueryRow(f.ctx, `SELECT id FROM thread_entry WHERE ticket_id = $1 AND body = 'still broken'`, out.ID).Scan(&entryID); err != nil {
		t.Fatal(err)
	}
	// The reopen happens first: the event row predates the message.
	var eventAt, entryAt time.Time
	_ = f.tx.QueryRow(f.ctx, `SELECT created_at FROM ticket_event WHERE id = $1`, eventID).Scan(&eventAt)
	_ = f.tx.QueryRow(f.ctx, `SELECT created_at FROM thread_entry WHERE id = $1`, entryID).Scan(&entryAt)
	if n := f.int(t, `SELECT count(*) FROM ticket_event WHERE ticket_id = $1 AND id > $2`, out.ID, eventID); n != 0 {
		t.Fatalf("events after the reopen: %d", n)
	}
	if entryAt.Before(eventAt) {
		t.Fatalf("entry %v before event %v", entryAt, eventAt)
	}
}

func TestCloseAndReopen(t *testing.T) {
	f := newPortal(t, 100)
	out := f.open(t, "ann@x.test", "S")
	ann := f.principal(t, "ann@x.test")
	if _, err := f.svc.Reopen(f.ctx, ann, out.ID); !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("reopen open ticket: %v", err)
	}
	v, err := f.svc.Close(f.ctx, ann, out.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.State != "closed" || v.ClosedAt == nil {
		t.Fatalf("closed view %+v", v.TicketRow)
	}
	kind, data := f.lastEvent(t, out.ID)
	if kind != "closed" || userIDOf(data) != ann.UserID {
		t.Fatalf("close event %s %v", kind, data)
	}
	if _, err := f.svc.Close(f.ctx, ann, out.ID); !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("close twice: %v", err)
	}
	v, err = f.svc.Reopen(f.ctx, ann, out.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.State != "open" || v.ClosedAt != nil {
		t.Fatalf("reopened view %+v", v.TicketRow)
	}
	kind, data = f.lastEvent(t, out.ID)
	if kind != "reopened" || userIDOf(data) != ann.UserID {
		t.Fatalf("reopen event %s %v", kind, data)
	}
	other := f.open(t, "bob@x.test", "B")
	if _, err := f.svc.Close(f.ctx, ann, other.ID); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("close other's ticket: %v", err)
	}
}

func TestDownloadChecksTicket(t *testing.T) {
	f := newPortal(t, 100)
	mine := f.upload(t, "mine.txt", "mine")
	a, err := f.svc.OpenTicket(f.ctx, nil, "ip", OpenInput{Email: "ann@x.test", Subject: "A", Message: "m", DeptID: &f.dept.ID, FileIDs: []int64{mine.ID}, FileTokens: []string{mine.Token}})
	if err != nil {
		t.Fatal(err)
	}
	theirs := f.upload(t, "theirs.txt", "theirs")
	b, err := f.svc.OpenTicket(f.ctx, nil, "ip2", OpenInput{Email: "bob@x.test", Subject: "B", Message: "m", DeptID: &f.dept.ID, FileIDs: []int64{theirs.ID}, FileTokens: []string{theirs.Token}})
	if err != nil {
		t.Fatal(err)
	}
	ann := f.principal(t, "ann@x.test")
	meta, rc, err := f.svc.Download(f.ctx, ann, a.ID, mine.ID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(rc)
	rc.Close()
	if meta.Name != "mine.txt" || string(body) != "mine" {
		t.Fatalf("download %+v %q", meta, body)
	}
	// Bob's file through Ann's ticket, and Bob's ticket itself, are both 404.
	if _, _, err := f.svc.Download(f.ctx, ann, a.ID, theirs.ID); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("other ticket's file: %v", err)
	}
	if _, _, err := f.svc.Download(f.ctx, ann, b.ID, theirs.ID); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("other user's ticket: %v", err)
	}
	// A file attached to a staff note on Ann's own ticket stays hidden.
	noteFile, err := f.files.Upload(f.ctx, f.staff, "internal.txt", "text/plain", strings.NewReader("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.tickets.Note(f.ctx, f.staff, a.ID, ticket.NoteInput{Body: "n", FileIDs: []int64{noteFile.ID}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.svc.Download(f.ctx, ann, a.ID, noteFile.ID); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("note attachment: %v", err)
	}
}

func TestAnonymousOpenRateLimited(t *testing.T) {
	f := newPortal(t, 1)
	in := OpenInput{Email: "ann@x.test", Subject: "S", Message: "m", DeptID: &f.dept.ID}
	if _, err := f.svc.OpenTicket(f.ctx, nil, "203.0.113.5", in); err != nil {
		t.Fatal(err)
	}
	var rl *apperr.RateLimitedError
	if _, err := f.svc.OpenTicket(f.ctx, nil, "203.0.113.6", in); !errors.As(err, &rl) || rl.RetryAfter <= 0 {
		t.Fatalf("same address: %v", err)
	}
	in.Email = "other@x.test"
	if _, err := f.svc.OpenTicket(f.ctx, nil, "203.0.113.5", in); !errors.As(err, &rl) {
		t.Fatalf("same ip: %v", err)
	}
	if n := f.int(t, `SELECT count(*) FROM ticket WHERE requester_email IN ('ann@x.test', 'other@x.test')`); n != 1 {
		t.Fatalf("tickets %d", n)
	}
}
