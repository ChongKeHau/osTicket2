package dept

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

type fake struct {
	list   func() ([]Department, error)
	get    func(id int64) (*Department, error)
	create func(in CreateInput) (*Department, error)
	update func(id int64, in UpdateInput) (*Department, error)
	del    func(id int64) error
}

func (f *fake) List(context.Context) ([]Department, error)                    { return f.list() }
func (f *fake) Get(_ context.Context, id int64) (*Department, error)          { return f.get(id) }
func (f *fake) Create(_ context.Context, in CreateInput) (*Department, error) { return f.create(in) }
func (f *fake) Update(_ context.Context, id int64, in UpdateInput) (*Department, error) {
	return f.update(id, in)
}
func (f *fake) Delete(_ context.Context, id int64) error { return f.del(id) }

func router(f *fake, admin bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	private := r.Group("/api/v1", func(c *gin.Context) {
		auth.WithPrincipal(c, auth.Principal{StaffID: 1, IsAdmin: admin})
	})
	NewHandler(f).Mount(private)
	return r
}

func call(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestDepartmentRoutes(t *testing.T) {
	f := &fake{
		list:   func() ([]Department, error) { return []Department{{ID: 1, Name: "Support"}}, nil },
		get:    func(id int64) (*Department, error) { return nil, apperr.ErrNotFound },
		create: func(in CreateInput) (*Department, error) { return &Department{ID: 2, Name: in.Name}, nil },
		update: func(id int64, in UpdateInput) (*Department, error) { return &Department{ID: id, Name: *in.Name}, nil },
		del:    func(id int64) error { return apperr.ErrConflict },
	}
	admin := router(f, true)
	if w := call(admin, http.MethodGet, "/api/v1/departments", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "Support") {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	if w := call(admin, http.MethodGet, "/api/v1/departments/7", ""); w.Code != 404 {
		t.Fatalf("get missing: %d", w.Code)
	}
	if w := call(admin, http.MethodGet, "/api/v1/departments/x", ""); w.Code != 400 {
		t.Fatalf("bad id: %d", w.Code)
	}
	if w := call(admin, http.MethodPost, "/api/v1/departments", `{}`); w.Code != 400 {
		t.Fatalf("create missing name: %d", w.Code)
	}
	if w := call(admin, http.MethodPost, "/api/v1/departments", `{"name":"Billing"}`); w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if w := call(admin, http.MethodPatch, "/api/v1/departments/2", `{"name":"B2"}`); w.Code != 200 {
		t.Fatalf("update: %d", w.Code)
	}
	if w := call(admin, http.MethodDelete, "/api/v1/departments/2", ""); w.Code != 409 {
		t.Fatalf("delete conflict: %d", w.Code)
	}
	agent := router(f, false)
	if w := call(agent, http.MethodGet, "/api/v1/departments", ""); w.Code != 200 {
		t.Fatalf("agent list: %d", w.Code)
	}
	if w := call(agent, http.MethodPost, "/api/v1/departments", `{"name":"Nope"}`); w.Code != 403 {
		t.Fatalf("agent create: %d", w.Code)
	}
	if w := call(agent, http.MethodDelete, "/api/v1/departments/2", ""); w.Code != 403 {
		t.Fatalf("agent delete: %d", w.Code)
	}
}
