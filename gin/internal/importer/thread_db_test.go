package importer

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grandpine/ticket-api/internal/attachment"
	"github.com/grandpine/ticket-api/internal/db/testutil"
)

func setupThroughTickets(t *testing.T) (*Sink, *Source, *Lookup, *Report) {
	t.Helper()
	tx := testutil.Tx(t)
	ctx := context.Background()
	src := openTestSource(t)
	sink := NewSink(tx, 2, false) // small batches exercise the multi-flush paths
	lk, rep := NewLookup(), NewReport()
	runReferenceSteps(t, sink, src, lk, rep)
	if err := sink.Step(ctx, func(w *Writer) error { return importTickets(ctx, src, w, lk, rep) }); err != nil {
		t.Fatal(err)
	}
	return sink, src, lk, rep
}

func TestImportEntriesParentLinks(t *testing.T) {
	sink, src, lk, rep := setupThroughTickets(t)
	ctx := context.Background()
	if err := sink.Step(ctx, func(w *Writer) error { return importEntries(ctx, src, w, lk, rep) }); err != nil {
		t.Fatal(err)
	}
	c := rep.Counter(EntityEntries)
	// 10 read: 8 written (ticket 1: 1,2,3,4,5; ticket 2: 6,7; ticket 3: 8), entry 9 (deleted ticket) and 11 (type X) skipped.
	if c.Read != 10 || c.Written != 8 || c.Skipped != 2 {
		t.Fatalf("entries = %+v", c)
	}
	var err error
	var parent *int64
	var kind, format string
	if err = sink.db.QueryRow(ctx, "SELECT parent_id, type, format FROM thread_entry WHERE id = $1", lk.Entries[4]).Scan(&parent, &kind, &format); err != nil {
		t.Fatal(err)
	}
	if parent == nil || *parent != lk.Entries[5] || kind != "message" || format != "text" {
		t.Fatalf("entry 4 = parent %v type %s format %s", parent, kind, format)
	}
	if err = sink.db.QueryRow(ctx, "SELECT parent_id, format FROM thread_entry WHERE id = $1", lk.Entries[5]).Scan(&parent, &format); err != nil || parent != nil || format != "html" {
		t.Fatalf("entry 5 = %v %s, %v", parent, format, err)
	}
	if err = sink.db.QueryRow(ctx, "SELECT parent_id FROM thread_entry WHERE id = $1", lk.Entries[2]).Scan(&parent); err != nil || parent == nil || *parent != lk.Entries[1] {
		t.Fatalf("entry 2 parent = %v, %v", parent, err)
	}
	var staff *int64
	var title *string
	if err = sink.db.QueryRow(ctx, "SELECT staff_id, title FROM thread_entry WHERE id = $1", lk.Entries[3]).Scan(&staff, &title); err != nil || staff == nil || *staff != lk.Staff[1] || title == nil || *title != "Ops note" {
		t.Fatalf("entry 3 = %v %v, %v", staff, title, err)
	}
}

// TestImportEntriesSanitisesText checks that a NUL byte and an invalid UTF-8
// byte in a MySQL body, which Postgres text rejects, are cleaned and noted
// rather than aborting the step.
func TestImportEntriesSanitisesText(t *testing.T) {
	sink, src, lk, rep := setupThroughTickets(t)
	ctx := context.Background()
	// A utf8mb4 text column refuses 0xFF in strict mode, so the body column is
	// made binary for this test (as in installs whose data bypassed the
	// charset) and restored afterwards.
	if _, err := src.db.ExecContext(ctx, "ALTER TABLE ost_thread_entry MODIFY body MEDIUMBLOB NOT NULL"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		if _, err := src.db.ExecContext(ctx, "DELETE FROM ost_thread_entry WHERE id = 50"); err != nil {
			t.Error(err)
		}
		if _, err := src.db.ExecContext(ctx, "ALTER TABLE ost_thread_entry MODIFY body text NOT NULL"); err != nil {
			t.Error(err)
		}
	})
	if _, err := src.db.ExecContext(ctx, "INSERT INTO ost_thread_entry (id,pid,thread_id,staff_id,user_id,type,flags,poster,source,title,body,format,ip_address,created,updated) VALUES (50,0,30,0,0,'M',0,'Poster','Web',NULL,CONCAT('bad', CHAR(0), 'byte', UNHEX('FF')),'text','','2020-01-13 12:00:00','2020-01-13 12:00:00')"); err != nil {
		t.Fatal(err)
	}
	if err := sink.Step(ctx, func(w *Writer) error { return importEntries(ctx, src, w, lk, rep) }); err != nil {
		t.Fatal(err)
	}
	var body string
	if err := sink.db.QueryRow(ctx, "SELECT body FROM thread_entry WHERE id = $1", lk.Entries[50]).Scan(&body); err != nil {
		t.Fatal(err)
	}
	if body != "badbyte�" {
		t.Fatalf("body = %q, want %q", body, "badbyte�")
	}
	found := false
	for _, s := range rep.Counter(EntityEntries).Samples {
		if s.ID == 50 && s.Reason == "text sanitised" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a text-sanitised note for entry 50, samples = %+v", rep.Counter(EntityEntries).Samples)
	}
}

func TestImportFilesBothBackends(t *testing.T) {
	sink, src, lk, rep := setupThroughTickets(t)
	ctx := context.Background()
	if err := sink.Step(ctx, func(w *Writer) error { return importEntries(ctx, src, w, lk, rep) }); err != nil {
		t.Fatal(err)
	}
	filesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(filesDir, "fskey2"), []byte("PNG!"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := attachment.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := sink.Step(ctx, func(w *Writer) error { return importFiles(ctx, src, w, lk, rep, store, filesDir) }); err != nil {
		t.Fatal(err)
	}
	f := rep.Counter(EntityFiles)
	a := rep.Counter(EntityAttachments)
	// Files: 1 (chunks) and 2 (disk) written; 3 (backend S) and 4 (missing) skipped.
	if f.Written != 2 || f.Skipped != 2 {
		t.Fatalf("files = %+v", f)
	}
	// Attachments: 6 read; entry3/file1 and entry7/file2 written; file 3 and 4 skipped; attachment 5 (deleted ticket) and 6 (duplicate) noted.
	if a.Read != 6 || a.Written != 2 || a.Skipped != 2 || len(a.Samples) != 4 {
		t.Fatalf("attachments = %+v", a)
	}
	var key, name, mime, sum string
	var size int64
	var by *int64
	if err := sink.db.QueryRow(ctx, "SELECT key, name, mime, size, sha256, uploaded_by FROM file WHERE id = $1", lk.Files[1]).Scan(&key, &name, &mime, &size, &sum, &by); err != nil {
		t.Fatal(err)
	}
	if name != "notes.txt" || mime != "text/plain" || size != 11 || sum != "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9" || by == nil || *by != lk.Staff[1] {
		t.Fatalf("file 1 = %s %s %d %s %v", name, mime, size, sum, by)
	}
	rc, err := store.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if string(b) != "hello world" {
		t.Fatalf("stored bytes = %q", b)
	}
	if err := sink.db.QueryRow(ctx, "SELECT key, size FROM file WHERE id = $1", lk.Files[2]).Scan(&key, &size); err != nil || size != 4 {
		t.Fatalf("file 2 = %d, %v", size, err)
	}
	rc, _ = store.Open(ctx, key)
	b, _ = io.ReadAll(rc)
	rc.Close()
	if string(b) != "PNG!" {
		t.Fatalf("disk bytes = %q", b)
	}
	var n int
	var inline bool
	if err := sink.db.QueryRow(ctx, "SELECT count(*) FROM attachment").Scan(&n); err != nil || n != 2 {
		t.Fatalf("attachment rows = %d, %v", n, err)
	}
	if err := sink.db.QueryRow(ctx, "SELECT inline FROM attachment WHERE thread_entry_id = $1", lk.Entries[7]).Scan(&inline); err != nil || !inline {
		t.Fatalf("inline = %v, %v", inline, err)
	}
	if !rep.NeedsAttention() {
		t.Fatal("skipped attachments need attention")
	}
}

func TestImportFilesWithoutFilesDir(t *testing.T) {
	sink, src, lk, rep := setupThroughTickets(t)
	ctx := context.Background()
	if err := sink.Step(ctx, func(w *Writer) error { return importEntries(ctx, src, w, lk, rep) }); err != nil {
		t.Fatal(err)
	}
	if err := sink.Step(ctx, func(w *Writer) error { return importFiles(ctx, src, w, lk, rep, discardStorage{}, "") }); err != nil {
		t.Fatal(err)
	}
	if rep.Counter(EntityFiles).Reasons[ReasonFilesDirMissing] != 2 || rep.Counter(EntityFiles).Written != 1 {
		t.Fatalf("files = %+v", rep.Counter(EntityFiles))
	}
}

// TestImportFilesSharedAcrossEntries checks that a source file attached to two
// different entries gets a second file row and a second copy of the bytes,
// since attachment has a UNIQUE index on file_id (one file row per attachment);
// a further attachment of that same (file, entry) pair is still a duplicate,
// not a third copy.
func TestImportFilesSharedAcrossEntries(t *testing.T) {
	sink, src, lk, rep := setupThroughTickets(t)
	ctx := context.Background()
	if err := sink.Step(ctx, func(w *Writer) error { return importEntries(ctx, src, w, lk, rep) }); err != nil {
		t.Fatal(err)
	}
	if _, err := src.db.ExecContext(ctx, "INSERT INTO ost_attachment (id, object_id, type, file_id, inline) VALUES (8, 8, 'H', 1, 0), (9, 8, 'H', 1, 0)"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := src.db.ExecContext(context.Background(), "DELETE FROM ost_attachment WHERE id IN (8, 9)"); err != nil {
			t.Fatal(err)
		}
	})
	filesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(filesDir, "fskey2"), []byte("PNG!"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := attachment.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := sink.Step(ctx, func(w *Writer) error { return importFiles(ctx, src, w, lk, rep, store, filesDir) }); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := sink.db.QueryRow(ctx, "SELECT count(*) FROM attachment").Scan(&n); err != nil || n != 3 {
		t.Fatalf("attachment rows = %d, %v", n, err)
	}
	if err := sink.db.QueryRow(ctx, "SELECT count(*) FROM file").Scan(&n); err != nil || n != 3 {
		t.Fatalf("file rows = %d, %v", n, err)
	}
	var fileID int64
	if err := sink.db.QueryRow(ctx, "SELECT file_id FROM attachment WHERE thread_entry_id = $1", lk.Entries[8]).Scan(&fileID); err != nil {
		t.Fatal(err)
	}
	if fileID == lk.Files[1] {
		t.Fatalf("expected entry 8's copy to have a fresh file id, got file 1's id %d", fileID)
	}
	var key, name, sum string
	if err := sink.db.QueryRow(ctx, "SELECT key, name, sha256 FROM file WHERE id = $1", fileID).Scan(&key, &name, &sum); err != nil {
		t.Fatal(err)
	}
	var origKey, origName, origSum string
	if err := sink.db.QueryRow(ctx, "SELECT key, name, sha256 FROM file WHERE id = $1", lk.Files[1]).Scan(&origKey, &origName, &origSum); err != nil {
		t.Fatal(err)
	}
	if key == origKey {
		t.Fatalf("expected a fresh storage key, got file 1's key %q again", key)
	}
	if name != "notes.txt" || name != origName {
		t.Fatalf("name = %q, want %q (matching file 1)", name, origName)
	}
	if sum != origSum {
		t.Fatalf("sha256 = %q, want %q (matching file 1's bytes)", sum, origSum)
	}
	if rep.Counter(EntityFiles).Written != 3 {
		t.Fatalf("files written = %d, want 3", rep.Counter(EntityFiles).Written)
	}
	found := false
	want := fmt.Sprintf("duplicate attachment on entry %d", lk.Entries[8])
	for _, s := range rep.Counter(EntityAttachments).Samples {
		if strings.Contains(s.Reason, want) {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a duplicate-attachment note for entry %d, samples = %+v", lk.Entries[8], rep.Counter(EntityAttachments).Samples)
	}
}
