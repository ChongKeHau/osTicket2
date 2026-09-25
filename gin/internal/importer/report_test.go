package importer

import (
	"strings"
	"testing"
)

func TestReportCountsAndSamples(t *testing.T) {
	r := NewReport()
	r.Read(EntityTickets)
	r.Read(EntityTickets)
	r.Written(EntityTickets)
	r.Skip(EntityTickets, 7, ReasonDeletedStatus)
	for i := int64(0); i < 25; i++ {
		r.Skip(EntityAttachments, i, "file missing")
	}
	c := r.Counter(EntityTickets)
	if c.Read != 2 || c.Written != 1 || c.Skipped != 1 || c.Reasons[ReasonDeletedStatus] != 1 {
		t.Fatalf("ticket counter = %+v", c)
	}
	a := r.Counter(EntityAttachments)
	if a.Skipped != 25 || len(a.Samples) != 20 || a.Samples[0].ID != 0 {
		t.Fatalf("attachment counter = %+v", a)
	}
	if !r.NeedsAttention() {
		t.Fatal("attachment skips should need attention")
	}
}

func TestReportNeedsAttentionOnlyForRealSkips(t *testing.T) {
	r := NewReport()
	r.Skip(EntityTickets, 1, ReasonDeletedStatus)
	r.Skip(EntityEvents, 1, "unmapped event")
	if r.NeedsAttention() {
		t.Fatal("deleted-status tickets and event skips are expected")
	}
	r.Skip(EntityTickets, 2, "unknown department")
	if !r.NeedsAttention() {
		t.Fatal("a ticket skipped for another reason needs attention")
	}
}

func TestReportString(t *testing.T) {
	r := NewReport()
	r.Read(EntityStaff)
	r.Merged(EntityPriorities)
	r.Skip(EntityStaff, 3, "duplicate email")
	s := r.String()
	for _, want := range []string{"entity", "read", "written", "merged", "skipped", "staff", "priorities", "duplicate email", "#3"} {
		if !strings.Contains(s, want) {
			t.Fatalf("report missing %q:\n%s", want, s)
		}
	}
}

func TestLookupInitialised(t *testing.T) {
	lk := NewLookup()
	lk.Staff[1] = 1
	lk.DeletedStatus[4] = true
	if lk.Tickets == nil || lk.Files == nil || len(lk.Staff) != 1 {
		t.Fatal("maps must be initialised")
	}
}
