package importer

import (
	"context"
	"strings"
	"time"
)

// topicActive is osTicket's Topic::FLAG_ACTIVE (0x0002).
func topicActive(flags uint32) bool { return flags&2 != 0 }

func importTopics(ctx context.Context, src *Source, w *Writer, lk *Lookup, rep *Report) error {
	seed, err := seedByName(ctx, w, lk, "help_topic")
	if err != nil {
		return err
	}
	items, err := src.Topics(ctx)
	if err != nil {
		return err
	}
	now := time.Now()
	var batch [][]any
	for _, tp := range items {
		rep.Read(EntityTopics)
		var tc textCleaner
		name := strings.TrimSpace(tc.clean(tp.Name))
		tc.note(rep, EntityTopics, tp.ID)
		if id, ok := seed[strings.ToLower(name)]; ok {
			lk.Topics[tp.ID] = id
			rep.Merged(EntityTopics)
			continue
		}
		var dept, prio *int64
		if d, ok := lk.Departments[tp.DeptID]; ok {
			dept = &d
		}
		if p, ok := lk.Priorities[tp.PriorityID]; ok {
			prio = &p
		}
		id := allocIDNoted(lk, rep, EntityTopics, "help_topic", tp.ID)
		lk.Topics[tp.ID] = id
		created := orZero(tp.Created, now)
		batch = append(batch, []any{id, name, dept, prio, topicActive(tp.Flags), tp.Sort, created, orZero(tp.Updated, created)})
		rep.Written(EntityTopics)
	}
	return w.Insert(ctx, "help_topic", []string{"id", "name", "dept_id", "priority_id", "is_active", "sort_order", "created_at", "updated_at"}, batch)
}
