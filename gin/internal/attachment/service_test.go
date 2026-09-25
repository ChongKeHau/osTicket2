package attachment

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/db/testutil"
	"github.com/jackc/pgx/v5"
)

type fx struct {
	ctx     context.Context
	tx      pgx.Tx
	q       *db.Queries
	svc     Service
	store   *LocalStorage
	support db.Department
	billing db.Department
	agent   auth.Principal
	other   auth.Principal
	admin   auth.Principal
}

func newFixture(t *testing.T) *fx {
	t.Helper()
	ctx := context.Background()
	tx := testutil.Tx(t)
	q := db.New(tx)
	store, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := &fx{ctx: ctx, tx: tx, q: q, store: store}
	f.svc = NewService(tx, store, 10, []string{"text/plain", "image/png"})
	f.support, _ = q.FirstDepartment(ctx)
	f.billing, _ = q.CreateDepartment(ctx, db.CreateDepartmentParams{Name: "Billing", IsPublic: true})
	mk := func(name string, admin bool, dept int64) int64 {
		st, err := q.CreateStaff(ctx, db.CreateStaffParams{Username: name, Email: name + "@x.test", PasswordHash: "h", IsAdmin: admin, IsActive: true, PrimaryDeptID: dept})
		if err != nil {
			t.Fatal(err)
		}
		return st.ID
	}
	f.agent = auth.Principal{StaffID: mk("agent", false, f.support.ID), DeptIDs: []int64{f.support.ID}}
	f.other = auth.Principal{StaffID: mk("other", false, f.billing.ID), DeptIDs: []int64{f.billing.ID}}
	f.admin = auth.Principal{StaffID: mk("admin", true, f.support.ID), IsAdmin: true}
	return f
}

// attachToTicket creates a ticket in dept with one message entry and links fileID to it.
func (f *fx) attachToTicket(t *testing.T, fileID, dept int64) {
	t.Helper()
	status, _ := f.q.DefaultStatus(f.ctx)
	prio, _ := f.q.DefaultPriority(f.ctx)
	n, _ := f.q.NextTicketNumber(f.ctx)
	id, err := f.q.CreateTicket(f.ctx, db.CreateTicketParams{Number: fmt.Sprintf("%06d", n), Subject: "s", StatusID: status.ID, DeptID: dept, PriorityID: prio.ID, RequesterEmail: "r@x.test", Source: db.TicketSourceWeb, Extra: []byte("{}")})
	if err != nil {
		t.Fatal(err)
	}
	entry, err := f.q.CreateThreadEntry(f.ctx, db.CreateThreadEntryParams{TicketID: id, Type: db.ThreadEntryTypeMessage, Poster: "r", Body: "b", Format: db.BodyFormatHtml})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.q.CreateAttachment(f.ctx, db.CreateAttachmentParams{ThreadEntryID: entry.ID, FileID: fileID}); err != nil {
		t.Fatal(err)
	}
}

func TestUploadRules(t *testing.T) {
	f := newFixture(t)
	fl, err := f.svc.Upload(f.ctx, f.agent, "notes.txt", "text/plain", strings.NewReader("0123456789"))
	if err != nil || fl.Size != 10 || fl.Name != "notes.txt" || fl.Mime != "text/plain" {
		t.Fatalf("upload: %+v %v", fl, err)
	}
	if _, err := f.svc.Upload(f.ctx, f.agent, "big.txt", "text/plain", strings.NewReader("01234567890")); !errors.Is(err, apperr.ErrPayloadTooLarge) {
		t.Fatalf("too large: %v", err)
	}
	var ve *apperr.ValidationError
	if _, err := f.svc.Upload(f.ctx, f.agent, "x.exe", "application/octet-stream", strings.NewReader("x")); !errors.As(err, &ve) || ve.Fields["file"] == "" {
		t.Fatalf("disallowed mime: %v", err)
	}
	if _, err := f.svc.Upload(f.ctx, f.agent, "", "text/plain", strings.NewReader("x")); !errors.As(err, &ve) || ve.Fields["file"] == "" {
		t.Fatalf("empty name: %v", err)
	}
	if _, err := f.svc.Upload(f.ctx, f.agent, "..", "text/plain", strings.NewReader("x")); !errors.As(err, &ve) || ve.Fields["file"] == "" {
		t.Fatalf("'..' base name must be rejected: %v", err)
	}
	var n int
	_ = f.tx.QueryRow(f.ctx, `SELECT count(*) FROM file`).Scan(&n)
	if n != 1 {
		t.Fatalf("rejected uploads must not leave rows: %d", n)
	}
	blobs := 0
	if err := filepath.Walk(f.store.dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			blobs++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if blobs != 1 {
		t.Fatalf("rejected uploads must not leave blobs on disk: %d", blobs)
	}
}

// TestUploadFilenameTruncationOnRuneBoundary proves a filename over 255
// bytes made of multi-byte runes is truncated to a valid UTF-8 string of at
// most 255 bytes, not split mid-rune, and that truncation keeps the tail
// (the extension), not the head.
func TestUploadFilenameTruncationOnRuneBoundary(t *testing.T) {
	f := newFixture(t)
	long := strings.Repeat("é", 300) + ".txt" // 'é' is 2 bytes in UTF-8: 600+ bytes total
	fl, err := f.svc.Upload(f.ctx, f.agent, long, "text/plain", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	if len(fl.Name) > 255 {
		t.Fatalf("name must be truncated to at most 255 bytes, got %d: %q", len(fl.Name), fl.Name)
	}
	if !utf8.ValidString(fl.Name) {
		t.Fatalf("truncated name must still be valid UTF-8: %q", fl.Name)
	}
	if !strings.HasSuffix(fl.Name, ".txt") {
		t.Fatalf("truncation must keep the tail (extension), got %q", fl.Name)
	}
}

func TestDownloadAuthorization(t *testing.T) {
	f := newFixture(t)
	fl, err := f.svc.Upload(f.ctx, f.agent, "a.txt", "text/plain", strings.NewReader("abc"))
	if err != nil {
		t.Fatal(err)
	}
	// unattached: uploader and admin may read, others may not
	meta, rc, err := f.svc.Download(f.ctx, f.agent, fl.ID)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if string(b) != "abc" || meta.Name != "a.txt" {
		t.Fatalf("uploader download: %q %+v", b, meta)
	}
	if _, _, err := f.svc.Download(f.ctx, f.other, fl.ID); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("other on unattached: %v", err)
	}
	if _, rc, err := f.svc.Download(f.ctx, f.admin, fl.ID); err != nil {
		t.Fatalf("admin on unattached: %v", err)
	} else {
		rc.Close()
	}
	// attached to a billing ticket: billing agent may read, support uploader may not
	f.attachToTicket(t, fl.ID, f.billing.ID)
	if _, rc, err := f.svc.Download(f.ctx, f.other, fl.ID); err != nil {
		t.Fatalf("dept member on attached: %v", err)
	} else {
		rc.Close()
	}
	if _, _, err := f.svc.Download(f.ctx, f.agent, fl.ID); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("uploader outside dept on attached: %v", err)
	}
	if _, _, err := f.svc.Download(f.ctx, f.admin, 999999); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("missing file: %v", err)
	}
}

func TestGC(t *testing.T) {
	f := newFixture(t)
	old, err := f.svc.Upload(f.ctx, f.agent, "old.txt", "text/plain", strings.NewReader("1"))
	if err != nil {
		t.Fatal(err)
	}
	kept, err := f.svc.Upload(f.ctx, f.agent, "kept.txt", "text/plain", strings.NewReader("2"))
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := f.svc.Upload(f.ctx, f.agent, "fresh.txt", "text/plain", strings.NewReader("3"))
	if err != nil {
		t.Fatal(err)
	}
	oldRow, err := f.q.GetFile(f.ctx, old.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.tx.Exec(f.ctx, `UPDATE file SET created_at = now() - interval '2 days' WHERE id IN ($1, $2)`, old.ID, kept.ID); err != nil {
		t.Fatal(err)
	}
	f.attachToTicket(t, kept.ID, f.support.ID)
	n, err := f.svc.GC(f.ctx, 24*time.Hour)
	if err != nil || n != 1 {
		t.Fatalf("gc: %d %v", n, err)
	}
	if _, err := f.q.GetFile(f.ctx, old.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("old row must be gone: %v", err)
	}
	if _, err := f.store.Open(f.ctx, oldRow.Key); err == nil {
		t.Fatal("old blob must be deleted")
	}
	for _, id := range []int64{kept.ID, fresh.ID} {
		if _, err := f.q.GetFile(f.ctx, id); err != nil {
			t.Fatalf("file %d must survive: %v", id, err)
		}
	}
	row, _ := f.q.GetFile(f.ctx, kept.ID)
	if _, err := f.store.Open(f.ctx, row.Key); err != nil {
		t.Fatalf("kept blob must exist: %v", err)
	}
}
