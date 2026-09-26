package attachment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/jackc/pgx/v5"
)

type File struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Mime string `json:"mime"`
	Size int64  `json:"size"`
}

type Service interface {
	Upload(ctx context.Context, p auth.Principal, name, mime string, r io.Reader) (*File, error)
	Download(ctx context.Context, p auth.Principal, id int64) (*File, io.ReadCloser, error)
	GC(ctx context.Context, olderThan time.Duration) (int, error)

	// UploadAnonymous stores a file with no staff uploader (uploaded_by NULL):
	// portal uploads, which become owned by the ticket they are attached to.
	UploadAnonymous(ctx context.Context, name, mime string, r io.Reader) (*File, error)
	// DownloadForTicket opens a file only when it is attached to a
	// customer-visible entry (message or response, never a note) of ticketID;
	// anything else is ErrNotFound. The caller has already checked that the
	// requester may see the ticket.
	DownloadForTicket(ctx context.Context, ticketID, fileID int64) (*File, io.ReadCloser, error)
}

type service struct {
	db       db.Beginner
	store    Storage
	maxBytes int64
	allowed  map[string]bool
}

func NewService(b db.Beginner, store Storage, maxBytes int64, allowedMIME []string) Service {
	allowed := make(map[string]bool, len(allowedMIME))
	for _, m := range allowedMIME {
		allowed[strings.ToLower(strings.TrimSpace(m))] = true
	}
	return &service{db: b, store: store, maxBytes: maxBytes, allowed: allowed}
}

func (s *service) Upload(ctx context.Context, p auth.Principal, name, mime string, r io.Reader) (*File, error) {
	uploader := p.StaffID
	return s.upload(ctx, &uploader, name, mime, r)
}

func (s *service) UploadAnonymous(ctx context.Context, name, mime string, r io.Reader) (*File, error) {
	return s.upload(ctx, nil, name, mime, r)
}

func (s *service) upload(ctx context.Context, uploader *int64, name, mime string, r io.Reader) (*File, error) {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "" || name == "." || name == "/" || name == ".." {
		return nil, apperr.Validation("file", "filename is required")
	}
	// Drop any invalid UTF-8 first, then truncate over-length names on a
	// rune boundary, keeping the tail (not the head) so an extension like
	// .txt/.pdf survives.
	name = strings.ToValidUTF8(name, "")
	if len(name) > 255 {
		i := len(name) - 255
		for i < len(name) && !utf8.RuneStart(name[i]) {
			i++
		}
		name = name[i:]
	}
	// Re-check after cleaning: an all-invalid or now-degenerate name (e.g.
	// entirely invalid UTF-8) must still be rejected, not silently uploaded
	// as an empty name.
	if name == "" || name == "." || name == "/" || name == ".." {
		return nil, apperr.Validation("file", "filename is required")
	}
	mime = strings.ToLower(strings.TrimSpace(strings.Split(mime, ";")[0]))
	if !s.allowed[mime] {
		return nil, apperr.Validation("file", "file type "+mime+" is not allowed")
	}
	var kb [16]byte
	if _, err := rand.Read(kb[:]); err != nil {
		return nil, err
	}
	key := hex.EncodeToString(kb[:])
	size, sum, err := s.store.Put(ctx, key, io.LimitReader(r, s.maxBytes+1))
	if err != nil {
		return nil, err
	}
	if size > s.maxBytes {
		_ = s.store.Delete(ctx, key)
		return nil, fmt.Errorf("%w: file exceeds %d bytes", apperr.ErrPayloadTooLarge, s.maxBytes)
	}
	row, err := db.New(s.db).CreateFile(ctx, db.CreateFileParams{
		Key: key, Name: name, Mime: mime, Size: size, Sha256: sum, Backend: "local", UploadedBy: uploader,
	})
	if err != nil {
		_ = s.store.Delete(ctx, key)
		return nil, err
	}
	return &File{ID: row.ID, Name: row.Name, Mime: row.Mime, Size: row.Size}, nil
}

func (s *service) Download(ctx context.Context, p auth.Principal, id int64) (*File, io.ReadCloser, error) {
	q := db.New(s.db)
	row, err := q.GetFile(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, fmt.Errorf("file %d: %w", id, apperr.ErrNotFound)
	}
	if err != nil {
		return nil, nil, err
	}
	deptID, err := q.FileTicketDeptID(ctx, id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// not attached yet: only the uploader or an admin may read it
		if !p.IsAdmin && (row.UploadedBy == nil || *row.UploadedBy != p.StaffID) {
			return nil, nil, fmt.Errorf("file %d: %w", id, apperr.ErrNotFound)
		}
	case err != nil:
		return nil, nil, err
	default:
		if !p.CanSeeDept(deptID) {
			return nil, nil, fmt.Errorf("file %d: %w", id, apperr.ErrNotFound)
		}
	}
	return s.open(ctx, row)
}

func (s *service) DownloadForTicket(ctx context.Context, ticketID, fileID int64) (*File, io.ReadCloser, error) {
	q := db.New(s.db)
	ok, err := q.FileOnCustomerEntry(ctx, db.FileOnCustomerEntryParams{FileID: fileID, TicketID: ticketID})
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, fmt.Errorf("file %d: %w", fileID, apperr.ErrNotFound)
	}
	row, err := q.GetFile(ctx, fileID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, fmt.Errorf("file %d: %w", fileID, apperr.ErrNotFound)
	}
	if err != nil {
		return nil, nil, err
	}
	return s.open(ctx, row)
}

func (s *service) open(ctx context.Context, row db.File) (*File, io.ReadCloser, error) {
	rc, err := s.store.Open(ctx, row.Key)
	if err != nil {
		return nil, nil, fmt.Errorf("open blob %s: %w", row.Key, err)
	}
	return &File{ID: row.ID, Name: row.Name, Mime: row.Mime, Size: row.Size}, rc, nil
}

func (s *service) GC(ctx context.Context, olderThan time.Duration) (int, error) {
	q := db.New(s.db)
	rows, err := q.ListUnattachedFilesBefore(ctx, time.Now().Add(-olderThan))
	if err != nil {
		return 0, err
	}
	n := 0
	for _, row := range rows {
		affected, err := q.DeleteUnattachedFile(ctx, row.ID)
		// Belt and braces beside the query's own WHERE NOT EXISTS guard: a
		// foreign key violation (a concurrent attach winning the race
		// between the list and the delete) is treated the same as the
		// guard returning 0 rows -- skipped, not aborting the whole run.
		if db.IsForeignKeyViolation(err) {
			continue
		}
		if err != nil {
			return n, err
		}
		if affected == 0 {
			// a concurrent request attached this file between the list and
			// the delete; leave its blob alone and don't count it.
			continue
		}
		if err := s.store.Delete(ctx, row.Key); err != nil {
			return n, fmt.Errorf("delete blob %s: %w", row.Key, err)
		}
		n++
	}
	return n, nil
}
