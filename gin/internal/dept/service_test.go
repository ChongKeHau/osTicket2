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
