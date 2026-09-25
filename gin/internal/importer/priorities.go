package importer

import (
	"context"
	"strings"
)

func importPriorities(ctx context.Context, src *Source, w *Writer, lk *Lookup, rep *Report) error {
	seed, err := seedByName(ctx, w, lk, "ticket_priority")
	if err != nil {
		return err
	}
	lk.DefaultPriority = seed["normal"]
	items, err := src.Priorities(ctx)
	if err != nil {
		return err
	}
	var batch [][]any
	for _, p := range items {
		rep.Read(EntityPriorities)
		var tc textCleaner
		name := strings.TrimSpace(tc.clean(p.Name))
		color := tc.clean(p.Color)
		tc.note(rep, EntityPriorities, p.ID)
		key := strings.ToLower(name)
		if id, ok := seed[key]; ok {
			lk.Priorities[p.ID] = id
			rep.Merged(EntityPriorities)
			continue
		}
		id := lk.allocID("ticket_priority", p.ID)
		seed[key] = id
		lk.Priorities[p.ID] = id
		batch = append(batch, []any{id, name, p.Urgency, color})
		rep.Written(EntityPriorities)
	}
	return w.Insert(ctx, "ticket_priority", []string{"id", "name", "urgency", "color"}, batch)
}
