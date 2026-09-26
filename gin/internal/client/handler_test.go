package client

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

// fakeIdentity records calls; known addresses are the only ones it "mails".
type fakeIdentity struct {
	fakeLoader
	known    map[string]bool
	mailed   []string
	password []Principal
	logout   []string
	// scheduled counts background requests handed to the service.
	scheduled int
}

// Register mirrors the service: an address that already has an account gets no mail.
func (f *fakeIdentity) Register(in RegisterInput) {
	f.scheduled++
	if !f.known[in.Email] {
		f.mailed = append(f.mailed, "register:"+in.Email)
	}
}

func (f *fakeIdentity) Login(_ context.Context, email, password string) (*Session, error) {
	if f.known[email] && password == "secret123" {
		return &Session{AccessToken: "a", RefreshToken: "r", ExpiresIn: 900, User: Profile{ID: 5, Email: email}}, nil
	}
	return nil, apperr.ErrUnauthorized
}

// mail records a scheduled request; only known addresses get mail.
func (f *fakeIdentity) mail(kind, email string) {
	f.scheduled++
	if f.known[email] {
		f.mailed = append(f.mailed, kind+":"+email)
	}
}

func (f *fakeIdentity) RequestLink(email string)           { f.mail("link", email) }
func (f *fakeIdentity) RequestReset(email string)          { f.mail("reset", email) }
func (f *fakeIdentity) RequestAccess(email, number string) { f.mail("access#"+number, email) }

func (f *fakeIdentity) Exchange(_ context.Context, raw string) (*Session, error) {
	if raw == "good" {
		return &Session{AccessToken: "a", RefreshToken: "r", Kind: "signin", User: Profile{ID: 5}}, nil
	}
	return nil, apperr.ErrTokenInvalid
}

func (f *fakeIdentity) Refresh(_ context.Context, raw string) (*Session, error) {
	if raw == "r" {
		return &Session{AccessToken: "a2", RefreshToken: "r2"}, nil
	}
	return nil, apperr.ErrUnauthorized
}

func (f *fakeIdentity) Logout(_ context.Context, userID int64, raw string) error {
	f.logout = append(f.logout, raw)
	return nil
}

func (f *fakeIdentity) Me(_ context.Context, userID int64) (*Profile, error) {
	return &Profile{ID: userID, Email: "u@x.test", Name: "U", Verified: true}, nil
}

func (f *fakeIdentity) UpdateName(_ context.Context, userID int64, name string) (*Profile, error) {
	return &Profile{ID: userID, Name: name}, nil
}

func (f *fakeIdentity) SetPassword(_ context.Context, p Principal, in PasswordInput) error {
	if in.CurrentPassword == "" && !p.PasswordReset {
		return apperr.Validation("current_password", "required")
	}
	f.password = append(f.password, p)
	return nil
}

type harness struct {
	r      *gin.Engine
	svc    *fakeIdentity
	tokens *Tokens
}

func newHarness(limit int) *harness {
	gin.SetMode(gin.TestMode)
	svc := &fakeIdentity{known: map[string]bool{"pat@x.test": true}}
	tokens := NewTokens(secret, time.Minute)
	r := gin.New()
	NewHandler(svc, NewLimiter(limit, time.Minute)).MountAuth(r.Group("/portal"), tokens)
	return &harness{r: r, svc: svc, tokens: tokens}
}

func (h *harness) call(method, path, tok, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	w := httptest.NewRecorder()
	h.r.ServeHTTP(w, req)
	return w
}

type envelope struct {
	Error struct {
		Code   string            `json:"code"`
		Fields map[string]string `json:"fields"`
	} `json:"error"`
}

func decodeErr(t *testing.T, w *httptest.ResponseRecorder) envelope {
	t.Helper()
	var e envelope
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatalf("body %q: %v", w.Body.String(), err)
	}
	return e
}

func TestHandlerRequestsAlwaysAccepted(t *testing.T) {
	h := newHarness(100)
	// Known and unknown addresses get byte-identical responses; register
	// answers 201 even for an address that already has an account.
	cases := []struct {
		path, known, unknown string
		status               int
	}{
		{"/portal/auth/link", `{"email":"pat@x.test"}`, `{"email":"ghost@x.test"}`, http.StatusAccepted},
		{"/portal/auth/reset", `{"email":"pat@x.test"}`, `{"email":"ghost@x.test"}`, http.StatusAccepted},
		{"/portal/access", `{"email":"pat@x.test","number":"100001"}`, `{"email":"ghost@x.test","number":"100001"}`, http.StatusAccepted},
		{"/portal/auth/register", `{"email":"pat@x.test","name":"Pat"}`, `{"email":"ghost@x.test","name":"Ghost"}`, http.StatusCreated},
	}
	for _, c := range cases {
		wk := h.call(http.MethodPost, c.path, "", c.known)
		wu := h.call(http.MethodPost, c.path, "", c.unknown)
		if wk.Code != c.status || wu.Code != c.status || wk.Body.String() != "{}" || wu.Body.String() != "{}" {
			t.Fatalf("%s: known %d %s, unknown %d %s", c.path, wk.Code, wk.Body.String(), wu.Code, wu.Body.String())
		}
		if len(wk.Header()) != len(wu.Header()) {
			t.Fatalf("%s: headers differ %v vs %v", c.path, wk.Header(), wu.Header())
		}
	}
	if h.svc.scheduled != 8 {
		t.Fatalf("scheduled %d requests, want 8", h.svc.scheduled)
	}
	want := []string{"link:pat@x.test", "reset:pat@x.test", "access#100001:pat@x.test", "register:ghost@x.test"}
	if strings.Join(h.svc.mailed, ",") != strings.Join(want, ",") {
		t.Fatalf("mailed %v", h.svc.mailed)
	}
	if w := h.call(http.MethodPost, "/portal/auth/link", "", `{"email":"not-an-address"}`); w.Code != 400 {
		t.Fatalf("malformed email: %d", w.Code)
	}
	if w := h.call(http.MethodPost, "/portal/access", "", `{"email":"pat@x.test"}`); w.Code != 400 || decodeErr(t, w).Error.Fields["number"] == "" {
		t.Fatalf("missing number: %d %s", w.Code, w.Body.String())
	}
}

func TestHandlerLogin(t *testing.T) {
	h := newHarness(100)
	w := h.call(http.MethodPost, "/portal/auth/login", "", `{"email":"pat@x.test","password":"secret123"}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"refresh_token":"r"`) {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	for _, body := range []string{
		`{"email":"pat@x.test","password":"wrong"}`,
		`{"email":"ghost@x.test","password":"secret123"}`,
	} {
		w := h.call(http.MethodPost, "/portal/auth/login", "", body)
		e := decodeErr(t, w)
		if w.Code != 401 || e.Error.Code != "unauthorized" || e.Error.Fields != nil || strings.Contains(w.Body.String(), "fields") {
			t.Fatalf("bad login %s: %d %s", body, w.Code, w.Body.String())
		}
	}
}

func TestHandlerRateLimit(t *testing.T) {
	h := newHarness(1)
	if w := h.call(http.MethodPost, "/portal/auth/link", "", `{"email":"pat@x.test"}`); w.Code != 202 {
		t.Fatalf("first: %d", w.Code)
	}
	w := h.call(http.MethodPost, "/portal/auth/link", "", `{"email":"pat@x.test"}`)
	e := decodeErr(t, w)
	if w.Code != 429 || e.Error.Code != "rate_limited" || e.Error.Fields["retry_after"] == "" || e.Error.Fields["retry_after"] == "0" {
		t.Fatalf("second: %d %s", w.Code, w.Body.String())
	}
	if h.svc.scheduled != 1 {
		t.Fatalf("rate-limited request reached the service: %d", h.svc.scheduled)
	}
	// Login shares the budget and is checked before the password.
	w = h.call(http.MethodPost, "/portal/auth/login", "", `{"email":"pat@x.test","password":"secret123"}`)
	if w.Code != 429 {
		t.Fatalf("login over budget: %d %s", w.Code, w.Body.String())
	}
}

func TestHandlerRegisterRateLimited(t *testing.T) {
	h := newHarness(1)
	if w := h.call(http.MethodPost, "/portal/auth/register", "", `{"email":"new@x.test","name":"New"}`); w.Code != 201 {
		t.Fatalf("first: %d %s", w.Code, w.Body.String())
	}
	// Same address from another IP: the per-address budget trips.
	req := httptest.NewRequest(http.MethodPost, "/portal/auth/register", strings.NewReader(`{"email":"NEW@x.test","name":"New"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.9:1234"
	w := httptest.NewRecorder()
	h.r.ServeHTTP(w, req)
	e := decodeErr(t, w)
	if w.Code != 429 || e.Error.Code != "rate_limited" || e.Error.Fields["retry_after"] == "" {
		t.Fatalf("second by address: %d %s", w.Code, w.Body.String())
	}
	// Another address from the first IP: the per-IP budget trips.
	w = h.call(http.MethodPost, "/portal/auth/register", "", `{"email":"other@x.test","name":"O"}`)
	if w.Code != 429 {
		t.Fatalf("second by ip: %d %s", w.Code, w.Body.String())
	}
	if h.svc.scheduled != 1 {
		t.Fatalf("rate-limited registration reached the service: %d", h.svc.scheduled)
	}
}

func TestHandlerExchangeRegisterRefresh(t *testing.T) {
	h := newHarness(100)
	w := h.call(http.MethodPost, "/portal/auth/exchange", "", `{"token":"bad"}`)
	if w.Code != 410 || decodeErr(t, w).Error.Code != "token_invalid" {
		t.Fatalf("bad token: %d %s", w.Code, w.Body.String())
	}
	if w := h.call(http.MethodPost, "/portal/auth/exchange", "", `{"token":"good"}`); w.Code != 200 || !strings.Contains(w.Body.String(), `"kind":"signin"`) {
		t.Fatalf("good token: %d %s", w.Code, w.Body.String())
	}
	if w := h.call(http.MethodPost, "/portal/auth/exchange", "", `{}`); w.Code != 400 {
		t.Fatalf("missing token: %d", w.Code)
	}
	if w := h.call(http.MethodPost, "/portal/auth/register", "", `{"email":"new@x.test"}`); w.Code != 400 || decodeErr(t, w).Error.Fields["name"] == "" {
		t.Fatalf("register without name: %d %s", w.Code, w.Body.String())
	}
	if w := h.call(http.MethodPost, "/portal/auth/register", "", `{"email":"nope","name":"N"}`); w.Code != 400 || decodeErr(t, w).Error.Fields["email"] == "" {
		t.Fatalf("register bad email: %d %s", w.Code, w.Body.String())
	}
	if w := h.call(http.MethodPost, "/portal/auth/refresh", "", `{"refresh_token":"r"}`); w.Code != 200 || !strings.Contains(w.Body.String(), `"r2"`) {
		t.Fatalf("refresh: %d %s", w.Code, w.Body.String())
	}
	if w := h.call(http.MethodPost, "/portal/auth/refresh", "", `{"refresh_token":"x"}`); w.Code != 401 {
		t.Fatalf("bad refresh: %d", w.Code)
	}
}

func TestHandlerSignedInRoutes(t *testing.T) {
	h := newHarness(100)
	user, _, _ := h.tokens.IssueAccess(5, nil, false)
	tid := int64(9)
	guest, _, _ := h.tokens.IssueAccess(5, &tid, false)
	reset, _, _ := h.tokens.IssueAccess(5, nil, true)

	if w := h.call(http.MethodGet, "/portal/me", "", ""); w.Code != 401 {
		t.Fatalf("me without token: %d", w.Code)
	}
	if w := h.call(http.MethodGet, "/portal/me", user, ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"id":5`) {
		t.Fatalf("me: %d %s", w.Code, w.Body.String())
	}
	for _, rc := range []struct{ method, path, body string }{
		{http.MethodGet, "/portal/me", ""},
		{http.MethodPatch, "/portal/me", `{"name":"G"}`},
		{http.MethodPost, "/portal/me/password", `{"password":"newpass123"}`},
	} {
		w := h.call(rc.method, rc.path, guest, rc.body)
		if w.Code != 403 || decodeErr(t, w).Error.Code != "guest_session" {
			t.Fatalf("guest %s %s: %d %s", rc.method, rc.path, w.Code, w.Body.String())
		}
	}
	// A guest may still log out.
	if w := h.call(http.MethodPost, "/portal/auth/logout", guest, `{"refresh_token":"g"}`); w.Code != 204 {
		t.Fatalf("guest logout: %d %s", w.Code, w.Body.String())
	}

	// A reset session reaches only GET me and POST me/password.
	if w := h.call(http.MethodGet, "/portal/me", reset, ""); w.Code != 200 {
		t.Fatalf("reset me: %d %s", w.Code, w.Body.String())
	}
	if w := h.call(http.MethodPost, "/portal/me/password", reset, `{"password":"newpass123"}`); w.Code != 204 {
		t.Fatalf("reset password: %d %s", w.Code, w.Body.String())
	}
	if len(h.svc.password) != 1 || !h.svc.password[0].PasswordReset || h.svc.password[0].UserID != 5 {
		t.Fatalf("principal passed to SetPassword %+v", h.svc.password)
	}
	for _, rc := range []struct{ method, path, body string }{
		{http.MethodPatch, "/portal/me", `{"name":"R"}`},
		{http.MethodPost, "/portal/auth/logout", `{"refresh_token":"r"}`},
	} {
		w := h.call(rc.method, rc.path, reset, rc.body)
		if w.Code != 403 || decodeErr(t, w).Error.Code != "reset_session" {
			t.Fatalf("reset %s %s: %d %s", rc.method, rc.path, w.Code, w.Body.String())
		}
	}

	if w := h.call(http.MethodPatch, "/portal/me", user, `{"name":"Newname"}`); w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"Newname"`) {
		t.Fatalf("patch me: %d %s", w.Code, w.Body.String())
	}
	if w := h.call(http.MethodPatch, "/portal/me", user, `{}`); w.Code != 400 {
		t.Fatalf("patch me without name: %d", w.Code)
	}
	w := h.call(http.MethodPost, "/portal/me/password", user, `{"password":"newpass123"}`)
	if w.Code != 400 || decodeErr(t, w).Error.Fields["current_password"] == "" {
		t.Fatalf("password without current: %d %s", w.Code, w.Body.String())
	}
	if w := h.call(http.MethodPost, "/portal/me/password", user, `{"password":"newpass123","current_password":"secret123"}`); w.Code != 204 {
		t.Fatalf("password change: %d %s", w.Code, w.Body.String())
	}
	if w := h.call(http.MethodPost, "/portal/auth/logout", user, `{"refresh_token":"r"}`); w.Code != 204 {
		t.Fatalf("logout: %d %s", w.Code, w.Body.String())
	}
	if strings.Join(h.svc.logout, ",") != "g,r" {
		t.Fatalf("logout calls %v", h.svc.logout)
	}
	if w := h.call(http.MethodPost, "/portal/auth/logout", user, `{}`); w.Code != 400 {
		t.Fatalf("logout without token: %d", w.Code)
	}
}
