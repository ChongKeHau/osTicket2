package importer

import (
	"encoding/json"
	"testing"
)

func TestMapSource(t *testing.T) {
	for in, want := range map[string]string{"Web": "web", "Phone": "phone", "API": "api", "Email": "other", "Other": "other", "": "other", "web": "web"} {
		if got := mapSource(in); got != want {
			t.Errorf("%q = %q want %q", in, got, want)
		}
	}
}

func TestMapTicketRequesterFallback(t *testing.T) {
	users := map[int64]SrcUser{1: {ID: 1, DefaultEmailID: 1, Name: "Pat"}, 2: {ID: 2, DefaultEmailID: 2, Name: "No Email"}, 3: {ID: 3, DefaultEmailID: 3}}
	emails := map[int64]SrcUserEmail{1: {ID: 1, UserID: 1, Address: "pat@example.test"}, 3: {ID: 3, UserID: 3, Address: "fallback@example.test"}}
	if e, ph := requesterEmail(SrcTicket{ID: 1, UserID: 1, UserEmailID: 1}, users, emails); e != "pat@example.test" || ph {
		t.Fatal(e, ph)
	}
	if e, ph := requesterEmail(SrcTicket{ID: 2, UserID: 3, UserEmailID: 0}, users, emails); e != "fallback@example.test" || ph {
		t.Fatal("default email fallback", e, ph)
	}
	if e, ph := requesterEmail(SrcTicket{ID: 3, UserID: 2}, users, emails); e != "unknown-3@imported.invalid" || !ph {
		t.Fatal("placeholder", e, ph)
	}
	if e, ph := requesterEmail(SrcTicket{ID: 4, UserID: 99}, users, emails); e != "unknown-4@imported.invalid" || !ph {
		t.Fatal("missing user", e, ph)
	}
}

func TestTicketNumber(t *testing.T) {
	taken := map[string]bool{}
	if n, changed := ticketNumber(SrcTicket{ID: 1, Number: "100001"}, taken); n != "100001" || changed {
		t.Fatal(n, changed)
	}
	if n, changed := ticketNumber(SrcTicket{ID: 3, Number: ""}, taken); n != "000003" || !changed {
		t.Fatal(n, changed)
	}
	if n, changed := ticketNumber(SrcTicket{ID: 5, Number: "100001"}, taken); n != "100001-5" || !changed {
		t.Fatal(n, changed)
	}
}

func TestTicketExtra(t *testing.T) {
	b, err := ticketExtra(SrcTicket{ID: 1, Source: "Email"})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if len(m) != 1 || m["source"] != "Email" {
		t.Fatalf("extra = %v", m)
	}
	b, _ = ticketExtra(SrcTicket{ID: 1, SLAID: 2, TeamID: 3, Flags: 4, IPAddress: "10.0.0.1", SourceExtra: "ext", Source: "Web", EmailID: 6, UserID: 7})
	_ = json.Unmarshal(b, &m)
	for _, k := range []string{"sla_id", "team_id", "flags", "ip_address", "source_extra", "source", "email_id", "user_id"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("extra missing %s: %v", k, m)
		}
	}
	if b, _ = ticketExtra(SrcTicket{}); string(b) != "{}" {
		t.Fatalf("empty extra = %s", b)
	}
}
