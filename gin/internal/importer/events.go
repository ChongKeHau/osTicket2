package importer

import (
	"context"
	"encoding/json"
	"time"
)

var eventKinds = map[string]string{
	"created": "created", "closed": "closed", "reopened": "reopened",
	"assigned": "assigned", "transferred": "transferred", "edited": "edited",
}

// mapEvent maps an osTicket event name and data blob to a ticket_event kind and JSON payload.
func mapEvent(name, data string) (string, []byte, bool) {
	kind, ok := eventKinds[name]
	if !ok {
		return "", nil, false
	}
	var obj map[string]any
	payload := []byte("{}")
	if data != "" {
		if err := json.Unmarshal([]byte(data), &obj); err == nil && obj != nil {
			payload = []byte(data)
		} else {
			payload, _ = json.Marshal(map[string]string{"raw": data})
		}
	}
	if kind == "assigned" {
		staff, _ := obj["staff"].(float64)
		if staff <= 0 {
			kind = "unassigned"
		}
	}
	return kind, payload, true
}

func importEvents(ctx context.Context, src *Source, w *Writer, lk *Lookup, rep *Report) error {
	threads, err := src.Threads(ctx)
	if err != nil {
		return err
	}
	now := time.Now()
	var batch [][]any
	err = src.Events(ctx, func(e SrcEvent) error {
		rep.Read(EntityEvents)
		if e.Annulled {
			rep.Skip(EntityEvents, e.ID, "annulled")
			return nil
		}
		ticketID, ok := lk.Tickets[threads[e.ThreadID]]
		if !ok {
			rep.Skip(EntityEvents, e.ID, "ticket not imported")
			return nil
		}
		kind, payload, ok := mapEvent(e.Name, e.Data)
		if !ok {
			rep.Skip(EntityEvents, e.ID, "unmapped event "+e.Name)
			return nil
		}
		var staff *int64
		if s, ok := lk.Staff[e.StaffID]; ok {
			staff = &s
		}
		id := lk.allocID("ticket_event", e.ID)
		batch = append(batch, []any{id, ticketID, staff, kind, payload, orZero(e.Timestamp, now)})
		rep.Written(EntityEvents)
		return nil
	})
	if err != nil {
		return err
	}
	return w.Insert(ctx, "ticket_event", []string{"id", "ticket_id", "staff_id", "kind", "data", "created_at"}, batch)
}

// setLastResponse derives last_response_at from the newest response entry of each ticket.
func setLastResponse(ctx context.Context, w *Writer) error {
	return w.Exec(ctx, `UPDATE ticket t SET last_response_at = r.latest
		FROM (SELECT ticket_id, max(created_at) AS latest FROM thread_entry WHERE type = 'response' GROUP BY ticket_id) r
		WHERE r.ticket_id = t.id`)
}
