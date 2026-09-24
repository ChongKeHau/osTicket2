package auth

import (
	"context"
	"encoding/json"
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
	logout  func(raw string) error
	me      func(id int64) (*StaffProfile, error)
	load    func(id int64) (Principal, error)
}

func (f *fakeSvc) Login(_ context.Context, u, p string) (*Session, error)  { return f.login(u, p) }
func (f *fakeSvc) Refresh(_ context.Context, raw string) (*Session, error) { return f.refresh(raw) }
func (f *fakeSvc) Logout(_ context.Context, raw string) error              { return f.logout(raw) }
func (f *fakeSvc) Me(_ context.Context, id int64) (*StaffProfile, error)   { return f.me(id) }
func (f *fakeSvc) LoadPrincipal(_ context.Context, id int64) (Principal, error) {
	return f.load(id)
}

func router(f *fakeSvc, tk *Tokens) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
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
	f := &fakeSvc{
		refresh: func(raw string) (*Session, error) {
			if raw == "good" {
				return &Session{AccessToken: "a2"}, nil
			}
			return nil, apperr.ErrUnauthorized
		},
		logout: func(raw string) error { return nil },
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
