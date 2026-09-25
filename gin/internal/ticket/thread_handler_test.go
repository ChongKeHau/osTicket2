package ticket

import (
	"net/http"
	"strings"
	"testing"
)

func TestThreadRoutes(t *testing.T) {
	f := &fakeSvc{}
	r := newRouter(f)
	if w := do(r, http.MethodPost, "/api/v1/tickets/1/reply", `{}`); w.Code != 400 {
		t.Fatalf("reply without body: %d", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/tickets/1/reply", `{"body":"hi","format":"markdown"}`); w.Code != 400 {
		t.Fatalf("reply bad format: %d", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/tickets/1/reply", `{"body":"hi","file_ids":[1,2]}`); w.Code != 201 || !strings.Contains(w.Body.String(), `"response"`) {
		t.Fatalf("reply: %d %s", w.Code, w.Body.String())
	}
	if w := do(r, http.MethodPost, "/api/v1/tickets/1/notes", `{"body":"n"}`); w.Code != 201 {
		t.Fatalf("note: %d", w.Code)
	}
	if w := do(r, http.MethodGet, "/api/v1/tickets/1/thread?after=5&limit=20", ""); w.Code != 200 || f.lastAfter != 5 || f.lastLimit != 20 {
		t.Fatalf("thread params: %d after=%d limit=%d", w.Code, f.lastAfter, f.lastLimit)
	}
	if w := do(r, http.MethodGet, "/api/v1/tickets/1/thread", ""); w.Code != 200 || f.lastLimit != 50 {
		t.Fatalf("thread defaults: %d limit=%d", w.Code, f.lastLimit)
	}
	if w := do(r, http.MethodGet, "/api/v1/tickets/1/thread?limit=500", ""); w.Code != 400 {
		t.Fatalf("thread limit too big: %d", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/tickets/1/status", `{}`); w.Code != 400 {
		t.Fatalf("status missing: %d", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/tickets/1/status", `{"status_id":99}`); w.Code != 400 {
		t.Fatalf("status unknown: %d", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/tickets/1/status", `{"status_id":2}`); w.Code != 200 {
		t.Fatalf("status: %d", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/tickets/1/assign", `{}`); w.Code != 400 {
		t.Fatalf("assign missing staff_id key: %d", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/tickets/1/assign", `{"staff_id":null}`); w.Code != 200 || f.lastAssign != nil {
		t.Fatalf("unassign: %d %v", w.Code, f.lastAssign)
	}
	if w := do(r, http.MethodPost, "/api/v1/tickets/1/assign", `{"staff_id":4}`); w.Code != 200 || f.lastAssign == nil || *f.lastAssign != 4 {
		t.Fatalf("assign: %d %v", w.Code, f.lastAssign)
	}
	if w := do(r, http.MethodPost, "/api/v1/tickets/1/transfer", `{}`); w.Code != 400 {
		t.Fatalf("transfer missing dept: %d", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/tickets/1/transfer", `{"dept_id":2}`); w.Code != 200 {
		t.Fatalf("transfer: %d", w.Code)
	}
	if w := do(r, http.MethodGet, "/api/v1/tickets/1/events", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "created") {
		t.Fatalf("events: %d %s", w.Code, w.Body.String())
	}
}
