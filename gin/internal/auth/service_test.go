package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/db/testutil"
	"golang.org/x/crypto/bcrypt"
)

func init() { bcryptCost = bcrypt.MinCost }

type fixture struct {
	ctx   context.Context
	q     *db.Queries
	svc   *Service
	dept  db.Department
	dept2 db.Department
	agent db.Staff
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	tx := testutil.Tx(t)
	q := db.New(tx)
	dept, err := q.FirstDepartment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var dept2 db.Department
	if err := tx.QueryRow(ctx, `INSERT INTO department (name) VALUES ('Billing') RETURNING id, name, is_public, manager_id, created_at, updated_at`).
		Scan(&dept2.ID, &dept2.Name, &dept2.IsPublic, &dept2.ManagerID, &dept2.CreatedAt, &dept2.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	hash, _ := HashPassword("password1")
	agent, err := q.CreateStaff(ctx, db.CreateStaffParams{
		Username: "agent", Email: "agent@example.test", PasswordHash: hash,
		FirstName: "Ann", LastName: "Agent", IsAdmin: false, IsActive: true, PrimaryDeptID: dept.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.AddStaffDepartment(ctx, db.AddStaffDepartmentParams{StaffID: agent.ID, DeptID: dept2.ID}); err != nil {
		t.Fatal(err)
	}
	svc := NewService(tx, NewTokens("0123456789abcdef0123456789abcdef", 15*time.Minute), 14*24*time.Hour)
	return &fixture{ctx: ctx, q: q, svc: svc, dept: dept, dept2: dept2, agent: agent}
}

func TestLogin(t *testing.T) {
	f := newFixture(t)
	sess, err := f.svc.Login(f.ctx, "agent", "password1")
	if err != nil {
		t.Fatal(err)
	}
	if sess.AccessToken == "" || sess.RefreshToken == "" || sess.ExpiresIn != 900 {
		t.Fatalf("session: %+v", sess)
	}
	if sess.Staff.Username != "agent" || len(sess.Staff.DepartmentIDs) != 2 {
		t.Fatalf("profile: %+v", sess.Staff)
	}
	for name, cred := range map[string][2]string{
		"wrong password": {"agent", "nope"},
		"unknown user":   {"ghost", "password1"},
	} {
		if _, err := f.svc.Login(f.ctx, cred[0], cred[1]); !errors.Is(err, apperr.ErrUnauthorized) {
			t.Errorf("%s: got %v", name, err)
		}
	}
	if _, err := f.q.DB().Exec(f.ctx, `UPDATE staff SET is_active = false WHERE id = $1`, f.agent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Login(f.ctx, "agent", "password1"); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatalf("inactive: got %v", err)
	}
}

func TestRefreshRotatesAndRejectsReuse(t *testing.T) {
	f := newFixture(t)
	sess, err := f.svc.Login(f.ctx, "agent", "password1")
	if err != nil {
		t.Fatal(err)
	}
	next, err := f.svc.Refresh(f.ctx, sess.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if next.RefreshToken == sess.RefreshToken {
		t.Fatal("refresh token must rotate")
	}
	if _, err := f.svc.Refresh(f.ctx, sess.RefreshToken); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatalf("reuse of consumed token: got %v", err)
	}
	if _, err := f.svc.Refresh(f.ctx, "unknown"); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatalf("unknown token: got %v", err)
	}
	if err := f.svc.Logout(f.ctx, f.agent.ID, next.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Refresh(f.ctx, next.RefreshToken); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatalf("after logout: got %v", err)
	}
}

func TestRefreshExpired(t *testing.T) {
	f := newFixture(t)
	f.svc.now = func() time.Time { return time.Now().Add(-30 * 24 * time.Hour) }
	sess, err := f.svc.Login(f.ctx, "agent", "password1")
	if err != nil {
		t.Fatal(err)
	}
	f.svc.now = time.Now
	if _, err := f.svc.Refresh(f.ctx, sess.RefreshToken); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatalf("expired refresh: got %v", err)
	}
}

func TestLoadPrincipalAndMe(t *testing.T) {
	f := newFixture(t)
	p, err := f.svc.LoadPrincipal(f.ctx, f.agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if p.IsAdmin || !p.CanSeeDept(f.dept.ID) || !p.CanSeeDept(f.dept2.ID) || p.CanSeeDept(999999) {
		t.Fatalf("principal: %+v", p)
	}
	if _, err := f.svc.LoadPrincipal(f.ctx, 999999); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatalf("unknown staff: %v", err)
	}
	me, err := f.svc.Me(f.ctx, f.agent.ID)
	if err != nil || me.Email != "agent@example.test" || len(me.DepartmentIDs) != 2 {
		t.Fatalf("me: %+v %v", me, err)
	}
}

func TestCreateAdmin(t *testing.T) {
	f := newFixture(t)
	id, err := CreateAdmin(f.ctx, f.q.DB().(db.Beginner), "root", "root@example.test", "rootpassword", "", "")
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.svc.LoadPrincipal(f.ctx, id)
	if err != nil || !p.IsAdmin {
		t.Fatalf("admin principal: %+v %v", p, err)
	}
	st, err := f.q.GetStaff(f.ctx, id)
	if err != nil || st.FirstName != "root" || st.LastName != "" {
		t.Fatalf("empty first/last name defaults: %+v %v", st, err)
	}
	id2, err := CreateAdmin(f.ctx, f.q.DB().(db.Beginner), "root2", "root2@example.test", "rootpassword", "Root", "Two")
	if err != nil {
		t.Fatal(err)
	}
	st2, err := f.q.GetStaff(f.ctx, id2)
	if err != nil || st2.FirstName != "Root" || st2.LastName != "Two" {
		t.Fatalf("explicit first/last name: %+v %v", st2, err)
	}
	// A real unique-constraint violation aborts the shared test transaction
	// (CreateAdmin doesn't run inside db.WithTx), so this must be the last
	// statement in the test.
	if _, err := CreateAdmin(f.ctx, f.q.DB().(db.Beginner), "root", "other@example.test", "rootpassword", "", ""); !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("duplicate admin: %v", err)
	}
}

// TestLogoutOnlyRevokesOwnToken proves that Logout is scoped to the caller's
// staff id: a token that belongs to a different staff id is left alone.
func TestLogoutOnlyRevokesOwnToken(t *testing.T) {
	f := newFixture(t)
	sess, err := f.svc.Login(f.ctx, "agent", "password1")
	if err != nil {
		t.Fatal(err)
	}
	other, err := f.q.CreateStaff(f.ctx, db.CreateStaffParams{
		Username: "intruder", Email: "intruder@example.test", PasswordHash: "h",
		IsActive: true, PrimaryDeptID: f.dept.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Logout(f.ctx, other.ID, sess.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Refresh(f.ctx, sess.RefreshToken); err != nil {
		t.Fatalf("logout with the wrong staff id must not revoke another staff's token: %v", err)
	}
}

// TestRefreshMissingStaffAfterConsume proves that Refresh maps a missing
// staff row (found after the refresh token itself was successfully
// consumed) to ErrUnauthorized rather than surfacing the raw pgx.ErrNoRows.
// The orphan row is created by dropping the FK constraint for the rest of
// this test's transaction (it, and everything else, is rolled back at
// cleanup regardless): refresh_token.staff_id cascades on a real staff
// delete, which would remove the token too and never exercise this path.
// The constraint is dropped for good (not re-added) because
// ConsumeRefreshToken's UPDATE re-triggers FK validation on the row even
// though it doesn't touch staff_id.
func TestRefreshMissingStaffAfterConsume(t *testing.T) {
	f := newFixture(t)
	raw, hash, err := NewRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	conn := f.q.DB()
	if _, err := conn.Exec(f.ctx, `ALTER TABLE refresh_token DROP CONSTRAINT refresh_token_staff_id_fkey`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(f.ctx, `INSERT INTO refresh_token (token_hash, staff_id, expires_at) VALUES ($1, 999999, now() + interval '1 day')`, hash); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Refresh(f.ctx, raw); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatalf("refresh for a token whose staff row is gone: %v", err)
	}
}
