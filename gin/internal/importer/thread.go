package importer

import (
	"context"
	"strings"
	"time"
)

func mapEntryType(t string) (string, bool) {
	switch t {
	case "M":
		return "message", true
	case "R":
		return "response", true
	case "N":
		return "note", true
	default:
		return "", false
	}
}

func mapFormat(f string) string {
	if strings.EqualFold(f, "text") {
		return "text"
	}
	return "html"
}

func importEntries(ctx context.Context, src *Source, w *Writer, lk *Lookup, rep *Report) error {
	threads, err := src.Threads(ctx)
	if err != nil {
		return err
	}
	type parentLink struct{ child, parent, thread int64 }
	var links []parentLink
	entryThread := map[int64]int64{} // target entry id → source thread id
	now := time.Now()
	var batch [][]any
	err = src.Entries(ctx, func(e SrcEntry) error {
		rep.Read(EntityEntries)
		srcTicket, ok := threads[e.ThreadID]
		if !ok {
			rep.Skip(EntityEntries, e.ID, "thread is not a ticket thread")
			return nil
		}
		ticketID, ok := lk.Tickets[srcTicket]
		if !ok {
			rep.Skip(EntityEntries, e.ID, "ticket not imported")
			return nil
		}
		kind, ok := mapEntryType(e.Type)
		if !ok {
			rep.Skip(EntityEntries, e.ID, "unsupported entry type "+e.Type)
			return nil
		}
		var staff *int64
		if s, ok := lk.Staff[e.StaffID]; ok {
			staff = &s
		}
		var title *string
		if t := strings.TrimSpace(e.Title); t != "" {
			title = &t
		}
		id := lk.allocID("thread_entry", e.ID)
		lk.Entries[e.ID] = id
		entryThread[id] = e.ThreadID
		if e.PID != 0 {
			links = append(links, parentLink{child: id, parent: e.PID, thread: e.ThreadID})
		}
		created := orZero(e.Created, now)
		batch = append(batch, []any{id, ticketID, kind, staff, e.Poster, title, e.Body, mapFormat(e.Format), created, orZero(e.Updated, created)})
		rep.Written(EntityEntries)
		return nil
	})
	if err != nil {
		return err
	}
	if err := w.Insert(ctx, "thread_entry", []string{"id", "ticket_id", "type", "staff_id", "poster", "title", "body", "format", "created_at", "updated_at"}, batch); err != nil {
		return err
	}
	// Second pass: parents may have higher ids than their children, so link after all rows exist.
	for _, l := range links {
		parent, ok := lk.Entries[l.parent]
		if !ok || entryThread[parent] != l.thread {
			continue
		}
		if err := w.Exec(ctx, "UPDATE thread_entry SET parent_id = $1 WHERE id = $2", parent, l.child); err != nil {
			return err
		}
	}
	return nil
}
