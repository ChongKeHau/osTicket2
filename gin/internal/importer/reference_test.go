package importer

import (
	"strings"
	"testing"
	"time"

	"github.com/grandpine/ticket-api/internal/auth"
)

func TestMapStatus(t *testing.T) {
	cases := []struct {
		name, state, want string
		ok                bool
	}{
		{"Open", "open", "open", true},
		{"Waiting", "open", "open", true},
		{"Resolved", "closed", "resolved", true},
		{"resolved", "closed", "resolved", true},
		{"Closed", "closed", "closed", true},
		{"Archived", "archived", "closed", true},
		{"Deleted", "deleted", "", false},
		{"Weird", "", "open", true},
	}
	for _, c := range cases {
		got, ok := mapStatusState(c.name, c.state)
		if got != c.want || ok != c.ok {
			t.Errorf("%s/%s = %q,%v want %q,%v", c.name, c.state, got, ok, c.want, c.ok)
		}
	}
}

func TestDedupeName(t *testing.T) {
	taken := map[string]bool{"support": true}
	if got := dedupeName("Billing", taken); got != "Billing" {
		t.Fatal(got)
	}
	if got := dedupeName("billing ", taken); got != "billing (2)" {
		t.Fatal(got)
	}
	if got := dedupeName("BILLING", taken); got != "BILLING (3)" {
		t.Fatal(got)
	}
	if got := dedupeName(" Support", taken); got != "Support (2)" {
		t.Fatal(got)
	}
}

func TestStaffEmail(t *testing.T) {
	taken := map[string]bool{}
	if e, replaced := staffEmail("ada", "Ada@Example.test", taken); e != "Ada@Example.test" || replaced {
		t.Fatal(e, replaced)
	}
	if e, replaced := staffEmail("bob", "ADA@example.test", taken); e != "bob@imported.invalid" || !replaced {
		t.Fatal(e, replaced)
	}
	if e, replaced := staffEmail("lea", "", taken); e != "lea@imported.invalid" || !replaced {
		t.Fatal(e, replaced)
	}
}

func TestStaffPassword(t *testing.T) {
	const phpHash = "$2y$10$uBGMLSBZVCGO.AIPCUG6V.pCoB3BL30EoiTv11HjETcXGxkjbyl.a"
	h, reset, err := staffPassword(phpHash)
	if err != nil || reset || h != phpHash {
		t.Fatal(h, reset, err)
	}
	if !auth.CheckPassword(h, "agentpass1") {
		t.Fatal("Go bcrypt must verify a PHP $2y$ hash")
	}
	h2, reset, err := staffPassword("")
	if err != nil || !reset || !strings.HasPrefix(h2, "$2a$") {
		t.Fatal(h2, reset, err)
	}
	if auth.CheckPassword(h2, "") {
		t.Fatal("random hash must not verify the empty password")
	}
}

func TestTopicActiveAndTimes(t *testing.T) {
	if topicActive(0) || !topicActive(2) || !topicActive(3) || topicActive(1) {
		t.Fatal("flag bit 2 is active")
	}
	fb := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	if orZero(time.Time{}, fb) != fb || orZero(fb.Add(time.Hour), fb) != fb.Add(time.Hour) {
		t.Fatal("orZero")
	}
	if nullTime(time.Time{}) != nil || nullTime(fb) == nil {
		t.Fatal("nullTime")
	}
}
