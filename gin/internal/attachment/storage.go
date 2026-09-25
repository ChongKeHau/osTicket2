// Package attachment stores uploaded files and links them to thread entries.
package attachment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

type Storage interface {
	Put(ctx context.Context, key string, r io.Reader) (size int64, sha256Hex string, err error)
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

var keyRe = regexp.MustCompile(`^[a-f0-9]{6,64}$`)

// LocalStorage keeps files under dir/<first two chars of key>/<key>.
type LocalStorage struct{ dir string }

func NewLocalStorage(dir string) (*LocalStorage, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("storage dir: %w", err)
	}
	return &LocalStorage{dir: dir}, nil
}

func (s *LocalStorage) path(key string) (string, error) {
	if !keyRe.MatchString(key) {
		return "", errors.New("invalid storage key")
	}
	return filepath.Join(s.dir, key[:2], key), nil
}

func (s *LocalStorage) Put(_ context.Context, key string, r io.Reader) (int64, string, error) {
	p, err := s.path(key)
	if err != nil {
		return 0, "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return 0, "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".upload-*")
	if err != nil {
		return 0, "", err
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), r)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return 0, "", err
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

func (s *LocalStorage) Open(_ context.Context, key string) (io.ReadCloser, error) {
	p, err := s.path(key)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}

func (s *LocalStorage) Delete(_ context.Context, key string) error {
	p, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
