package topic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
)

type fake struct{ topics []Topic }

func (f *fake) List(context.Context) ([]Topic, error)           { return f.topics, nil }
func (f *fake) Get(_ context.Context, id int64) (*Topic, error) { return nil, apperr.ErrNotFound }
func (f *fake) Create(_ context.Context, in CreateInput) (*Topic, error) {
	return &Topic{ID: 9, Name: in.Name}, nil
}
func (f *fake) Update(_ context.Context, id int64, in UpdateInput) (*Topic, error) {
	return &Topic{ID: id, Name: "u"}, nil
}
func (f *fake) Delete(_ context.Context, id int64) error { return nil }

func router(admin bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	private := r.Group("/api/v1", func(c *gin.Context) {
		auth.WithPrincipal(c, auth.Principal{StaffID: 1, IsAdmin: admin})
	})
	NewHandler(&fake{topics: []Topic{{ID: 1, Name: "General"}}}).Mount(private)
	return r
}

func call(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestTopicRoutes(t *testing.T) {
	admin, agent := router(true), router(false)
	if w := call(agent, http.MethodGet, "/api/v1/topics", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "General") {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	if w := call(agent, http.MethodGet, "/api/v1/topics/3", ""); w.Code != 404 {
		t.Fatalf("get: %d", w.Code)
	}
	if w := call(agent, http.MethodPost, "/api/v1/topics", `{"name":"X"}`); w.Code != 403 {
		t.Fatalf("agent create: %d", w.Code)
	}
	if w := call(admin, http.MethodPost, "/api/v1/topics", `{"name":""}`); w.Code != 400 {
		t.Fatalf("empty name: %d", w.Code)
	}
	if w := call(admin, http.MethodPost, "/api/v1/topics", `{"name":"X"}`); w.Code != 201 {
		t.Fatalf("create: %d", w.Code)
	}
	if w := call(admin, http.MethodPatch, "/api/v1/topics/1", `{"sort_order":2}`); w.Code != 200 {
		t.Fatalf("update: %d", w.Code)
	}
	if w := call(admin, http.MethodDelete, "/api/v1/topics/1", ""); w.Code != 204 {
		t.Fatalf("delete: %d", w.Code)
	}
}
