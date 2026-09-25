package importer

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
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
		if f.Key == "" || filepath.Base(f.Key) != f.Key || strings.ContainsAny(f.Key, "/\\") {
			return nil, "invalid file key", nil
		}
		if filesDir == "" {
			return nil, ReasonFilesDirMissing, nil
		}
		for _, p := range []string{filepath.Join(filesDir, f.Key), filepath.Join(filesDir, f.Key[:min(2, len(f.Key))], f.Key)} {
			if fh, err := os.Open(p); err == nil {
				return fh, "", nil
			}
		}
		return nil, "file not found on disk", nil
	default:
		return nil, "unsupported storage backend " + f.Backend, nil
	}
}

func importFiles(ctx context.Context, src *Source, w *Writer, lk *Lookup, rep *Report, store attachment.Storage, filesDir string) error {
	// claimed tracks, for each source file id that has been copied at least once,
	// the target entry id of the entry that first claimed it. attachment has a
	// UNIQUE index on file_id, so a source file attached to a second, different
	// entry needs its own file row and its own copy of the bytes.
	claimed := map[int64]int64{}
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
		if size != a.File.Size {
			rep.Note(EntityFiles, a.FileID, fmt.Sprintf("size %d differs from source %d", size, a.File.Size))
		}
		name := strings.TrimSpace(a.File.Name)
		if name == "" {
			name = key
		}
		mime := a.File.Type
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

	var attachments [][]any
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

		var targetFileID int64
		firstEntry, imported := claimed[a.FileID]
		switch {
		case !imported:
			// First time this source file is attached to any imported entry.
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
			lk.Files[a.FileID] = id
			claimed[a.FileID] = entryID
			rep.Written(EntityFiles)
			targetFileID = id
		case firstEntry == entryID:
			// Same file, same entry again: attachment's PK is (thread_entry_id, file_id).
			rep.Note(EntityAttachments, a.FileID, fmt.Sprintf("duplicate attachment on entry %d", a.EntryID))
			return nil
		default:
			// Same source file, a different entry: attachment.file_id is UNIQUE, so this
			// entry needs its own copy of the file.
			rep.Read(EntityFiles)
			id, reason, err := copyFile(ctx, src, w, rep, store, filesDir, a, entryStaff[entryID])
			if err != nil {
				return err
			}
			if reason != "" {
				rep.Skip(EntityAttachments, a.FileID, reason)
				return nil
			}
			rep.Note(EntityFiles, a.FileID, fmt.Sprintf("copied again for entry %d", a.EntryID))
			rep.Written(EntityFiles)
			targetFileID = id
		}

		attachments = append(attachments, []any{entryID, targetFileID, a.Inline})
		rep.Written(EntityAttachments)
		return nil
	})
	if err != nil {
		return err
	}
	return w.Insert(ctx, "attachment", []string{"thread_entry_id", "file_id", "inline"}, attachments)
}
