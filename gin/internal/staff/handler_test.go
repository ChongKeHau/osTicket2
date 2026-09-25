package staff

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

type fake struct{}

func (fake) List(context.Context) ([]Staff, error) { return []Staff{{ID: 1, Username: "ann"}}, nil }
func (fake) Get(_ context.Context, id int64) (*Staff, error) {
	if id == 1 {
		return &Staff{ID: 1, Username: "ann"}, nil
	}
	return nil, apperr.ErrNotFound
}
func (fake) Create(_ context.Context, in CreateInput) (*Staff, error) {
	return &Staff{ID: 2, Username: in.Username}, nil
}
func (fake) Update(_ context.Context, id int64, in UpdateInput) (*Staff, error) {
	return &Staff{ID: id}, nil
}
func (fake) SetPassword(_ context.Context, id int64, pw string) error {
	if len(pw) < 8 {
		return apperr.Validation("password", "too short")
	}
	return nil
}

func router(admin bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	private := r.Group("/api/v1", func(c *gin.Context) {
		auth.WithPrincipal(c, auth.Principal{StaffID: 1, IsAdmin: admin})
	})
	NewHandler(fake{}).Mount(private)
	return r
}

func call(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestStaffRoutes(t *testing.T) {
	admin, agent := router(true), router(false)
	if w := call(agent, http.MethodGet, "/api/v1/staff", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "ann") {
		t.Fatalf("agent list: %d %s", w.Code, w.Body.String())
	}
	if w := call(agent, http.MethodGet, "/api/v1/staff/1", ""); w.Code != 200 {
		t.Fatalf("agent get: %d", w.Code)
	}
	if w := call(agent, http.MethodPost, "/api/v1/staff", `{}`); w.Code != 403 {
		t.Fatalf("agent create: %d", w.Code)
	}
	if w := call(admin, http.MethodPost, "/api/v1/staff", `{"username":"bob","email":"not-an-email","password":"password1","primary_dept_id":1}`); w.Code != 400 {
		t.Fatalf("bad email: %d %s", w.Code, w.Body.String())
	}
	if w := call(admin, http.MethodPost, "/api/v1/staff", `{"username":"bob","email":"bob@x.test","password":"password1","primary_dept_id":1}`); w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if w := call(admin, http.MethodPatch, "/api/v1/staff/2", `{"is_active":false}`); w.Code != 200 {
		t.Fatalf("update: %d", w.Code)
	}
	if w := call(admin, http.MethodPost, "/api/v1/staff/2/password", `{"password":"short"}`); w.Code != 400 {
		t.Fatalf("short password: %d", w.Code)
	}
	if w := call(admin, http.MethodPost, "/api/v1/staff/2/password", `{"password":"longenough"}`); w.Code != 204 {
		t.Fatalf("set password: %d", w.Code)
	}
	if w := call(admin, http.MethodGet, "/api/v1/staff/9", ""); w.Code != 404 {
		t.Fatalf("get missing: %d", w.Code)
	}
}
