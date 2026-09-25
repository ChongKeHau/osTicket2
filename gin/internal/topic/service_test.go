package topic

import (
	"context"
	"errors"
	"testing"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/db/testutil"
)

func TestTopicCRUD(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	svc := NewService(tx)
	q := db.New(tx)

	list, err := svc.List(ctx)
	if err != nil || len(list) != 1 || list[0].Name != "General Inquiry" || list[0].DeptID == nil {
		t.Fatalf("seeded: %+v %v", list, err)
	}
	dept, _ := q.FirstDepartment(ctx)
	prio, _ := q.DefaultPriority(ctx)
	bad := int64(999999)
	var ve *apperr.ValidationError
	if _, err := svc.Create(ctx, CreateInput{Name: "X", DeptID: &bad}); !errors.As(err, &ve) || ve.Fields["dept_id"] == "" {
		t.Fatalf("unknown dept: %v", err)
	}
	if _, err := svc.Create(ctx, CreateInput{Name: "X", PriorityID: &bad}); !errors.As(err, &ve) || ve.Fields["priority_id"] == "" {
		t.Fatalf("unknown priority: %v", err)
	}
	created, err := svc.Create(ctx, CreateInput{Name: "Refunds", DeptID: &dept.ID, PriorityID: &prio.ID})
	if err != nil || !created.IsActive || created.SortOrder != 0 {
		t.Fatalf("create: %+v %v", created, err)
	}
	inactive := false
	order := int32(5)
	updated, err := svc.Update(ctx, created.ID, UpdateInput{IsActive: &inactive, SortOrder: &order})
	if err != nil || updated.IsActive || updated.SortOrder != 5 || updated.Name != "Refunds" {
		t.Fatalf("update: %+v %v", updated, err)
	}
	if _, err := svc.Get(ctx, bad); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("get missing: %v", err)
	}
	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := svc.Delete(ctx, created.ID); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
	// referenced topic cannot be deleted
	status, _ := q.DefaultStatus(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO ticket (number, subject, status_id, dept_id, topic_id, priority_id, requester_email)
		VALUES ('000001', 's', $1, $2, $3, $4, 'r@x.test')`, status.ID, dept.ID, list[0].ID, prio.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, list[0].ID); !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("delete referenced: %v", err)
	}
}

// TestDeleteTopicForeignKeyViolation proves db.IsForeignKeyViolation
// recognizes the real FK error DeleteTopic raises when a topic is still
// referenced. This is the belt-and-braces mapping the service falls back on
// beside its own CountTopicReferences guard: the query is called directly
// (bypassing that guard) against a topic a ticket references.
func TestDeleteTopicForeignKeyViolation(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	q := db.New(tx)
	dept, err := q.FirstDepartment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	prio, err := q.DefaultPriority(ctx)
	if err != nil {
		t.Fatal(err)
	}
	topics, err := q.ListTopics(ctx)
	if err != nil || len(topics) == 0 {
		t.Fatalf("seeded topics: %+v %v", topics, err)
	}
	status, err := q.DefaultStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO ticket (number, subject, status_id, dept_id, topic_id, priority_id, requester_email)
		VALUES ('000002', 's', $1, $2, $3, $4, 'r@x.test')`, status.ID, dept.ID, topics[0].ID, prio.ID); err != nil {
		t.Fatal(err)
	}
	err = db.WithTx(ctx, tx, func(q *db.Queries) error {
		_, err := q.DeleteTopic(ctx, topics[0].ID)
		return err
	})
	if !db.IsForeignKeyViolation(err) {
		t.Fatalf("expected a foreign key violation, got %v", err)
	}
}

func TestUpdateTopicClearsRefs(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	svc := NewService(tx)
	q := db.New(tx)
	dept, _ := q.FirstDepartment(ctx)
	prio, _ := q.DefaultPriority(ctx)
	created, err := svc.Create(ctx, CreateInput{Name: "Refunds", DeptID: &dept.ID, PriorityID: &prio.ID})
	if err != nil || created.DeptID == nil || created.PriorityID == nil {
		t.Fatalf("create: %+v %v", created, err)
	}
	// Absent refs leave them unchanged.
	name := "Refunds 2"
	kept, err := svc.Update(ctx, created.ID, UpdateInput{Name: &name})
	if err != nil || kept.DeptID == nil || kept.PriorityID == nil {
		t.Fatalf("absent refs changed them: %+v %v", kept, err)
	}
	onlyPrio, err := svc.Update(ctx, created.ID, UpdateInput{ClearPriority: true})
	if err != nil || onlyPrio.PriorityID != nil || onlyPrio.DeptID == nil || *onlyPrio.DeptID != dept.ID {
		t.Fatalf("clear priority: %+v %v", onlyPrio, err)
	}
	cleared, err := svc.Update(ctx, created.ID, UpdateInput{ClearDept: true})
	if err != nil || cleared.DeptID != nil || cleared.PriorityID != nil {
		t.Fatalf("clear dept: %+v %v", cleared, err)
	}
	got, err := svc.Get(ctx, created.ID)
	if err != nil || got.DeptID != nil || got.PriorityID != nil || got.Name != name {
		t.Fatalf("row after clear: %+v %v", got, err)
	}
}
