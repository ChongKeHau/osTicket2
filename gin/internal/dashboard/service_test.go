package dashboard

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/db/testutil"
)

type fx struct {
	ctx              context.Context
	q                *db.Queries
	svc              *Service
	deptA, deptB     int64
	staffA           int64
	topic            int64
	ticketA, ticketB int64
}

func setup(t *testing.T) *fx {
	t.Helper()
	tx := testutil.Tx(t)
	ctx := context.Background()
	q := db.New(tx)
	a, err := q.CreateDepartment(ctx, db.CreateDepartmentParams{Name: "A", IsPublic: true})
	if err != nil {
		t.Fatal(err)
	}
	b, err := q.CreateDepartment(ctx, db.CreateDepartmentParams{Name: "B", IsPublic: true})
	if err != nil {
		t.Fatal(err)
	}
	st, err := q.CreateStaff(ctx, db.CreateStaffParams{Username: "amy", Email: "amy@x.test", PasswordHash: "h", FirstName: "Amy", LastName: "A", IsActive: true, PrimaryDeptID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	tp, err := q.CreateTopic(ctx, db.CreateTopicParams{Name: "Billing", IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	f := &fx{ctx: ctx, q: q, svc: NewService(tx), deptA: a.ID, deptB: b.ID, staffA: st.ID, topic: tp.ID}
	f.ticketA = f.ticket(t, a.ID, &tp.ID)
	f.ticketB = f.ticket(t, b.ID, nil)
	return f
}

// ticket inserts a minimal ticket row; adapted to CreateTicket returning the
// new ticket's id directly (int64, error), not a row struct.
func (f *fx) ticket(t *testing.T, deptID int64, topicID *int64) int64 {
	t.Helper()
	pr, err := f.q.ListPriorities(f.ctx)
	if err != nil || len(pr) == 0 {
		t.Fatalf("priorities: %v", err)
	}
	sts, err := f.q.ListStatuses(f.ctx)
	if err != nil || len(sts) == 0 {
		t.Fatalf("statuses: %v", err)
	}
	id, err := f.q.CreateTicket(f.ctx, db.CreateTicketParams{
		Number:  "T" + time.Now().Format("150405.000000") + string(rune('A'+deptID%26)),
		Subject: "s", DeptID: deptID, TopicID: topicID, PriorityID: pr[0].ID, StatusID: sts[0].ID,
		RequesterName: "r", RequesterEmail: "r@x.test", Source: db.TicketSourceWeb, Extra: []byte(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *fx) event(t *testing.T, ticketID int64, staffID *int64, kind db.TicketEventKind, at time.Time) {
	t.Helper()
	if err := f.q.CreateTicketEvent(f.ctx, db.CreateTicketEventParams{TicketID: ticketID, StaffID: staffID, Kind: kind, Data: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}
	// CreateTicketEvent uses now(); move the row to the wanted time.
	if _, err := f.q.DB().Exec(f.ctx, `UPDATE ticket_event SET created_at = $1 WHERE id = (SELECT max(id) FROM ticket_event WHERE ticket_id = $2)`, at, ticketID); err != nil {
		t.Fatal(err)
	}
}

var start = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

func TestStatsSeriesZeroFillAndWindowEdges(t *testing.T) {
	f := setup(t)
	f.event(t, f.ticketA, &f.staffA, db.TicketEventKindCreated, start)                                // first instant: in
	f.event(t, f.ticketA, &f.staffA, db.TicketEventKindAssigned, start.Add(24*time.Hour+time.Minute)) // day 2
	f.event(t, f.ticketA, &f.staffA, db.TicketEventKindClosed, start.Add(7*24*time.Hour))             // == end: out
	f.event(t, f.ticketA, &f.staffA, db.TicketEventKindReopened, start.Add(-time.Second))             // before: out

	st, err := f.svc.Stats(f.ctx, auth.Principal{IsAdmin: true}, start, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Series) != 7 {
		t.Fatalf("series len %d", len(st.Series))
	}
	if st.Series[0].Date != "2026-09-01" || st.Series[0].Opened != 1 {
		t.Fatalf("day0 %+v", st.Series[0])
	}
	if st.Series[1].Assigned != 1 {
		t.Fatalf("day1 %+v", st.Series[1])
	}
	for i := 2; i < 7; i++ {
		p := st.Series[i]
		if p.Opened+p.Assigned+p.Closed+p.Reopened != 0 {
			t.Fatalf("day%d not zero: %+v", i, p)
		}
	}
	if st.Start != "2026-09-01" || st.Period != 7 {
		t.Fatalf("meta %+v", st)
	}
}

func TestStatsScopesToVisibleDepartments(t *testing.T) {
	f := setup(t)
	f.event(t, f.ticketA, &f.staffA, db.TicketEventKindCreated, start)
	// staffA acted on a ticket in dept B, which staffA cannot see.
	f.event(t, f.ticketB, &f.staffA, db.TicketEventKindAssigned, start)

	st, err := f.svc.Stats(f.ctx, auth.Principal{StaffID: f.staffA, DeptIDs: []int64{f.deptA}}, start, 7)
	if err != nil {
		t.Fatal(err)
	}
	if st.Series[0].Opened != 1 || st.Series[0].Assigned != 0 {
		t.Fatalf("leaked: %+v", st.Series[0])
	}
	if len(st.ByDepartment) != 1 || st.ByDepartment[0].Name != "A" {
		t.Fatalf("by dept %+v", st.ByDepartment)
	}
	if len(st.ByStaff) != 1 || st.ByStaff[0].Assigned != 0 {
		t.Fatalf("by staff %+v", st.ByStaff)
	}

	all, err := f.svc.Stats(f.ctx, auth.Principal{IsAdmin: true}, start, 7)
	if err != nil {
		t.Fatal(err)
	}
	if all.Series[0].Assigned != 1 || len(all.ByDepartment) != 2 {
		t.Fatalf("admin %+v", all)
	}
}

func TestStatsNullAttribution(t *testing.T) {
	f := setup(t)
	f.event(t, f.ticketB, nil, db.TicketEventKindCreated, start) // no topic, no staff (email/API)
	st, err := f.svc.Stats(f.ctx, auth.Principal{IsAdmin: true}, start, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.ByTopic) != 1 || st.ByTopic[0].ID != nil || st.ByTopic[0].Name != "— none —" || st.ByTopic[0].Opened != 1 {
		t.Fatalf("by topic %+v", st.ByTopic)
	}
	if len(st.ByStaff) != 1 || st.ByStaff[0].ID != nil || st.ByStaff[0].Name != "— system —" {
		t.Fatalf("by staff %+v", st.ByStaff)
	}
}

func TestStatsEmptyWindow(t *testing.T) {
	f := setup(t)
	st, err := f.svc.Stats(f.ctx, auth.Principal{IsAdmin: true}, start.AddDate(1, 0, 0), 14)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Series) != 14 || len(st.ByDepartment) != 0 || len(st.ByTopic) != 0 || len(st.ByStaff) != 0 {
		t.Fatalf("empty window %+v", st)
	}
	if st.ByDepartment == nil || st.ByTopic == nil || st.ByStaff == nil {
		t.Fatal("breakdowns must be empty slices, not nil, so JSON renders []")
	}
}

func TestStatsRejectsBadPeriod(t *testing.T) {
	f := setup(t)
	if _, err := f.svc.Stats(f.ctx, auth.Principal{IsAdmin: true}, start, 10); err == nil {
		t.Fatal("expected validation error")
	}
}
