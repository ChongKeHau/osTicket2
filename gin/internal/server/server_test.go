package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type pinger struct{ err error }

func (p pinger) Ping(context.Context) error { return p.err }

func TestHealth(t *testing.T) {
	r := New(Options{Pinger: pinger{}})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	if w.Code != 200 {
		t.Fatalf("healthy: %d", w.Code)
	}
	r = New(Options{Pinger: pinger{err: errors.New("down")}})
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	if w.Code != 503 {
		t.Fatalf("unhealthy: %d", w.Code)
	}
}

func TestRequestIDAndRecovery(t *testing.T) {
	r := New(Options{Pinger: pinger{}, Mount: func(public, private *gin.RouterGroup) {
		public.GET("/boom", func(c *gin.Context) { panic("kaboom") })
		public.GET("/id", func(c *gin.Context) { c.String(200, c.GetString("request_id")) })
	}})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/boom", nil))
	if w.Code != 500 || !strings.Contains(w.Body.String(), `"internal"`) {
		t.Fatalf("recovery: %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("X-Request-Id") == "" {
		t.Fatal("missing generated request id")
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/id", nil)
	req.Header.Set("X-Request-Id", "abc-123")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Body.String() != "abc-123" || w.Header().Get("X-Request-Id") != "abc-123" {
		t.Fatalf("request id passthrough: %q %q", w.Body.String(), w.Header().Get("X-Request-Id"))
	}
	req.Header.Set("X-Request-Id", strings.Repeat("x", 100))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Header().Get("X-Request-Id") == strings.Repeat("x", 100) {
		t.Fatal("overlong request id should be replaced")
	}
}

func TestMaxBodyBytes(t *testing.T) {
	r := New(Options{Pinger: pinger{}, Mount: func(public, private *gin.RouterGroup) {
		public.POST("/echo", func(c *gin.Context) {
			var body struct {
				V string `json:"v"`
			}
			if err := c.ShouldBindJSON(&body); err != nil {
				c.String(http.StatusBadRequest, "too large or malformed")
				return
			}
			c.Status(http.StatusOK)
		})
		// A route under /api/v1/files must not be capped by MaxBodyBytes: it
		// has its own, larger limit set by attachment.Handler.upload.
		public.POST("/files/echo-len", func(c *gin.Context) {
			b, err := io.ReadAll(c.Request.Body)
			if err != nil {
				c.String(http.StatusBadRequest, "err: %v", err)
				return
			}
			c.String(http.StatusOK, fmt.Sprintf("%d", len(b)))
		})
	}})

	big := bytes.Repeat([]byte("a"), 2<<20) // 2 MiB

	// A 2 MiB JSON body to a public, non-/files route is rejected: the
	// MaxBodyBytes middleware caps the body before Gin's JSON binder can
	// read all of it.
	body := append([]byte(`{"v":"`), big...)
	body = append(body, []byte(`"}`)...)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/echo", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code < 400 || w.Code >= 500 {
		t.Fatalf("oversized body to /echo must be rejected with a 4xx, got %d", w.Code)
	}

	// The same size body to /api/v1/files/... is not wrapped by the
	// middleware and is read in full.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/files/echo-len", bytes.NewReader(big))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != fmt.Sprintf("%d", len(big)) {
		t.Fatalf("/api/v1/files must not be size-capped by MaxBodyBytes: %d %q", w.Code, w.Body.String())
	}
}

func TestCORSAndAuthGroup(t *testing.T) {
	called := false
	r := New(Options{
		Pinger:      pinger{},
		CORSOrigins: []string{"http://app.test"},
		RequireAuth: func(c *gin.Context) { called = true; c.AbortWithStatus(401) },
		Mount: func(public, private *gin.RouterGroup) {
			private.GET("/secret", func(c *gin.Context) { c.Status(200) })
		},
	})
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/secret", nil)
	req.Header.Set("Origin", "http://app.test")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "http://app.test" {
		t.Fatalf("preflight: %d %v", w.Code, w.Header())
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/secret", nil)
	req.Header.Set("Origin", "http://evil.test")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("unknown origin must not be allowed")
	}
	if !called || w.Code != 401 {
		t.Fatalf("auth middleware not applied: called=%v code=%d", called, w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/nope", nil))
	if w.Code != 404 || !strings.Contains(w.Body.String(), `"not_found"`) {
		t.Fatalf("no route: %d %s", w.Code, w.Body.String())
	}
}
