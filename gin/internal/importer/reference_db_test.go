package importer

import (
	"context"
	"strings"
	"testing"

	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/db/testutil"
)

// runReferenceSteps imports everything up to topics inside the test transaction.
func runReferenceSteps(t *testing.T, sink *Sink, src *Source, lk *Lookup, rep *Report) {
	t.Helper()
	ctx := context.Background()
	steps := []func(context.Context, *Source, *Writer, *Lookup, *Report) error{
		importPriorities, importStatuses, importDepartmentsPass1, importStaff, importDepartmentsPass2, importTopics,
	}
	for i, step := range steps {
		if err := sink.Step(ctx, func(w *Writer) error { return step(ctx, src, w, lk, rep) }); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
}

func TestReferenceSteps(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	src := openTestSource(t)
	sink := NewSink(tx, 2, false)
	lk, rep := NewLookup(), NewReport()
	runReferenceSteps(t, sink, src, lk, rep)

	// Priorities: 1-4 merge with the seed, VIP is new and keeps id 5.
	var n int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM ticket_priority").Scan(&n); err != nil || n != 5 {
		t.Fatalf("priorities = %d, %v", n, err)
	}
	if lk.Priorities[5] != 5 || lk.Priorities[2] != lk.DefaultPriority || rep.Counter(EntityPriorities).Merged != 4 {
		t.Fatalf("priority map = %+v, report %+v", lk.Priorities, rep.Counter(EntityPriorities))
	}
	// Statuses: Resolved is resolved, Archived closed, Waiting open, Deleted skipped.
	var state string
	if err := tx.QueryRow(ctx, "SELECT state FROM ticket_status WHERE id = $1", lk.Statuses[2]).Scan(&state); err != nil || state != "resolved" {
		t.Fatalf("resolved state = %q, %v", state, err)
	}
	if err := tx.QueryRow(ctx, "SELECT state FROM ticket_status WHERE id = $1", lk.Statuses[4]).Scan(&state); err != nil || state != "closed" {
		t.Fatalf("archived state = %q, %v", state, err)
	}
	if err := tx.QueryRow(ctx, "SELECT state FROM ticket_status WHERE id = $1", lk.Statuses[6]).Scan(&state); err != nil || state != "open" {
		t.Fatalf("waiting state = %q, %v", state, err)
	}
	if !lk.DeletedStatus[5] || rep.Counter(EntityStatuses).Reasons[ReasonDeletedStatus] != 1 {
		t.Fatal("deleted status not recorded")
	}
	// Departments: Support merged, Billing kept, "billing " renamed, manager set in pass 2.
	var name string
	var mgr *int64
	if err := tx.QueryRow(ctx, "SELECT name FROM department WHERE id = $1", lk.Departments[3]).Scan(&name); err != nil || name != "billing (2)" {
		t.Fatalf("renamed dept = %q, %v", name, err)
	}
	if err := tx.QueryRow(ctx, "SELECT manager_id FROM department WHERE id = $1", lk.DefaultDept).Scan(&mgr); err != nil || mgr == nil || *mgr != lk.Staff[1] {
		t.Fatalf("support manager = %v, %v", mgr, err)
	}
	if lk.Departments[1] != lk.DefaultDept || lk.Departments[2] != 2 {
		t.Fatalf("dept map = %+v", lk.Departments)
	}
	// Every manager in the fixture is imported (Support's manager is staff 1), so pass 2
	// must not have recorded a missing-manager note.
	for _, s := range rep.Counter(EntityDepartments).Samples {
		if strings.Contains(s.Reason, "not imported") {
			t.Fatalf("unexpected missing-manager note: %+v", s)
		}
	}
	// Staff: passwords kept, duplicate email replaced, ldap user reset and moved to the seed dept.
	var hash, email string
	var prim int64
	if err := tx.QueryRow(ctx, "SELECT password_hash, email FROM staff WHERE id = $1", lk.Staff[1]).Scan(&hash, &email); err != nil || !auth.CheckPassword(hash, "agentpass1") || email != "ada@example.test" {
		t.Fatalf("ada = %q %q, %v", hash, email, err)
	}
	if err := tx.QueryRow(ctx, "SELECT email FROM staff WHERE id = $1", lk.Staff[2]).Scan(&email); err != nil || email != "bob@imported.invalid" {
		t.Fatalf("bob email = %q, %v", email, err)
	}
	if err := tx.QueryRow(ctx, "SELECT password_hash, primary_dept_id FROM staff WHERE id = $1", lk.Staff[3]).Scan(&hash, &prim); err != nil || prim != lk.DefaultDept || auth.CheckPassword(hash, "") {
		t.Fatalf("ldap user = %q %d, %v", hash, prim, err)
	}
	// ada: Support (primary) + Billing; bob: Billing (primary) + Support; lea: Support. Dept 9 rows dropped.
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM staff_department").Scan(&n); err != nil || n != 5 {
		t.Fatalf("memberships = %d, %v", n, err)
	}
	// Topics: General Inquiry merged, Refunds inactive with dept Billing, Orphan dept null.
	var active bool
	var dept *int64
	if err := tx.QueryRow(ctx, "SELECT is_active, dept_id FROM help_topic WHERE id = $1", lk.Topics[2]).Scan(&active, &dept); err != nil || active || dept == nil || *dept != 2 {
		t.Fatalf("refunds = %v %v, %v", active, dept, err)
	}
	if err := tx.QueryRow(ctx, "SELECT is_active, dept_id FROM help_topic WHERE id = $1", lk.Topics[3]).Scan(&active, &dept); err != nil || !active || dept != nil {
		t.Fatalf("orphan = %v %v, %v", active, dept, err)
	}
	if rep.Counter(EntityTopics).Merged != 1 || rep.Counter(EntityTopics).Written != 2 {
		t.Fatalf("topic report = %+v", rep.Counter(EntityTopics))
	}
}

// TestPass2MissingManagerNoted checks that importDepartmentsPass2 records a note (not a skip)
// when a pending manager's source staff id was never imported.
func TestPass2MissingManagerNoted(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	sink := NewSink(tx, 2, false)
	lk := NewLookup()
	lk.PendingManagers[7] = 99
	rep := NewReport()
	if err := sink.Step(ctx, func(w *Writer) error { return importDepartmentsPass2(ctx, nil, w, lk, rep) }); err != nil {
		t.Fatalf("pass2: %v", err)
	}
	c := rep.Counter(EntityDepartments)
	if c.Skipped != 0 {
		t.Fatalf("skipped = %d, want 0", c.Skipped)
	}
	found := false
	for _, s := range c.Samples {
		if s.ID == 7 && strings.Contains(s.Reason, "99") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a sample for dept 7 mentioning staff 99, got %+v", c.Samples)
	}
}
