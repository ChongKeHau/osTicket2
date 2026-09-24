package ticket

import (
	"encoding/json"
	"errors"
	"regexp"
	"testing"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/httpx"
)

func TestCreateWithTopicDefaults(t *testing.T) {
	f := newFixture(t)
	tk, err := f.svc.Create(f.ctx, f.agent, CreateInput{
		Subject: "Printer on fire", Message: "help", RequesterEmail: "r@x.test", TopicID: &f.topic.ID,
		Source: "phone", Extra: json.RawMessage(`{"phone":"123"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^\d{6}$`).MatchString(tk.Number) {
		t.Fatalf("number %q", tk.Number)
	}
	if tk.Department.ID != f.support.ID || tk.Priority.Name != "normal" || tk.Status.ID != f.open.ID || tk.State != "open" {
		t.Fatalf("defaults: %+v", tk)
	}
	if tk.Topic == nil || tk.Topic.ID != f.topic.ID || tk.Source != "phone" || string(tk.Extra) != `{"phone": "123"}` {
		t.Fatalf("fields: %+v extra=%s", tk, tk.Extra)
	}
	if tk.Assignee != nil || tk.IsAnswered || tk.ClosedAt != nil {
		t.Fatalf("initial state: %+v", tk)
	}
	if n := f.count(t, `SELECT count(*) FROM thread_entry WHERE ticket_id = $1 AND type = 'message'`, tk.ID); n != 1 {
		t.Fatalf("message entries: %d", n)
	}
	if n := f.count(t, `SELECT count(*) FROM ticket_event WHERE ticket_id = $1 AND kind = 'created' AND staff_id = $2`, tk.ID, f.agent.StaffID); n != 1 {
		t.Fatalf("created event: %d", n)
	}
	second := f.create(t, f.agent, "Second", f.support.ID)
	if second.Number == tk.Number {
		t.Fatal("numbers must be unique")
	}
}

func TestCreateValidationAndVisibility(t *testing.T) {
	f := newFixture(t)
	var ve *apperr.ValidationError
	_, err := f.svc.Create(f.ctx, f.agent, CreateInput{Subject: "s", Message: "m", RequesterEmail: "r@x.test"})
	if !errors.As(err, &ve) || ve.Fields["dept_id"] == "" {
		t.Fatalf("no dept: %v", err)
	}
	bad := int64(999999)
	_, err = f.svc.Create(f.ctx, f.agent, CreateInput{Subject: "s", Message: "m", RequesterEmail: "r@x.test", TopicID: &bad, PriorityID: &bad})
	if !errors.As(err, &ve) || ve.Fields["topic_id"] == "" || ve.Fields["priority_id"] == "" {
		t.Fatalf("unknown refs: %v", err)
	}
	_, err = f.svc.Create(f.ctx, f.agent, CreateInput{Subject: "s", Message: "m", RequesterEmail: "r@x.test", DeptID: &f.support.ID, Extra: json.RawMessage(`[1]`)})
	if !errors.As(err, &ve) || ve.Fields["extra"] == "" {
		t.Fatalf("extra not object: %v", err)
	}
	_, err = f.svc.Create(f.ctx, f.agent, CreateInput{Subject: "s", Message: "m", RequesterEmail: "r@x.test", DeptID: &f.billing.ID})
	if !errors.Is(err, apperr.ErrForbidden) {
		t.Fatalf("agent into invisible dept: %v", err)
	}
	if _, err := f.svc.Create(f.ctx, f.admin, CreateInput{Subject: "s", Message: "m", RequesterEmail: "r@x.test", DeptID: &f.billing.ID}); err != nil {
		t.Fatalf("admin into any dept: %v", err)
	}
	if n := f.count(t, `SELECT count(*) FROM ticket`); n != 1 {
		t.Fatalf("failed creates must not leave rows: %d", n)
	}
}

func TestGetVisibility(t *testing.T) {
	f := newFixture(t)
	tk := f.create(t, f.agent, "Mine", f.support.ID)
	if _, err := f.svc.Get(f.ctx, f.agent, tk.ID); err != nil {
		t.Fatalf("owner dept: %v", err)
	}
	if _, err := f.svc.Get(f.ctx, f.other, tk.ID); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("other dept must be 404: %v", err)
	}
	if _, err := f.svc.Get(f.ctx, f.admin, tk.ID); err != nil {
		t.Fatalf("admin: %v", err)
	}
	if _, err := f.svc.Get(f.ctx, f.admin, 999999); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestListFilters(t *testing.T) {
	f := newFixture(t)
	a := f.create(t, f.admin, "Printer jam", f.support.ID)
	f.create(t, f.admin, "Invoice wrong", f.billing.ID)
	f.create(t, f.admin, "Printer toner", f.support.ID)
	page := httpx.Page{Page: 1, PageSize: 25}

	res, err := f.svc.List(f.ctx, f.admin, ListFilter{Page: page})
	if err != nil || res.Total != 3 || len(res.Items) != 3 {
		t.Fatalf("admin all: %+v %v", res, err)
	}
	res, _ = f.svc.List(f.ctx, f.agent, ListFilter{Page: page})
	if res.Total != 2 {
		t.Fatalf("agent sees own depts only: %d", res.Total)
	}
	res, _ = f.svc.List(f.ctx, f.agent, ListFilter{Page: page, DeptID: &f.billing.ID})
	if res.Total != 0 {
		t.Fatalf("agent filtering invisible dept: %d", res.Total)
	}
	res, _ = f.svc.List(f.ctx, f.admin, ListFilter{Page: page, Q: "printer"})
	if res.Total != 2 {
		t.Fatalf("search: %d", res.Total)
	}
	res, _ = f.svc.List(f.ctx, f.admin, ListFilter{Page: page, AssignedTo: "none"})
	if res.Total != 3 {
		t.Fatalf("unassigned: %d", res.Total)
	}
	res, _ = f.svc.List(f.ctx, f.admin, ListFilter{Page: page, AssignedTo: "me"})
	if res.Total != 0 {
		t.Fatalf("assigned to me: %d", res.Total)
	}
	res, _ = f.svc.List(f.ctx, f.admin, ListFilter{Page: page, State: "closed"})
	if res.Total != 0 {
		t.Fatalf("closed: %d", res.Total)
	}
	res, _ = f.svc.List(f.ctx, f.admin, ListFilter{Page: page, StatusID: &f.open.ID, Sort: "created_at"})
	if res.Total != 3 || res.Items[0].ID != a.ID {
		t.Fatalf("status + sort asc: %+v", res)
	}
	res, _ = f.svc.List(f.ctx, f.admin, ListFilter{Page: httpx.Page{Page: 2, PageSize: 2}})
	if res.Total != 3 || len(res.Items) != 1 || res.Page != 2 {
		t.Fatalf("pagination: %+v", res)
	}
	var ve *apperr.ValidationError
	if _, err := f.svc.List(f.ctx, f.admin, ListFilter{Page: page, Sort: "nope"}); !errors.As(err, &ve) || ve.Fields["sort"] == "" {
		t.Fatalf("bad sort: %v", err)
	}
	if _, err := f.svc.List(f.ctx, f.admin, ListFilter{Page: page, State: "weird"}); !errors.As(err, &ve) || ve.Fields["state"] == "" {
		t.Fatalf("bad state: %v", err)
	}
	if _, err := f.svc.List(f.ctx, f.admin, ListFilter{Page: page, AssignedTo: "bob"}); !errors.As(err, &ve) || ve.Fields["assigned_to"] == "" {
		t.Fatalf("bad assigned_to: %v", err)
	}
}

func TestUpdate(t *testing.T) {
	f := newFixture(t)
	tk := f.create(t, f.agent, "Old", f.support.ID)
	prios, _ := f.svc.ListPriorities(f.ctx)
	high := prios[2].ID
	subject := "New subject"
	email := "new@x.test"
	out, err := f.svc.Update(f.ctx, f.agent, tk.ID, UpdateInput{Subject: &subject, PriorityID: &high, RequesterEmail: &email, Extra: json.RawMessage(`{"a":1}`)})
	if err != nil {
		t.Fatal(err)
	}
	if out.Subject != subject || out.Priority.ID != high || out.RequesterEmail != email || string(out.Extra) != `{"a": 1}` {
		t.Fatalf("updated: %+v", out)
	}
	if n := f.count(t, `SELECT count(*) FROM ticket_event WHERE ticket_id = $1 AND kind = 'edited' AND data->'fields' ? 'subject'`, tk.ID); n != 1 {
		t.Fatalf("edited event: %d", n)
	}
	bad := int64(999999)
	var ve *apperr.ValidationError
	if _, err := f.svc.Update(f.ctx, f.agent, tk.ID, UpdateInput{PriorityID: &bad}); !errors.As(err, &ve) || ve.Fields["priority_id"] == "" {
		t.Fatalf("bad priority: %v", err)
	}
	if _, err := f.svc.Update(f.ctx, f.other, tk.ID, UpdateInput{Subject: &subject}); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("invisible: %v", err)
	}
}

func TestReferenceLists(t *testing.T) {
	f := newFixture(t)
	prios, err := f.svc.ListPriorities(f.ctx)
	if err != nil || len(prios) != 4 || prios[3].Name != "emergency" {
		t.Fatalf("priorities: %+v %v", prios, err)
	}
	statuses, err := f.svc.ListStatuses(f.ctx)
	if err != nil || len(statuses) != 3 || statuses[2].State != "closed" {
		t.Fatalf("statuses: %+v %v", statuses, err)
	}
}
