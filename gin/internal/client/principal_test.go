package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
)

type fakeLoader struct{}

func (fakeLoader) LoadClient(_ context.Context, id int64) (Principal, error) {
	if id == 404 {
		return Principal{}, errors.Join(errors.New("gone"), apperr.ErrUnauthorized)
	}
	return Principal{UserID: id, Email: "u@x.test", Verified: true}, nil
}

func TestRequireUserAndAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ct := NewTokens(secret, time.Minute)
	r := gin.New()
	g := r.Group("/p", RequireUser(ct, fakeLoader{}))
	g.GET("/any", func(c *gin.Context) {
		p, _ := FromContext(c)
		c.JSON(200, gin.H{"uid": p.UserID, "guest": p.IsGuest(), "pwr": p.PasswordReset})
	})
	g.GET("/acct", RequireAccount(), func(c *gin.Context) { c.Status(204) })
	r.GET("/bare", RequireAccount(), func(c *gin.Context) { c.Status(204) })

	call := func(tok, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	if w := call("", "/p/any"); w.Code != 401 {
		t.Fatalf("no token: %d", w.Code)
	}
	if w := call("garbage", "/p/any"); w.Code != 401 {
		t.Fatalf("garbage token: %d", w.Code)
	}
	user, _, _ := ct.IssueAccess(5, nil, false)
	if w := call(user, "/p/any"); w.Code != 200 || w.Body.String() != `{"guest":false,"pwr":false,"uid":5}` {
		t.Fatalf("user: %d %s", w.Code, w.Body.String())
	}
	if w := call(user, "/p/acct"); w.Code != 204 {
		t.Fatalf("account: %d", w.Code)
	}
	tid := int64(9)
	guest, _, _ := ct.IssueAccess(5, &tid, false)
	if w := call(guest, "/p/any"); w.Code != 200 || !strings.Contains(w.Body.String(), `"guest":true`) {
		t.Fatalf("guest on any route: %d %s", w.Code, w.Body.String())
	}
	if w := call(guest, "/p/acct"); w.Code != 403 || !strings.Contains(w.Body.String(), "guest_session") {
		t.Fatalf("guest on account route: %d %s", w.Code, w.Body.String())
	}
	staff, _, _ := auth.NewTokens(secret, time.Minute).IssueAccess(1, false)
	if w := call(staff, "/p/any"); w.Code != 401 {
		t.Fatalf("staff token on portal: %d", w.Code)
	}
	gone, _, _ := ct.IssueAccess(404, nil, false)
	if w := call(gone, "/p/any"); w.Code != 401 {
		t.Fatalf("unknown user: %d", w.Code)
	}
	if w := call(user, "/bare"); w.Code != 401 {
		t.Fatalf("account check without principal: %d", w.Code)
	}
}

func TestRequireUserPasswordResetSessions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ct := NewTokens(secret, time.Minute)
	r := gin.New()
	show := func(c *gin.Context) {
		p, _ := FromContext(c)
		c.JSON(200, gin.H{"pwr": p.PasswordReset})
	}
	r.GET("/default", RequireUser(ct, fakeLoader{}), show)
	r.GET("/password", RequireUser(ct, fakeLoader{}, AllowPasswordReset()), show)
	call := func(tok, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	reset, _, _ := ct.IssueAccess(5, nil, true)
	if w := call(reset, "/default"); w.Code != 403 || !strings.Contains(w.Body.String(), `"reset_session"`) {
		t.Fatalf("reset session on default route: %d %s", w.Code, w.Body.String())
	}
	if w := call(reset, "/password"); w.Code != 200 || w.Body.String() != `{"pwr":true}` {
		t.Fatalf("reset session on password route: %d %s", w.Code, w.Body.String())
	}
	user, _, _ := ct.IssueAccess(5, nil, false)
	for _, path := range []string{"/default", "/password"} {
		if w := call(user, path); w.Code != 200 || w.Body.String() != `{"pwr":false}` {
			t.Fatalf("normal session on %s: %d %s", path, w.Code, w.Body.String())
		}
	}
}
