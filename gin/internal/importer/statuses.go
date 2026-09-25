package importer

import (
	"context"
	"strings"
)

// mapStatusState maps an osTicket status to the API's ticket_state. ok is false for deleted.
func mapStatusState(name, state string) (string, bool) {
	switch strings.ToLower(state) {
	case "deleted":
		return "", false
	case "closed":
		if strings.EqualFold(strings.TrimSpace(name), "resolved") {
			return "resolved", true
		}
		return "closed", true
	case "archived":
		return "closed", true
	default:
		return "open", true
	}
}

func importStatuses(ctx context.Context, src *Source, w *Writer, lk *Lookup, rep *Report) error {
	seed, err := seedByName(ctx, w, lk, "ticket_status")
	if err != nil {
		return err
	}
	lk.DefaultStatus = seed["open"]
	items, err := src.Statuses(ctx)
	if err != nil {
		return err
	}
	var batch [][]any
	for _, st := range items {
		rep.Read(EntityStatuses)
		state, ok := mapStatusState(st.Name, st.State)
		if !ok {
			lk.DeletedStatus[st.ID] = true
			rep.Skip(EntityStatuses, st.ID, ReasonDeletedStatus)
			continue
		}
		var tc textCleaner
		name := strings.TrimSpace(tc.clean(st.Name))
		tc.note(rep, EntityStatuses, st.ID)
		key := strings.ToLower(name)
		if id, ok := seed[key]; ok {
			lk.Statuses[st.ID] = id
			rep.Merged(EntityStatuses)
			continue
		}
		id := allocIDNoted(lk, rep, EntityStatuses, "ticket_status", st.ID)
		seed[key] = id
		lk.Statuses[st.ID] = id
		batch = append(batch, []any{id, name, state, st.Sort})
		rep.Written(EntityStatuses)
	}
	return w.Insert(ctx, "ticket_status", []string{"id", "name", "state", "sort_order"}, batch)
}
