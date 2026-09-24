package ticket

import (
	"errors"
	"testing"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/db"
)

func (f *fx) file(t *testing.T, name string) db.File {
	t.Helper()
	fl, err := f.q.CreateFile(f.ctx, db.CreateFileParams{
		Key: "key-" + name, Name: name, Mime: "text/plain", Size: 3, Sha256: "abc", Backend: "local",
	})
	if err != nil {
		t.Fatal(err)
	}
	return fl
}

func TestReplyMarksAnsweredAndCanClose(t *testing.T) {
	f := newFixture(t)
	tk := f.create(t, f.agent, "Q", f.support.ID)
	fl := f.file(t, "a.txt")
	entry, err := f.svc.Reply(f.ctx, f.agent, tk.ID, ReplyInput{Body: "Answer", FileIDs: []int64{fl.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if entry.Type != "response" || entry.StaffID == nil || entry.Poster != "agent Person" || len(entry.Attachments) != 1 || entry.Attachments[0].Name != "a.txt" {
		t.Fatalf("entry: %+v", entry)
	}
	got, _ := f.svc.Get(f.ctx, f.agent, tk.ID)
	if !got.IsAnswered || got.LastResponseAt == nil || got.State != "open" {
		t.Fatalf("after reply: %+v", got)
	}
	if _, err := f.svc.Reply(f.ctx, f.agent, tk.ID, ReplyInput{Body: "Closing", StatusID: &f.closed.ID}); err != nil {
		t.Fatal(err)
	}
	got, _ = f.svc.Get(f.ctx, f.agent, tk.ID)
	if got.State != "closed" || got.ClosedAt == nil {
		t.Fatalf("reply with status: %+v", got)
	}
	if _, err := f.svc.Reply(f.ctx, f.other, tk.ID, ReplyInput{Body: "x"}); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("invisible reply: %v", err)
	}
}

func TestReplyOnClosedTicketKeepsStatus(t *testing.T) {
	f := newFixture(t)
	tk := f.create(t, f.agent, "Q", f.support.ID)
	if _, err := f.svc.SetStatus(f.ctx, f.agent, tk.ID, f.closed.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Reply(f.ctx, f.agent, tk.ID, ReplyInput{Body: "late reply"}); err != nil {
		t.Fatal(err)
	}
	got, _ := f.svc.Get(f.ctx, f.agent, tk.ID)
	if got.State != "closed" || got.ClosedAt == nil {
		t.Fatalf("reply must not reopen: %+v", got)
	}
}

func TestNoteDoesNotMarkAnswered(t *testing.T) {
	f := newFixture(t)
	tk := f.create(t, f.agent, "Q", f.support.ID)
	entry, err := f.svc.Note(f.ctx, f.agent, tk.ID, NoteInput{Title: "internal", Body: "note body", Format: "text"})
	if err != nil {
		t.Fatal(err)
	}
	if entry.Type != "note" || entry.Title == nil || *entry.Title != "internal" || entry.Format != "text" {
		t.Fatalf("note: %+v", entry)
	}
	got, _ := f.svc.Get(f.ctx, f.agent, tk.ID)
	if got.IsAnswered || got.LastResponseAt != nil {
		t.Fatalf("note must not answer: %+v", got)
	}
}

func TestThreadCursor(t *testing.T) {
	f := newFixture(t)
	tk := f.create(t, f.agent, "Q", f.support.ID)
	for i := 0; i < 4; i++ {
		if _, err := f.svc.Note(f.ctx, f.agent, tk.ID, NoteInput{Body: "n"}); err != nil {
			t.Fatal(err)
		}
	}
	page, err := f.svc.Thread(f.ctx, f.agent, tk.ID, 0, 2)
	if err != nil || len(page.Items) != 2 || page.NextAfter == nil || page.Items[0].Type != "message" {
		t.Fatalf("page 1: %+v %v", page, err)
	}
	page2, _ := f.svc.Thread(f.ctx, f.agent, tk.ID, *page.NextAfter, 2)
	if len(page2.Items) != 2 || page2.NextAfter == nil || page2.Items[0].ID <= page.Items[1].ID {
		t.Fatalf("page 2: %+v", page2)
	}
	page3, _ := f.svc.Thread(f.ctx, f.agent, tk.ID, *page2.NextAfter, 2)
	if len(page3.Items) != 1 || page3.NextAfter != nil {
		t.Fatalf("page 3: %+v", page3)
	}
	if _, err := f.svc.Thread(f.ctx, f.other, tk.ID, 0, 10); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("invisible thread: %v", err)
	}
}

func TestStatusTransitions(t *testing.T) {
	f := newFixture(t)
	tk := f.create(t, f.agent, "Q", f.support.ID)
	same, err := f.svc.SetStatus(f.ctx, f.agent, tk.ID, f.open.ID)
	if err != nil || same.State != "open" {
		t.Fatalf("same status: %+v %v", same, err)
	}
	if n := f.count(t, `SELECT count(*) FROM ticket_event WHERE ticket_id = $1 AND kind <> 'created'`, tk.ID); n != 0 {
		t.Fatalf("no-op must not write events: %d", n)
	}
	closed, err := f.svc.SetStatus(f.ctx, f.agent, tk.ID, f.closed.ID)
	if err != nil || closed.State != "closed" || closed.ClosedAt == nil {
		t.Fatalf("close: %+v %v", closed, err)
	}
	if n := f.count(t, `SELECT count(*) FROM ticket_event WHERE ticket_id = $1 AND kind = 'closed'`, tk.ID); n != 1 {
		t.Fatalf("closed event: %d", n)
	}
	resolved, err := f.svc.SetStatus(f.ctx, f.agent, tk.ID, f.resolved.ID)
	if err != nil || resolved.State != "resolved" || resolved.ClosedAt == nil {
		t.Fatalf("closed to resolved keeps closed_at: %+v %v", resolved, err)
	}
	if n := f.count(t, `SELECT count(*) FROM ticket_event WHERE ticket_id = $1 AND kind = 'status_changed'`, tk.ID); n != 1 {
		t.Fatalf("status_changed event: %d", n)
	}
	reopened, err := f.svc.SetStatus(f.ctx, f.agent, tk.ID, f.open.ID)
	if err != nil || reopened.State != "open" || reopened.ClosedAt != nil {
		t.Fatalf("reopen: %+v %v", reopened, err)
	}
	if n := f.count(t, `SELECT count(*) FROM ticket_event WHERE ticket_id = $1 AND kind = 'reopened'`, tk.ID); n != 1 {
		t.Fatalf("reopened event: %d", n)
	}
	var ve *apperr.ValidationError
	if _, err := f.svc.SetStatus(f.ctx, f.agent, tk.ID, 999999); !errors.As(err, &ve) || ve.Fields["status_id"] == "" {
		t.Fatalf("unknown status: %v", err)
	}
}

func TestAssign(t *testing.T) {
	f := newFixture(t)
	tk := f.create(t, f.agent, "Q", f.support.ID)
	var ve *apperr.ValidationError
	bad := int64(999999)
	if _, err := f.svc.Assign(f.ctx, f.agent, tk.ID, &bad); !errors.As(err, &ve) || ve.Fields["staff_id"] == "" {
		t.Fatalf("unknown staff: %v", err)
	}
	other := f.staff["other"].ID
	if _, err := f.svc.Assign(f.ctx, f.agent, tk.ID, &other); !errors.As(err, &ve) || ve.Fields["staff_id"] == "" {
		t.Fatalf("staff outside dept: %v", err)
	}
	if _, err := f.tx.Exec(f.ctx, `UPDATE staff SET is_active = false WHERE id = $1`, f.staff["admin"].ID); err != nil {
		t.Fatal(err)
	}
	adminID := f.staff["admin"].ID
	if _, err := f.svc.Assign(f.ctx, f.agent, tk.ID, &adminID); !errors.As(err, &ve) || ve.Fields["staff_id"] == "" {
		t.Fatalf("inactive staff: %v", err)
	}
	me := f.agent.StaffID
	out, err := f.svc.Assign(f.ctx, f.agent, tk.ID, &me)
	if err != nil || out.Assignee == nil || out.Assignee.ID != me || out.Assignee.Name != "agent Person" {
		t.Fatalf("assign: %+v %v", out, err)
	}
	out, err = f.svc.Assign(f.ctx, f.agent, tk.ID, nil)
	if err != nil || out.Assignee != nil {
		t.Fatalf("unassign: %+v %v", out, err)
	}
	if n := f.count(t, `SELECT count(*) FROM ticket_event WHERE ticket_id = $1 AND kind IN ('assigned','unassigned')`, tk.ID); n != 2 {
		t.Fatalf("assignment events: %d", n)
	}
}

func TestTransfer(t *testing.T) {
	f := newFixture(t)
	tk := f.create(t, f.admin, "Q", f.support.ID)
	me := f.agent.StaffID
	if _, err := f.svc.Assign(f.ctx, f.admin, tk.ID, &me); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Transfer(f.ctx, f.agent, tk.ID, f.billing.ID); !errors.Is(err, apperr.ErrForbidden) {
		t.Fatalf("agent transfer to invisible dept: %v", err)
	}
	var ve *apperr.ValidationError
	if _, err := f.svc.Transfer(f.ctx, f.admin, tk.ID, 999999); !errors.As(err, &ve) || ve.Fields["dept_id"] == "" {
		t.Fatalf("unknown dept: %v", err)
	}
	same, err := f.svc.Transfer(f.ctx, f.admin, tk.ID, f.support.ID)
	if err != nil || same.Assignee == nil {
		t.Fatalf("same dept no-op: %+v %v", same, err)
	}
	moved, err := f.svc.Transfer(f.ctx, f.admin, tk.ID, f.billing.ID)
	if err != nil || moved.Department.ID != f.billing.ID || moved.Assignee != nil {
		t.Fatalf("transfer must unassign agent who cannot see target: %+v %v", moved, err)
	}
	if n := f.count(t, `SELECT count(*) FROM ticket_event WHERE ticket_id = $1 AND kind = 'transferred'`, tk.ID); n != 1 {
		t.Fatalf("transferred event: %d", n)
	}
	if _, err := f.svc.Get(f.ctx, f.agent, tk.ID); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("agent lost visibility after transfer: %v", err)
	}
}

func TestAttachFilesRules(t *testing.T) {
	f := newFixture(t)
	tk := f.create(t, f.agent, "Q", f.support.ID)
	fl := f.file(t, "b.txt")
	if _, err := f.svc.Note(f.ctx, f.agent, tk.ID, NoteInput{Body: "n", FileIDs: []int64{fl.ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Note(f.ctx, f.agent, tk.ID, NoteInput{Body: "n", FileIDs: []int64{fl.ID}}); !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("attach twice: %v", err)
	}
	var ve *apperr.ValidationError
	if _, err := f.svc.Note(f.ctx, f.agent, tk.ID, NoteInput{Body: "n", FileIDs: []int64{999999}}); !errors.As(err, &ve) || ve.Fields["file_ids"] == "" {
		t.Fatalf("unknown file: %v", err)
	}
	if n := f.count(t, `SELECT count(*) FROM thread_entry WHERE ticket_id = $1`, tk.ID); n != 2 {
		t.Fatalf("failed notes must roll back entries: %d", n)
	}
	fl2 := f.file(t, "c.txt")
	created, err := f.svc.Create(f.ctx, f.agent, CreateInput{Subject: "with file", Message: "m", RequesterEmail: "r@x.test", DeptID: &f.support.ID, FileIDs: []int64{fl2.ID}})
	if err != nil {
		t.Fatal(err)
	}
	th, _ := f.svc.Thread(f.ctx, f.agent, created.ID, 0, 10)
	if len(th.Items) != 1 || len(th.Items[0].Attachments) != 1 || th.Items[0].Attachments[0].FileID != fl2.ID {
		t.Fatalf("create with file: %+v", th)
	}
}

func TestEvents(t *testing.T) {
	f := newFixture(t)
	tk := f.create(t, f.agent, "Q", f.support.ID)
	if _, err := f.svc.SetStatus(f.ctx, f.agent, tk.ID, f.closed.ID); err != nil {
		t.Fatal(err)
	}
	events, err := f.svc.Events(f.ctx, f.agent, tk.ID)
	if err != nil || len(events) != 2 || events[0].Kind != "created" || events[1].Kind != "closed" {
		t.Fatalf("events: %+v %v", events, err)
	}
	if events[1].Staff == nil || events[1].Staff.Name != "agent Person" || string(events[1].Data) == "" {
		t.Fatalf("event staff/data: %+v", events[1])
	}
	if _, err := f.svc.Events(f.ctx, f.other, tk.ID); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("invisible events: %v", err)
	}
}

// TestAttachmentUniqueConstraint proves the attachment_file_idx unique index (not
// just the application-level IsFileAttached check) stops a file from being
// attached to two entries, so a race between two concurrent attach requests for
// the same file cannot both succeed.
func TestAttachmentUniqueConstraint(t *testing.T) {
	f := newFixture(t)
	tk := f.create(t, f.agent, "Q", f.support.ID)
	entryA, err := f.svc.Note(f.ctx, f.agent, tk.ID, NoteInput{Body: "a"})
	if err != nil {
		t.Fatal(err)
	}
	entryB, err := f.svc.Note(f.ctx, f.agent, tk.ID, NoteInput{Body: "b"})
	if err != nil {
		t.Fatal(err)
	}
	fl := f.file(t, "d.txt")
	if err := f.q.CreateAttachment(f.ctx, db.CreateAttachmentParams{ThreadEntryID: entryA.ID, FileID: fl.ID}); err != nil {
		t.Fatal(err)
	}
	err = db.WithTx(f.ctx, f.tx, func(q *db.Queries) error {
		return q.CreateAttachment(f.ctx, db.CreateAttachmentParams{ThreadEntryID: entryB.ID, FileID: fl.ID})
	})
	if !db.IsUniqueViolation(err) {
		t.Fatalf("expected unique violation, got %v", err)
	}
}
