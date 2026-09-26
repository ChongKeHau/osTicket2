package ticket

import (
	"strings"
	"testing"
)

// ownerEmail returns the email of the end user the ticket is linked to, or ""
// when ticket.user_id is NULL.
func (f *fx) ownerEmail(t *testing.T, ticketID int64) string {
	t.Helper()
	var email *string
	if err := f.tx.QueryRow(f.ctx, `SELECT u.email FROM ticket t LEFT JOIN end_user u ON u.id = t.user_id WHERE t.id = $1`, ticketID).Scan(&email); err != nil {
		t.Fatal(err)
	}
	if email == nil {
		return ""
	}
	return *email
}

func TestStaffCreateLinksRequesterEndUser(t *testing.T) {
	f := newFixture(t)
	tk := f.create(t, f.agent, "Staff opened", f.support.ID)
	if got := f.ownerEmail(t, tk.ID); got != "req@x.test" {
		t.Fatalf("owner = %q", got)
	}
	// A second ticket for the same address, in another casing, reuses the end user.
	dept := f.support.ID
	tk2, err := f.svc.Create(f.ctx, f.agent, CreateInput{
		Subject: "Again", Message: "m", RequesterName: "Req", RequesterEmail: "REQ@X.test", DeptID: &dept,
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := f.count(t, `SELECT count(DISTINCT user_id) FROM ticket WHERE id = ANY($1)`, []int64{tk.ID, tk2.ID}); n != 1 {
		t.Fatalf("distinct owners = %d", n)
	}
	if n := f.count(t, `SELECT count(*) FROM end_user WHERE lower(email) = 'req@x.test'`); n != 1 {
		t.Fatalf("end users = %d", n)
	}
}

func TestCreateExternalLinksRequesterEndUser(t *testing.T) {
	f := newFixture(t)
	tk, err := f.svc.CreateExternal(f.ctx, ExternalCreateInput{
		Subject: "By mail", Body: "b", Format: "text", RequesterName: "Mo", RequesterEmail: "mo@x.test", DeptID: f.support.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.ownerEmail(t, tk.ID); got != "mo@x.test" {
		t.Fatalf("owner = %q", got)
	}
}

func TestUpdateRequesterEmailMovesEndUser(t *testing.T) {
	f := newFixture(t)
	tk := f.create(t, f.agent, "Moving", f.support.ID)
	email := "New.Person@x.test"
	if _, err := f.svc.Update(f.ctx, f.agent, tk.ID, UpdateInput{RequesterEmail: &email}); err != nil {
		t.Fatal(err)
	}
	if got := f.ownerEmail(t, tk.ID); !strings.EqualFold(got, email) {
		t.Fatalf("owner after change = %q", got)
	}
	// An update that does not touch the requester email leaves the link alone.
	subject := "Still moving"
	if _, err := f.svc.Update(f.ctx, f.agent, tk.ID, UpdateInput{Subject: &subject}); err != nil {
		t.Fatal(err)
	}
	if got := f.ownerEmail(t, tk.ID); !strings.EqualFold(got, email) {
		t.Fatalf("owner after subject edit = %q", got)
	}
}
