package attachment

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
)

type fake struct {
	lastName string
	lastMime string
}

func (f *fake) Upload(_ context.Context, p auth.Principal, name, mime string, r io.Reader) (*File, error) {
	f.lastName, f.lastMime = name, mime
	b, _ := io.ReadAll(r)
	if mime != "text/plain" {
		return nil, apperr.Validation("file", "not allowed")
	}
	return &File{ID: 3, Name: name, Mime: mime, Size: int64(len(b))}, nil
}
func (f *fake) Download(_ context.Context, p auth.Principal, id int64) (*File, io.ReadCloser, error) {
	if id != 3 {
		return nil, nil, apperr.ErrNotFound
	}
	return &File{ID: 3, Name: "../../evil name.txt", Mime: "text/plain", Size: 5}, io.NopCloser(strings.NewReader("hello")), nil
}
func (f *fake) GC(context.Context, time.Duration) (int, error) { return 0, nil }
func (f *fake) UploadAnonymous(context.Context, string, string, io.Reader) (*File, error) {
	return nil, apperr.ErrForbidden
}
func (f *fake) DownloadForTicket(context.Context, int64, int64) (*File, io.ReadCloser, error) {
	return nil, nil, apperr.ErrNotFound
}

func newRouter(f *fake) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	private := r.Group("/api/v1", func(c *gin.Context) {
		auth.WithPrincipal(c, auth.Principal{StaffID: 1})
	})
	NewHandler(f, 1<<20).Mount(private)
	return r
}

func multipartBody(t *testing.T, field, filename, mime, content string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{`form-data; name="` + field + `"; filename="` + filename + `"`}
	h["Content-Type"] = []string{mime}
	part, err := w.CreatePart(h)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte(content))
	_ = w.Close()
	return &buf, w.FormDataContentType()
}

func TestUploadHandler(t *testing.T) {
	f := &fake{}
	r := newRouter(f)
	body, ctype := multipartBody(t, "file", "notes.txt", "text/plain", "hello")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/files", body)
	req.Header.Set("Content-Type", ctype)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 201 || !strings.Contains(w.Body.String(), `"id":3`) || f.lastName != "notes.txt" || f.lastMime != "text/plain" {
		t.Fatalf("upload: %d %s name=%q mime=%q", w.Code, w.Body.String(), f.lastName, f.lastMime)
	}
	body, ctype = multipartBody(t, "wrong", "notes.txt", "text/plain", "hello")
	req = httptest.NewRequest(http.MethodPost, "/api/v1/files", body)
	req.Header.Set("Content-Type", ctype)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("missing file field: %d", w.Code)
	}
	body, ctype = multipartBody(t, "file", "x.bin", "application/octet-stream", "hello")
	req = httptest.NewRequest(http.MethodPost, "/api/v1/files", body)
	req.Header.Set("Content-Type", ctype)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("disallowed mime: %d", w.Code)
	}
}

func TestDownloadHandler(t *testing.T) {
	r := newRouter(&fake{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/files/3", nil))
	if w.Code != 200 || w.Body.String() != "hello" {
		t.Fatalf("download: %d %q", w.Code, w.Body.String())
	}
	cd := w.Header().Get("Content-Disposition")
	if !strings.Contains(cd, `filename="evil name.txt"`) || strings.Contains(cd, "..") {
		t.Fatalf("content-disposition must use base name: %q", cd)
	}
	if w.Header().Get("Content-Type") != "text/plain" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("headers: %v", w.Header())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/files/4", nil))
	if w.Code != 404 {
		t.Fatalf("missing: %d", w.Code)
	}
}
