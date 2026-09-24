package server

import (
	"context"
	"errors"
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
