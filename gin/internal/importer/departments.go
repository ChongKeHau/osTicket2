package importer

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// dedupeName trims and makes the name unique (case-insensitive) with a " (n)" suffix.
func dedupeName(name string, taken map[string]bool) string {
	base := strings.TrimSpace(name)
	cand := base
	for n := 2; taken[strings.ToLower(cand)]; n++ {
		cand = fmt.Sprintf("%s (%d)", base, n)
	}
	taken[strings.ToLower(cand)] = true
	return cand
}

func importDepartmentsPass1(ctx context.Context, src *Source, w *Writer, lk *Lookup, rep *Report) error {
	seed, err := seedByName(ctx, w, lk, "department")
	if err != nil {
		return err
	}
	lk.DefaultDept = seed["support"]
	taken := map[string]bool{}
	for name := range seed {
		taken[name] = true
	}
	items, err := src.Departments(ctx)
	if err != nil {
		return err
	}
	now := time.Now()
	var batch [][]any
	for _, d := range items {
		rep.Read(EntityDepartments)
		var tc textCleaner
		d.Name = tc.clean(d.Name)
		tc.note(rep, EntityDepartments, d.ID)
		trimmed := strings.TrimSpace(d.Name)
		if id, ok := seed[strings.ToLower(trimmed)]; ok {
			lk.Departments[d.ID] = id
			if d.ManagerID != 0 {
				lk.PendingManagers[id] = d.ManagerID
			}
			rep.Merged(EntityDepartments)
			continue
		}
		name := dedupeName(d.Name, taken)
		if name != trimmed {
			rep.Note(EntityDepartments, d.ID, "renamed to "+name)
		}
		id := lk.allocID("department", d.ID)
		lk.Departments[d.ID] = id
		if d.ManagerID != 0 {
			lk.PendingManagers[id] = d.ManagerID
		}
		created := orZero(d.Created, now)
		batch = append(batch, []any{id, name, d.IsPublic, created, orZero(d.Updated, created)})
		rep.Written(EntityDepartments)
	}
	return w.Insert(ctx, "department", []string{"id", "name", "is_public", "created_at", "updated_at"}, batch)
}

// importDepartmentsPass2 sets manager_id now that staff exist.
func importDepartmentsPass2(ctx context.Context, _ *Source, w *Writer, lk *Lookup, rep *Report) error {
	for deptID, srcManager := range lk.PendingManagers {
		staffID, ok := lk.Staff[srcManager]
		if !ok {
			rep.Note(EntityDepartments, deptID, fmt.Sprintf("manager staff %d not imported", srcManager))
			continue
		}
		if err := w.Exec(ctx, "UPDATE department SET manager_id = $1 WHERE id = $2", staffID, deptID); err != nil {
			return err
		}
	}
	return nil
}
