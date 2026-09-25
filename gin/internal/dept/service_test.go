package dept

import (
	"context"
	"errors"
	"testing"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/db/testutil"
)

func TestDepartmentCRUD(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	svc := NewService(tx)

	list, err := svc.List(ctx)
	if err != nil || len(list) != 1 || list[0].Name != "Support" {
		t.Fatalf("seeded list: %+v %v", list, err)
	}
	created, err := svc.Create(ctx, CreateInput{Name: "Billing"})
	if err != nil || created.Name != "Billing" || !created.IsPublic || created.ManagerID != nil {
		t.Fatalf("create: %+v %v", created, err)
	}
	if _, err := svc.Create(ctx, CreateInput{Name: "Billing"}); !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("duplicate name: %v", err)
	}
	bad := int64(999999)
	var ve *apperr.ValidationError
	if _, err := svc.Create(ctx, CreateInput{Name: "X", ManagerID: &bad}); !errors.As(err, &ve) || ve.Fields["manager_id"] == "" {
		t.Fatalf("unknown manager: %v", err)
	}
	st, err := db.New(tx).CreateStaff(ctx, db.CreateStaffParams{Username: "m", Email: "m@x.test", PasswordHash: "h", IsActive: true, PrimaryDeptID: created.ID})
	if err != nil {
		t.Fatal(err)
	}
	private := false
	name := "Billing & Accounts"
	updated, err := svc.Update(ctx, created.ID, UpdateInput{Name: &name, IsPublic: &private, ManagerID: &st.ID})
	if err != nil || updated.Name != name || updated.IsPublic || updated.ManagerID == nil || *updated.ManagerID != st.ID {
		t.Fatalf("update: %+v %v", updated, err)
	}
	if _, err := svc.Update(ctx, 999999, UpdateInput{Name: &name}); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("update missing: %v", err)
	}
	got, err := svc.Get(ctx, created.ID)
	if err != nil || got.Name != name {
		t.Fatalf("get: %+v %v", got, err)
	}
	if _, err := svc.Get(ctx, 999999); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("get missing: %v", err)
	}
	if err := svc.Delete(ctx, created.ID); !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("delete referenced: %v", err)
	}
	empty, _ := svc.Create(ctx, CreateInput{Name: "Empty"})
	if err := svc.Delete(ctx, empty.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := svc.Delete(ctx, empty.ID); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
}

// TestDeleteDepartmentForeignKeyViolation proves db.IsForeignKeyViolation
// recognizes the real FK error DeleteDepartment raises when a department is
// still referenced. This is the belt-and-braces mapping the service falls
// back on beside its own CountDepartmentReferences guard: the query is
// called directly (bypassing that guard) against the seeded "Support"
// department, which the seeded help_topic row already references.
func TestDeleteDepartmentForeignKeyViolation(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	q := db.New(tx)
	support, err := q.FirstDepartment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = db.WithTx(ctx, tx, func(q *db.Queries) error {
		_, err := q.DeleteDepartment(ctx, support.ID)
		return err
	})
	if !db.IsForeignKeyViolation(err) {
		t.Fatalf("expected a foreign key violation, got %v", err)
	}
}

func TestUpdateDepartmentClearsManager(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	svc := NewService(tx)
	d, err := svc.Create(ctx, CreateInput{Name: "Billing"})
	if err != nil {
		t.Fatal(err)
	}
	st, err := db.New(tx).CreateStaff(ctx, db.CreateStaffParams{Username: "m", Email: "m@x.test", PasswordHash: "h", IsActive: true, PrimaryDeptID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	set, err := svc.Update(ctx, d.ID, UpdateInput{ManagerID: &st.ID})
	if err != nil || set.ManagerID == nil || *set.ManagerID != st.ID {
		t.Fatalf("set manager: %+v %v", set, err)
	}
	// An absent manager leaves it unchanged.
	name := "Billing 2"
	kept, err := svc.Update(ctx, d.ID, UpdateInput{Name: &name})
	if err != nil || kept.ManagerID == nil || *kept.ManagerID != st.ID {
		t.Fatalf("absent manager changed it: %+v %v", kept, err)
	}
	cleared, err := svc.Update(ctx, d.ID, UpdateInput{ClearManager: true})
	if err != nil || cleared.ManagerID != nil {
		t.Fatalf("clear manager: %+v %v", cleared, err)
	}
	got, err := svc.Get(ctx, d.ID)
	if err != nil || got.ManagerID != nil || got.Name != name {
		t.Fatalf("row after clear: %+v %v", got, err)
	}
}
