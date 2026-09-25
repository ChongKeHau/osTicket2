package staff

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/db/testutil"
)

func TestStaffLifecycle(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	q := db.New(tx)
	svc := NewService(tx)
	dept, _ := q.FirstDepartment(ctx)
	var billingID int64
	if err := tx.QueryRow(ctx, `INSERT INTO department (name) VALUES ('Billing') RETURNING id`).Scan(&billingID); err != nil {
		t.Fatal(err)
	}

	var ve *apperr.ValidationError
	if _, err := svc.Create(ctx, CreateInput{Username: "a", Email: "a@x.test", Password: "short", PrimaryDeptID: dept.ID}); !errors.As(err, &ve) || ve.Fields["password"] == "" {
		t.Fatalf("short password: %v", err)
	}
	if _, err := svc.Create(ctx, CreateInput{Username: "a", Email: "a@x.test", Password: "password1", PrimaryDeptID: 999999}); !errors.As(err, &ve) || ve.Fields["primary_dept_id"] == "" {
		t.Fatalf("bad dept: %v", err)
	}
	created, err := svc.Create(ctx, CreateInput{
		Username: "ann", Email: "ann@x.test", Password: "password1", FirstName: "Ann",
		PrimaryDeptID: dept.ID, DepartmentIDs: []int64{billingID, dept.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !created.IsActive || created.IsAdmin || len(created.DepartmentIDs) != 2 || created.DepartmentIDs[0] != dept.ID {
		t.Fatalf("created: %+v", created)
	}
	if _, err := svc.Create(ctx, CreateInput{Username: "ann", Email: "other@x.test", Password: "password1", PrimaryDeptID: dept.ID}); !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("duplicate username: %v", err)
	}

	list, err := svc.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %+v %v", list, err)
	}

	// login to get a refresh token, then deactivate: tokens must be revoked
	authSvc := auth.NewService(tx, auth.NewTokens("0123456789abcdef0123456789abcdef", time.Minute), time.Hour)
	sess, err := authSvc.Login(ctx, "ann", "password1")
	if err != nil {
		t.Fatal(err)
	}
	inactive := false
	only := []int64{billingID}
	updated, err := svc.Update(ctx, created.ID, UpdateInput{IsActive: &inactive, DepartmentIDs: &only})
	if err != nil || updated.IsActive || len(updated.DepartmentIDs) != 2 {
		t.Fatalf("update: %+v %v", updated, err)
	}
	if _, err := authSvc.Refresh(ctx, sess.RefreshToken); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatalf("refresh after deactivation must fail: %v", err)
	}
	if _, err := svc.Update(ctx, 999999, UpdateInput{IsActive: &inactive}); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("update missing: %v", err)
	}

	if err := svc.SetPassword(ctx, created.ID, "newpassword1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetPassword(ctx, 999999, "newpassword1"); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("set password missing: %v", err)
	}
	active := true
	if _, err := svc.Update(ctx, created.ID, UpdateInput{IsActive: &active}); err != nil {
		t.Fatal(err)
	}
	sess2, err := authSvc.Login(ctx, "ann", "newpassword1")
	if err != nil {
		t.Fatalf("login with new password: %v", err)
	}
	got, err := svc.Get(ctx, created.ID)
	if err != nil || got.Username != "ann" {
		t.Fatalf("get: %+v %v", got, err)
	}

	// SetPassword must revoke existing refresh tokens, not just change the
	// hash: a stolen refresh token from before the reset must stop working.
	if err := svc.SetPassword(ctx, created.ID, "resetpassword1"); err != nil {
		t.Fatal(err)
	}
	if _, err := authSvc.Refresh(ctx, sess2.RefreshToken); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatalf("refresh after password reset must fail: %v", err)
	}
	if _, err := authSvc.Login(ctx, "ann", "resetpassword1"); err != nil {
		t.Fatalf("login with reset password: %v", err)
	}
}

// TestLastActiveAdminCannotBeRemoved proves that deactivating or demoting
// the sole remaining active admin is refused, but the same change succeeds
// once a second active admin exists.
func TestLastActiveAdminCannotBeRemoved(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	q := db.New(tx)
	svc := NewService(tx)
	dept, _ := q.FirstDepartment(ctx)

	admin, err := svc.Create(ctx, CreateInput{
		Username: "root", Email: "root@x.test", Password: "password1", IsAdmin: true, PrimaryDeptID: dept.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	inactive := false
	if _, err := svc.Update(ctx, admin.ID, UpdateInput{IsActive: &inactive}); !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("deactivating the last active admin: %v", err)
	}
	notAdmin := false
	if _, err := svc.Update(ctx, admin.ID, UpdateInput{IsAdmin: &notAdmin}); !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("demoting the last active admin: %v", err)
	}
	got, err := svc.Get(ctx, admin.ID)
	if err != nil || !got.IsAdmin || !got.IsActive {
		t.Fatalf("refused changes must not partially apply: %+v %v", got, err)
	}

	if _, err := svc.Create(ctx, CreateInput{
		Username: "root2", Email: "root2@x.test", Password: "password1", IsAdmin: true, PrimaryDeptID: dept.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, admin.ID, UpdateInput{IsAdmin: &notAdmin}); err != nil {
		t.Fatalf("demoting with another active admin present: %v", err)
	}
}
