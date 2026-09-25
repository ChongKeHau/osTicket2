package ticket

import (
	"context"
	"testing"

	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/db/testutil"
	"github.com/jackc/pgx/v5"
)

type fx struct {
	ctx      context.Context
	tx       pgx.Tx
	q        *db.Queries
	svc      Service
	support  db.Department
	billing  db.Department
	staff    map[string]db.Staff // "admin", "agent", "other"
	admin    auth.Principal
	agent    auth.Principal
	other    auth.Principal
	open     db.TicketStatus
	resolved db.TicketStatus
	closed   db.TicketStatus
	topic    db.HelpTopic
}

func newFixture(t *testing.T) *fx {
	t.Helper()
	ctx := context.Background()
	tx := testutil.Tx(t)
	q := db.New(tx)
	f := &fx{ctx: ctx, tx: tx, q: q, svc: NewService(tx), staff: map[string]db.Staff{}}
	var err error
	if f.support, err = q.FirstDepartment(ctx); err != nil {
		t.Fatal(err)
	}
	if f.billing, err = q.CreateDepartment(ctx, db.CreateDepartmentParams{Name: "Billing", IsPublic: true}); err != nil {
		t.Fatal(err)
	}
	mk := func(name string, admin bool, dept int64) db.Staff {
		st, err := q.CreateStaff(ctx, db.CreateStaffParams{
			Username: name, Email: name + "@x.test", PasswordHash: "h", FirstName: name, LastName: "Person",
			IsAdmin: admin, IsActive: true, PrimaryDeptID: dept,
		})
		if err != nil {
			t.Fatal(err)
		}
		f.staff[name] = st
		return st
	}
	adm := mk("admin", true, f.support.ID)
	ag := mk("agent", false, f.support.ID)
	ot := mk("other", false, f.billing.ID)
	f.admin = auth.Principal{StaffID: adm.ID, IsAdmin: true}
	f.agent = auth.Principal{StaffID: ag.ID, DeptIDs: []int64{f.support.ID}}
	f.other = auth.Principal{StaffID: ot.ID, DeptIDs: []int64{f.billing.ID}}
	statuses, err := q.ListStatuses(ctx)
	if err != nil || len(statuses) != 3 {
		t.Fatalf("statuses: %v %v", statuses, err)
	}
	f.open, f.resolved, f.closed = statuses[0], statuses[1], statuses[2]
	topics, err := q.ListTopics(ctx)
	if err != nil || len(topics) == 0 {
		t.Fatalf("topics: %v %v", topics, err)
	}
	f.topic = topics[0]
	return f
}

// create makes a ticket in dept as principal p; fails the test on error.
func (f *fx) create(t *testing.T, p auth.Principal, subject string, dept int64) *Ticket {
	t.Helper()
	tk, err := f.svc.Create(f.ctx, p, CreateInput{
		Subject: subject, Message: "<p>hello</p>", RequesterName: "Req", RequesterEmail: "req@x.test", DeptID: &dept,
	})
	if err != nil {
		t.Fatalf("create %q: %v", subject, err)
	}
	return tk
}

func (f *fx) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := f.tx.QueryRow(f.ctx, sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
