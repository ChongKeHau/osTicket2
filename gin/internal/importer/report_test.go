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

func TestReportNote(t *testing.T) {
	r := NewReport()
	r.Note(EntityDepartments, 3, "renamed to Billing (2)")
	c := r.Counter(EntityDepartments)
	if c.Skipped != 0 || len(c.Samples) != 1 || !strings.Contains(r.String(), "renamed to Billing (2)") {
		t.Fatalf("note not recorded: %+v\n%s", c, r.String())
	}
	if r.NeedsAttention() {
		t.Fatal("notes never need attention")
	}
}

func TestLookupAllocID(t *testing.T) {
	lk := NewLookup()
	lk.markTaken("ticket_priority", 1)
	lk.markTaken("ticket_priority", 2)
	if got := lk.allocID("ticket_priority", 5); got != 5 {
		t.Fatalf("free id kept: %d", got)
	}
	if got := lk.allocID("ticket_priority", 2); got != 6 {
		t.Fatalf("collision moves above max: %d", got)
	}
	if got := lk.allocID("ticket_priority", 6); got != 7 {
		t.Fatalf("second collision: %d", got)
	}
}

func TestAllocIDNotedReportsRemaps(t *testing.T) {
	lk, rep := NewLookup(), NewReport()
	lk.markTaken("department", 1)
	if got := allocIDNoted(lk, rep, EntityDepartments, "department", 4); got != 4 {
		t.Fatalf("free id kept: %d", got)
	}
	if got := allocIDNoted(lk, rep, EntityDepartments, "department", 1); got != 5 {
		t.Fatalf("collision: %d", got)
	}
	s := rep.Counter(EntityDepartments).Samples
	if len(s) != 1 || s[0].ID != 1 || s[0].Reason != "id remapped to 5" {
		t.Fatalf("samples = %+v, want one remap note for source id 1", s)
	}
}
