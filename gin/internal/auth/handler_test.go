package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/apperr"
)

type fakeSvc struct {
	login   func(u, p string) (*Session, error)
	refresh func(raw string) (*Session, error)
	logout  func(staffID int64, raw string) error
	me      func(id int64) (*StaffProfile, error)
	load    func(id int64) (Principal, error)
}

func (f *fakeSvc) Login(_ context.Context, u, p string) (*Session, error)  { return f.login(u, p) }
func (f *fakeSvc) Refresh(_ context.Context, raw string) (*Session, error) { return f.refresh(raw) }
func (f *fakeSvc) Logout(_ context.Context, staffID int64, raw string) error {
	return f.logout(staffID, raw)
}
func (f *fakeSvc) Me(_ context.Context, id int64) (*StaffProfile, error) { return f.me(id) }
func (f *fakeSvc) LoadPrincipal(_ context.Context, id int64) (Principal, error) {
	return f.load(id)
}

func router(f *fakeSvc, tk *Tokens) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// Mirror server.New's default: trust no proxy, so gin.Context.ClientIP()
	// (which the login rate limiter keys on) can't be steered by a
	// client-supplied X-Forwarded-For/X-Real-IP. A bare gin.New() trusts
	// every proxy by default, which would make TestLoginRateLimit* pass for
	// the wrong reason.
	if err := r.SetTrustedProxies(nil); err != nil {
		panic(err)
	}
	api := r.Group("/api/v1")
	public := api.Group("")
	private := api.Group("", RequireAuth(tk, f))
	NewHandler(f).Mount(public, private)
	admin := private.Group("", RequireAdmin())
	admin.GET("/admin-only", func(c *gin.Context) { c.Status(204) })
	return r
}

func call(r *gin.Engine, method, path, body, bearer string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestLoginHandler(t *testing.T) {
	f := &fakeSvc{login: func(u, p string) (*Session, error) {
		if u == "agent" && p == "password1" {
			return &Session{AccessToken: "a", RefreshToken: "r", ExpiresIn: 900}, nil
		}
		return nil, apperr.ErrUnauthorized
	}}
	r := router(f, NewTokens(secret, time.Minute))
	if w := call(r, http.MethodPost, "/api/v1/auth/login", `{"username":"agent"}`, ""); w.Code != 400 {
		t.Fatalf("missing password: %d %s", w.Code, w.Body.String())
	}
	if w := call(r, http.MethodPost, "/api/v1/auth/login", `{"username":"agent","password":"bad"}`, ""); w.Code != 401 {
		t.Fatalf("bad password: %d", w.Code)
	}
	w := call(r, http.MethodPost, "/api/v1/auth/login", `{"username":"agent","password":"password1"}`, "")
	var sess Session
	_ = json.Unmarshal(w.Body.Bytes(), &sess)
	if w.Code != 200 || sess.AccessToken != "a" {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
}

func TestRefreshAndLogoutHandlers(t *testing.T) {
	var lastLogoutStaffID int64
	f := &fakeSvc{
		refresh: func(raw string) (*Session, error) {
			if raw == "good" {
				return &Session{AccessToken: "a2"}, nil
			}
			return nil, apperr.ErrUnauthorized
		},
		logout: func(staffID int64, raw string) error { lastLogoutStaffID = staffID; return nil },
		load:   func(id int64) (Principal, error) { return Principal{StaffID: id}, nil },
	}
	tk := NewTokens(secret, time.Minute)
	r := router(f, tk)
	if w := call(r, http.MethodPost, "/api/v1/auth/refresh", `{"refresh_token":"bad"}`, ""); w.Code != 401 {
		t.Fatalf("bad refresh: %d", w.Code)
	}
	if w := call(r, http.MethodPost, "/api/v1/auth/refresh", `{"refresh_token":"good"}`, ""); w.Code != 200 {
		t.Fatalf("good refresh: %d", w.Code)
	}
	if w := call(r, http.MethodPost, "/api/v1/auth/logout", `{"refresh_token":"good"}`, ""); w.Code != 401 {
		t.Fatalf("logout without auth: %d", w.Code)
	}
	access, _, _ := tk.IssueAccess(5, false)
	if w := call(r, http.MethodPost, "/api/v1/auth/logout", `{"refresh_token":"good"}`, access); w.Code != 204 {
		t.Fatalf("logout: %d", w.Code)
	}
	if lastLogoutStaffID != 5 {
		t.Fatalf("logout must pass the caller's own staff id, got %d", lastLogoutStaffID)
	}
}

// TestRequireAuthBearerCaseInsensitive proves the "Bearer" scheme match is
// case-insensitive, per RFC 6750/7235, while the token itself still has to
// be correct.
func TestRequireAuthBearerCaseInsensitive(t *testing.T) {
	f := &fakeSvc{
		me:   func(id int64) (*StaffProfile, error) { return &StaffProfile{ID: id, Username: "u"}, nil },
		load: func(id int64) (Principal, error) { return Principal{StaffID: id}, nil },
	}
	tk := NewTokens(secret, time.Minute)
	r := router(f, tk)
	token, _, _ := tk.IssueAccess(3, false)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("lowercase bearer scheme must be accepted: %d %s", w.Code, w.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "BEARER "+token)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("uppercase bearer scheme must be accepted: %d %s", w.Code, w.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Basic "+token)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("a non-bearer scheme must still be rejected: %d", w.Code)
	}
}

// TestLoginRateLimiting proves the login handler blocks after 10 failed
// attempts for the same username+client within the window, that a different
// username is an independent counter, and that a success resets it.
func TestLoginRateLimiting(t *testing.T) {
	f := &fakeSvc{login: func(u, p string) (*Session, error) {
		if p == "password1" {
			return &Session{AccessToken: "a"}, nil
		}
		return nil, apperr.ErrUnauthorized
	}}
	r := router(f, NewTokens(secret, time.Minute))

	for i := 0; i < 10; i++ {
		if w := call(r, http.MethodPost, "/api/v1/auth/login", `{"username":"bob","password":"bad"}`, ""); w.Code != 401 {
			t.Fatalf("attempt %d: %d", i+1, w.Code)
		}
	}
	if w := call(r, http.MethodPost, "/api/v1/auth/login", `{"username":"bob","password":"bad"}`, ""); w.Code != 429 {
		t.Fatalf("11th failed attempt must be rate limited: %d", w.Code)
	}

	if w := call(r, http.MethodPost, "/api/v1/auth/login", `{"username":"alice","password":"bad"}`, ""); w.Code != 401 {
		t.Fatalf("a different username must not be limited: %d", w.Code)
	}

	for i := 0; i < 9; i++ {
		call(r, http.MethodPost, "/api/v1/auth/login", `{"username":"carol","password":"bad"}`, "")
	}
	if w := call(r, http.MethodPost, "/api/v1/auth/login", `{"username":"carol","password":"password1"}`, ""); w.Code != 200 {
		t.Fatalf("success on the 10th attempt: %d %s", w.Code, w.Body.String())
	}
	if w := call(r, http.MethodPost, "/api/v1/auth/login", `{"username":"carol","password":"bad"}`, ""); w.Code != 401 {
		t.Fatalf("a success must reset the counter, got rate limited: %d", w.Code)
	}
}

// TestLoginRateLimitIgnoresForgedXForwardedFor proves the rate limiter's
// client-IP key can't be reset by rotating X-Forwarded-For: with no trusted
// proxies configured (router()'s default, matching server.New's), gin's
// ClientIP() ignores the header and always returns the actual remote
// address, so 10 failures from the same connection still trip the limit
// even though each request claims a different forged IP.
func TestLoginRateLimitIgnoresForgedXForwardedFor(t *testing.T) {
	f := &fakeSvc{login: func(u, p string) (*Session, error) { return nil, apperr.ErrUnauthorized }}
	r := router(f, NewTokens(secret, time.Minute))

	post := func(forgedIP string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"dave","password":"bad"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", forgedIP)
		req.RemoteAddr = "192.0.2.50:1234" // the one real, constant remote address
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	for i := 0; i < 10; i++ {
		if w := post(fmt.Sprintf("203.0.113.%d", i)); w.Code != 401 {
			t.Fatalf("attempt %d with forged IP: %d", i+1, w.Code)
		}
	}
	if w := post("203.0.113.250"); w.Code != 429 {
		t.Fatalf("11th attempt, still with a fresh forged X-Forwarded-For, must be rate limited: %d", w.Code)
	}
}

func TestAuthMiddlewareAndMe(t *testing.T) {
	f := &fakeSvc{
		me: func(id int64) (*StaffProfile, error) { return &StaffProfile{ID: id, Username: "u"}, nil },
		load: func(id int64) (Principal, error) {
			if id == 9 {
				return Principal{}, apperr.ErrUnauthorized
			}
			return Principal{StaffID: id, IsAdmin: id == 1}, nil
		},
	}
	tk := NewTokens(secret, time.Minute)
	r := router(f, tk)
	if w := call(r, http.MethodGet, "/api/v1/me", "", ""); w.Code != 401 {
		t.Fatalf("no token: %d", w.Code)
	}
	if w := call(r, http.MethodGet, "/api/v1/me", "", "garbage"); w.Code != 401 {
		t.Fatalf("bad token: %d", w.Code)
	}
	deactivated, _, _ := tk.IssueAccess(9, false)
	if w := call(r, http.MethodGet, "/api/v1/me", "", deactivated); w.Code != 401 {
		t.Fatalf("deactivated staff: %d", w.Code)
	}
	agent, _, _ := tk.IssueAccess(2, false)
	w := call(r, http.MethodGet, "/api/v1/me", "", agent)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"username":"u"`) {
		t.Fatalf("me: %d %s", w.Code, w.Body.String())
	}
	if w := call(r, http.MethodGet, "/api/v1/admin-only", "", agent); w.Code != 403 {
		t.Fatalf("agent on admin route: %d", w.Code)
	}
	admin, _, _ := tk.IssueAccess(1, true)
	if w := call(r, http.MethodGet, "/api/v1/admin-only", "", admin); w.Code != 204 {
		t.Fatalf("admin on admin route: %d", w.Code)
	}
}
