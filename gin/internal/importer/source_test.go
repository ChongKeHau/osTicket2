package importer

import (
	"context"
	"io"
	"testing"
	"time"
)

func TestSourceReadsReferenceTables(t *testing.T) {
	s := openTestSource(t)
	ctx := context.Background()
	pr, err := s.Priorities(ctx)
	if err != nil || len(pr) != 5 || pr[4].Name != "VIP" || pr[4].Urgency != 5 {
		t.Fatalf("priorities = %+v, %v", pr, err)
	}
	st, err := s.Statuses(ctx)
	if err != nil || len(st) != 6 || st[1].State != "closed" || st[1].Name != "Resolved" {
		t.Fatalf("statuses = %+v, %v", st, err)
	}
	de, err := s.Departments(ctx)
	if err != nil || len(de) != 3 || de[0].ManagerID != 1 || de[2].Name != "billing " || de[1].IsPublic {
		t.Fatalf("departments = %+v, %v", de, err)
	}
	sf, err := s.Staff(ctx)
	if err != nil || len(sf) != 3 || sf[2].Email != "" || sf[2].Passwd != "" || !sf[0].IsAdmin || sf[2].IsActive {
		t.Fatalf("staff = %+v, %v", sf, err)
	}
	sd, err := s.StaffDepts(ctx)
	if err != nil || len(sd) != 3 {
		t.Fatalf("staff depts = %+v, %v", sd, err)
	}
	tp, err := s.Topics(ctx)
	if err != nil || len(tp) != 3 || tp[0].Flags != 2 || tp[1].PriorityID != 3 {
		t.Fatalf("topics = %+v, %v", tp, err)
	}
}

func TestSourceReadsTicketsAndForms(t *testing.T) {
	s := openTestSource(t)
	ctx := context.Background()
	users, err := s.Users(ctx)
	if err != nil || len(users) != 3 || users[3].DefaultEmailID != 3 {
		t.Fatalf("users = %+v, %v", users, err)
	}
	emails, err := s.UserEmails(ctx)
	if err != nil || len(emails) != 2 || emails[1].Address != "pat@example.test" {
		t.Fatalf("emails = %+v, %v", emails, err)
	}
	forms, err := s.FormAnswers(ctx)
	if err != nil || forms[1]["subject"].Value != "Printer on fire" || forms[1]["priority"].ValueID == nil || *forms[1]["priority"].ValueID != 3 {
		t.Fatalf("forms = %+v, %v", forms, err)
	}
	if _, ok := forms[3]["priority"]; ok {
		t.Fatal("ticket 3 has no priority answer")
	}
	var tickets []SrcTicket
	if err := s.Tickets(ctx, func(tk SrcTicket) error { tickets = append(tickets, tk); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(tickets) != 5 || tickets[0].Number != "100001" || tickets[0].Source != "Web" || tickets[1].Closed.IsZero() || !tickets[0].Closed.IsZero() {
		t.Fatalf("tickets = %+v", tickets)
	}
	want := time.Date(2020, 2, 1, 9, 0, 0, 0, time.UTC)
	if !tickets[0].DueDate.Equal(want) {
		t.Fatalf("due = %v, want %v", tickets[0].DueDate, want)
	}
	if tickets[2].SourceExtra != "ext 12" || tickets[1].UserEmailID != 0 {
		t.Fatalf("tickets = %+v", tickets)
	}
}

func TestSourceTimezoneInterpretsDatetimes(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Kuala_Lumpur")
	s, err := OpenSource(context.Background(), mysqlDSN(t), fixturePrefix, loc)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var first SrcTicket
	_ = s.Tickets(context.Background(), func(tk SrcTicket) error {
		if first.ID == 0 {
			first = tk
		}
		return nil
	})
	// 2020-01-10 10:00 in Kuala Lumpur (UTC+8) is 02:00 UTC.
	if got := first.Created.UTC(); got != time.Date(2020, 1, 10, 2, 0, 0, 0, time.UTC) {
		t.Fatalf("created = %v", got)
	}
}

func TestSourceReadsThreadsFilesEvents(t *testing.T) {
	s := openTestSource(t)
	ctx := context.Background()
	threads, err := s.Threads(ctx)
	if err != nil || len(threads) != 5 || threads[10] != 1 || threads[50] != 5 {
		t.Fatalf("threads = %+v, %v", threads, err)
	}
	var entries []SrcEntry
	if err := s.Entries(ctx, func(e SrcEntry) error { entries = append(entries, e); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 10 || entries[0].Type != "M" || entries[1].PID != 1 || entries[4].Format != "markdown" {
		t.Fatalf("entries = %+v", entries)
	}
	var atts []SrcAttachment
	if err := s.Attachments(ctx, func(a SrcAttachment) error { atts = append(atts, a); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(atts) != 6 || atts[0].Name != "renamed-notes.txt" || atts[0].File.Backend != "D" || atts[1].File.Key != "fskey2" || !atts[1].Inline {
		t.Fatalf("attachments = %+v", atts)
	}
	rc, err := s.OpenChunks(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(b) != "hello world" {
		t.Fatalf("chunks = %q", b)
	}
	var events []SrcEvent
	if err := s.Events(ctx, func(e SrcEvent) error { events = append(events, e); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(events) != 10 || events[0].Name != "created" || events[1].Data != `{"staff":1}` || !events[7].Annulled {
		t.Fatalf("events = %+v", events)
	}
}
