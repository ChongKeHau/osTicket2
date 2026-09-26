package attachment

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/db"
)

// entryWithFile creates a ticket with one entry of kind and links fileID to it.
func (f *fx) entryWithFile(t *testing.T, fileID int64, kind db.ThreadEntryType) int64 {
	t.Helper()
	status, _ := f.q.DefaultStatus(f.ctx)
	prio, _ := f.q.DefaultPriority(f.ctx)
	n, _ := f.q.NextTicketNumber(f.ctx)
	id, err := f.q.CreateTicket(f.ctx, db.CreateTicketParams{Number: fmt.Sprintf("%06d", n), Subject: "s", StatusID: status.ID, DeptID: f.support.ID, PriorityID: prio.ID, RequesterEmail: "r@x.test", Source: db.TicketSourceWeb, Extra: []byte("{}")})
	if err != nil {
		t.Fatal(err)
	}
	entry, err := f.q.CreateThreadEntry(f.ctx, db.CreateThreadEntryParams{TicketID: id, Type: kind, Poster: "r", Body: "b", Format: db.BodyFormatHtml})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.q.CreateAttachment(f.ctx, db.CreateAttachmentParams{ThreadEntryID: entry.ID, FileID: fileID}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestUploadAnonymousHasNoUploader(t *testing.T) {
	f := newFixture(t)
	fl, err := f.svc.UploadAnonymous(f.ctx, "a.txt", "text/plain", strings.NewReader("abc"))
	if err != nil || fl.Size != 3 {
		t.Fatalf("upload %+v %v", fl, err)
	}
	row, err := f.q.GetFile(f.ctx, fl.ID)
	if err != nil || row.UploadedBy != nil {
		t.Fatalf("uploaded_by %v %v", row.UploadedBy, err)
	}
	// The same size and type rules apply as for staff uploads.
	if _, err := f.svc.UploadAnonymous(f.ctx, "a.exe", "application/x-msdownload", strings.NewReader("x")); err == nil {
		t.Fatal("disallowed type accepted")
	}
	if _, err := f.svc.UploadAnonymous(f.ctx, "big.txt", "text/plain", strings.NewReader("0123456789X")); !errors.Is(err, apperr.ErrPayloadTooLarge) {
		t.Fatalf("oversize: %v", err)
	}
}

func TestDownloadForTicket(t *testing.T) {
	f := newFixture(t)
	up := func(body string) int64 {
		fl, err := f.svc.UploadAnonymous(f.ctx, "f.txt", "text/plain", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		return fl.ID
	}
	msgFile, noteFile, otherFile, loose := up("msg"), up("note"), up("other"), up("loose")
	ticketID := f.entryWithFile(t, msgFile, db.ThreadEntryTypeMessage)
	noteTicket := f.entryWithFile(t, noteFile, db.ThreadEntryTypeNote)
	f.entryWithFile(t, otherFile, db.ThreadEntryTypeResponse)

	meta, rc, err := f.svc.DownloadForTicket(f.ctx, ticketID, msgFile)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if meta.ID != msgFile || string(b) != "msg" {
		t.Fatalf("download %+v %q", meta, b)
	}
	for _, c := range []struct{ ticket, file int64 }{
		{ticketID, otherFile},  // attached to another ticket
		{noteTicket, noteFile}, // attached to a note
		{ticketID, loose},      // not attached at all
		{ticketID, 999999},     // no such file
	} {
		if _, _, err := f.svc.DownloadForTicket(f.ctx, c.ticket, c.file); !errors.Is(err, apperr.ErrNotFound) {
			t.Fatalf("ticket %d file %d: %v", c.ticket, c.file, err)
		}
	}
}
