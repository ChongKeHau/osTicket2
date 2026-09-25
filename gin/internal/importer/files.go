package importer

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/grandpine/ticket-api/internal/attachment"
)

// newStorageKey mirrors the upload service: 32 random bytes, hex encoded.
func newStorageKey() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// discardStorage hashes and counts bytes without keeping them (dry runs).
type discardStorage struct{}

func (discardStorage) Put(_ context.Context, _ string, r io.Reader) (int64, string, error) {
	h := sha256.New()
	n, err := io.Copy(h, r)
	if err != nil {
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

func (discardStorage) Open(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("dry run: nothing stored")
}

func (discardStorage) Delete(context.Context, string) error { return nil }

var _ attachment.Storage = discardStorage{}

// openSourceFile returns the bytes of an osTicket file from its backend.
func openSourceFile(ctx context.Context, src *Source, f SrcFile, filesDir string) (io.ReadCloser, string, error) {
	switch f.Backend {
	case "D":
		rc, err := src.OpenChunks(ctx, f.ID)
		return rc, "", err
	case "F":
		k := f.Key
		if k == "" || k == "." || k == ".." || filepath.Base(k) != k || strings.ContainsAny(k, "/\\") {
			return nil, "invalid file key", nil
		}
		if filesDir == "" {
			return nil, ReasonFilesDirMissing, nil
		}
		// The storage-fs plugin nests files one key character per level up to
		// its configured depth; a two-character first level is tried last.
		candidates := []string{filepath.Join(filesDir, k)}
		for depth := 1; depth <= min(3, len(k)); depth++ {
			parts := []string{filesDir}
			for i := 0; i < depth; i++ {
				parts = append(parts, k[i:i+1])
			}
			candidates = append(candidates, filepath.Join(append(parts, k)...))
		}
		if len(k) >= 2 {
			candidates = append(candidates, filepath.Join(filesDir, k[:2], k))
		}
		for _, p := range candidates {
			if fi, err := os.Stat(p); err != nil || !fi.Mode().IsRegular() {
				continue
			}
			if fh, err := os.Open(p); err == nil {
				return fh, "", nil
			}
		}
		return nil, "file not found on disk", nil
	default:
		return nil, "unsupported storage backend " + f.Backend, nil
	}
}

// removeBlobs deletes stored files whose rows were rolled back. It is best
// effort: failures are logged, not returned.
func removeBlobs(store attachment.Storage, keys []string) {
	for _, k := range keys {
		// A fresh context: the step's may be the one that was cancelled.
		if err := store.Delete(context.Background(), k); err != nil {
			slog.Warn("import: could not remove stored file after a failed step", "key", k, "err", err)
		}
	}
}

// importFiles copies attached files into store and inserts their file and
// attachment rows. Blobs are stored before their rows, so when the step fails
// (and its rows roll back) the blobs it wrote are deleted again.
func importFiles(ctx context.Context, src *Source, w *Writer, lk *Lookup, rep *Report, store attachment.Storage, filesDir string) (err error) {
	var stored []string
	defer func() {
		if err != nil {
			removeBlobs(store, stored)
		}
	}()
	// claimed tracks, for each source file id, every target entry that already
	// has its own copy of it (target entry id -> target file id). attachment has
	// a UNIQUE index on file_id, so a source file attached to a second, different
	// entry needs its own file row and its own copy of the bytes; the same
	// (file, entry) pair seen again is a duplicate attachment row.
	claimed := map[int64]map[int64]int64{}
	// Files that could not be copied, so later attachments of the same file skip with the same reason.
	failed := map[int64]string{}
	entryStaff := map[int64]*int64{}
	rows, err := w.Query(ctx, "SELECT id, staff_id FROM thread_entry")
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		var staff *int64
		if err := rows.Scan(&id, &staff); err != nil {
			rows.Close()
			return err
		}
		entryStaff[id] = staff
	}
	rows.Close()

	now := time.Now()

	// copyFile reads the source file's bytes, stores them under a fresh key, and
	// inserts a new file row for them, returning its target id. It is used both
	// for a source file's first copy and for any later copy a different claiming
	// entry needs.
	copyFile := func(ctx context.Context, src *Source, w *Writer, rep *Report, store attachment.Storage, filesDir string, a SrcAttachment, uploadedBy *int64) (int64, string, error) {
		rc, reason, err := openSourceFile(ctx, src, a.File, filesDir)
		if err != nil {
			return 0, "", err
		}
		if reason != "" {
			return 0, reason, nil
		}
		key, err := newStorageKey()
		if err != nil {
			rc.Close()
			return 0, "", err
		}
		size, sum, err := store.Put(ctx, key, rc)
		rc.Close()
		if err != nil {
			return 0, "", fmt.Errorf("store file %d: %w", a.FileID, err)
		}
		stored = append(stored, key)
		if size != a.File.Size {
			rep.Note(EntityFiles, a.FileID, fmt.Sprintf("size %d differs from source %d", size, a.File.Size))
		}
		// Each target file row belongs to one attachment, so the attachment's
		// own name (osTicket lets it differ from the file's) wins when set.
		var tc textCleaner
		name := strings.TrimSpace(tc.clean(a.Name))
		if name == "" {
			name = strings.TrimSpace(tc.clean(a.File.Name))
		}
		if name == "" {
			name = key
		}
		mime := tc.clean(a.File.Type)
		tc.note(rep, EntityFiles, a.FileID)
		if mime == "" {
			mime = "application/octet-stream"
		}
		id := lk.allocID("file", a.FileID)
		if err := w.Insert(ctx, "file", []string{"id", "key", "name", "mime", "size", "sha256", "backend", "uploaded_by", "created_at"},
			[][]any{{id, key, name, mime, size, sum, "local", uploadedBy, orZero(a.File.Created, now)}}); err != nil {
			return 0, "", err
		}
		return id, "", nil
	}

	attachments := w.NewBatcher("attachment", []string{"thread_entry_id", "file_id", "inline"})
	err = src.Attachments(ctx, func(a SrcAttachment) error {
		rep.Read(EntityAttachments)
		entryID, ok := lk.Entries[a.EntryID]
		if !ok {
			rep.Note(EntityAttachments, a.FileID, fmt.Sprintf("entry %d not imported", a.EntryID))
			return nil
		}
		if reason, ok := failed[a.FileID]; ok {
			rep.Skip(EntityAttachments, a.FileID, reason)
			return nil
		}
		if _, ok := claimed[a.FileID][entryID]; ok {
			// Same source file, same entry again: attachment's PK is (thread_entry_id, file_id).
			rep.Note(EntityAttachments, a.FileID, fmt.Sprintf("duplicate attachment on entry %d", a.EntryID))
			return nil
		}

		// A source file's first claimant keeps the original copy; every later,
		// different entry needs its own copy, since attachment.file_id is UNIQUE.
		first := len(claimed[a.FileID]) == 0
		rep.Read(EntityFiles)
		id, reason, err := copyFile(ctx, src, w, rep, store, filesDir, a, entryStaff[entryID])
		if err != nil {
			return err
		}
		if reason != "" {
			failed[a.FileID] = reason
			rep.Skip(EntityFiles, a.FileID, reason)
			rep.Skip(EntityAttachments, a.FileID, reason)
			return nil
		}
		if first {
			lk.Files[a.FileID] = id
		} else {
			rep.Note(EntityFiles, a.FileID, fmt.Sprintf("copied again for entry %d", a.EntryID))
		}
		rep.Written(EntityFiles)
		if claimed[a.FileID] == nil {
			claimed[a.FileID] = map[int64]int64{}
		}
		claimed[a.FileID][entryID] = id

		rep.Written(EntityAttachments)
		return attachments.Add(ctx, []any{entryID, id, a.Inline})
	})
	if err != nil {
		return err
	}
	return attachments.Flush(ctx)
}
