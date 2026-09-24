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
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "" || name == "." || name == "/" {
		return nil, apperr.Validation("file", "filename is required")
	}
	if len(name) > 255 {
		name = name[len(name)-255:]
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
	uploader := p.StaffID
	row, err := db.New(s.db).CreateFile(ctx, db.CreateFileParams{
		Key: key, Name: name, Mime: mime, Size: size, Sha256: sum, Backend: "local", UploadedBy: &uploader,
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
		if err := s.store.Delete(ctx, row.Key); err != nil {
			return n, fmt.Errorf("delete blob %s: %w", row.Key, err)
		}
		if err := q.DeleteFile(ctx, row.ID); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
