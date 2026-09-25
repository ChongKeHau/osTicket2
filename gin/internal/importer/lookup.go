package importer

import (
	"context"
	"fmt"
)

// IDMap maps a source (MySQL) id to the target (Postgres) id.
type IDMap map[int64]int64

// Lookup carries the id maps and defaults built by earlier steps for later ones.
type Lookup struct {
	Priorities, Statuses, Departments, Staff, Topics, Tickets, Entries, Files IDMap
	// DeletedStatus marks source status ids whose state is "deleted"; tickets in them are skipped.
	DeletedStatus map[int64]bool
	// Seed ids used as fallbacks: priority "normal", department "Support", status "Open".
	DefaultPriority, DefaultDept, DefaultStatus int64
	// PendingManagers holds target department id → source manager staff id, applied after staff import.
	PendingManagers map[int64]int64
	// TakenIDs holds, per table, the ids already present in the target (seed rows and inserted rows).
	TakenIDs map[string]map[int64]bool
}

// NewLookup returns a Lookup with every map initialised.
func NewLookup() *Lookup {
	return &Lookup{
		Priorities: IDMap{}, Statuses: IDMap{}, Departments: IDMap{}, Staff: IDMap{},
		Topics: IDMap{}, Tickets: IDMap{}, Entries: IDMap{}, Files: IDMap{},
		DeletedStatus:   map[int64]bool{},
		PendingManagers: map[int64]int64{},
		TakenIDs:        map[string]map[int64]bool{},
	}
}

// allocID keeps the source id when it is free in the target table, otherwise
// hands out the next id above everything seen so far. Either way the id is
// recorded as taken. Seed rows must be registered first with markTaken.
func (lk *Lookup) allocID(table string, srcID int64) int64 {
	taken := lk.TakenIDs[table]
	if taken == nil {
		taken = map[int64]bool{}
		lk.TakenIDs[table] = taken
	}
	id := srcID
	if taken[id] {
		id = 0
		for k := range taken {
			if k > id {
				id = k
			}
		}
		id++
	}
	taken[id] = true
	return id
}

// allocIDNoted is allocID that also notes, under entity, a source id that had
// to be renumbered because it collided with a seed or earlier row.
func allocIDNoted(lk *Lookup, rep *Report, e Entity, table string, srcID int64) int64 {
	id := lk.allocID(table, srcID)
	if id != srcID {
		rep.Note(e, srcID, fmt.Sprintf("id remapped to %d", id))
	}
	return id
}

// markTaken registers an id that already exists in the target table.
func (lk *Lookup) markTaken(table string, id int64) {
	if lk.TakenIDs[table] == nil {
		lk.TakenIDs[table] = map[int64]bool{}
	}
	lk.TakenIDs[table][id] = true
}

// seedByName loads the target table's existing rows as lower(name) → id and marks their ids taken.
func seedByName(ctx context.Context, w *Writer, lk *Lookup, table string) (map[string]int64, error) {
	rows, err := w.Query(ctx, "SELECT id, lower(name) FROM "+table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[name] = id
		lk.markTaken(table, id)
	}
	return out, rows.Err()
}
