package ticket

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/httpx"
)

type fakeSvc struct {
	lastFilter ListFilter
	lastCreate CreateInput
}

func (f *fakeSvc) Create(_ context.Context, p auth.Principal, in CreateInput) (*Ticket, error) {
	f.lastCreate = in
	return &Ticket{ID: 1, Subject: in.Subject}, nil
}
func (f *fakeSvc) Get(_ context.Context, p auth.Principal, id int64) (*Ticket, error) {
	if id == 1 {
		return &Ticket{ID: 1, Subject: "one"}, nil
	}
	return nil, apperr.ErrNotFound
}
func (f *fakeSvc) List(_ context.Context, p auth.Principal, fl ListFilter) (*httpx.List[Ticket], error) {
	f.lastFilter = fl
	return &httpx.List[Ticket]{Items: []Ticket{}, Page: fl.Page.Page, PageSize: fl.Page.PageSize}, nil
}
func (f *fakeSvc) Update(_ context.Context, p auth.Principal, id int64, in UpdateInput) (*Ticket, error) {
	return &Ticket{ID: id}, nil
}
func (f *fakeSvc) ListPriorities(context.Context) ([]Priority, error) {
	return []Priority{{ID: 1, Name: "low"}}, nil
}
func (f *fakeSvc) ListStatuses(context.Context) ([]Status, error) {
	return []Status{{ID: 1, Name: "Open", State: "open"}}, nil
}

func newRouter(f *fakeSvc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	private := r.Group("/api/v1", func(c *gin.Context) {
		auth.WithPrincipal(c, auth.Principal{StaffID: 7, DeptIDs: []int64{1}})
	})
	NewHandler(f).Mount(private)
	return r
}

func do(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestTicketCoreRoutes(t *testing.T) {
	f := &fakeSvc{}
	r := newRouter(f)
	if w := do(r, http.MethodGet, "/api/v1/priorities", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "low") {
		t.Fatalf("priorities: %d %s", w.Code, w.Body.String())
	}
	if w := do(r, http.MethodGet, "/api/v1/statuses", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "Open") {
		t.Fatalf("statuses: %d", w.Code)
	}
	w := do(r, http.MethodGet, "/api/v1/tickets?state=open&assigned_to=me&q=printer&sort=-priority&status=3&dept_id=1&page=2&page_size=10", "")
	if w.Code != 200 {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	fl := f.lastFilter
	if fl.State != "open" || fl.AssignedTo != "me" || fl.Q != "printer" || fl.Sort != "-priority" ||
		fl.StatusID == nil || *fl.StatusID != 3 || fl.DeptID == nil || *fl.DeptID != 1 || fl.Page.Page != 2 || fl.Page.PageSize != 10 {
		t.Fatalf("filter parsing: %+v", fl)
	}
	if w := do(r, http.MethodGet, "/api/v1/tickets?status=abc", ""); w.Code != 400 {
		t.Fatalf("bad status param: %d", w.Code)
	}
	if w := do(r, http.MethodGet, "/api/v1/tickets?page_size=0", ""); w.Code != 400 {
		t.Fatalf("bad page_size: %d", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/tickets", `{"subject":"s","message":"m","requester_email":"bad"}`); w.Code != 400 {
		t.Fatalf("bad email: %d", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/tickets", `{"subject":"`+strings.Repeat("x", 300)+`","message":"m","requester_email":"a@b.test"}`); w.Code != 400 {
		t.Fatalf("overlong subject: %d", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/tickets", `{"subject":"s","message":"m","requester_email":"a@b.test","dept_id":1,"source":"phone"}`); w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if f.lastCreate.Source != "phone" || f.lastCreate.DeptID == nil {
		t.Fatalf("create binding: %+v", f.lastCreate)
	}
	if w := do(r, http.MethodGet, "/api/v1/tickets/1", ""); w.Code != 200 {
		t.Fatalf("get: %d", w.Code)
	}
	if w := do(r, http.MethodGet, "/api/v1/tickets/2", ""); w.Code != 404 {
		t.Fatalf("get missing: %d", w.Code)
	}
	if w := do(r, http.MethodPatch, "/api/v1/tickets/1", `{"subject":""}`); w.Code != 400 {
		t.Fatalf("empty subject: %d", w.Code)
	}
	if w := do(r, http.MethodPatch, "/api/v1/tickets/1", `{"subject":"new"}`); w.Code != 200 {
		t.Fatalf("update: %d", w.Code)
	}
}
