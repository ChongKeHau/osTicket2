package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func mapSource(s string) string {
	switch strings.ToLower(s) {
	case "web":
		return "web"
	case "phone":
		return "phone"
	case "api":
		return "api"
	default:
		return "other"
	}
}

// requesterEmail resolves the ticket's email row, then the user's default, then a placeholder.
func requesterEmail(tk SrcTicket, users map[int64]SrcUser, emails map[int64]SrcUserEmail) (string, bool) {
	if e, ok := emails[tk.UserEmailID]; ok && strings.TrimSpace(e.Address) != "" {
		return strings.TrimSpace(e.Address), false
	}
	if u, ok := users[tk.UserID]; ok {
		if e, ok := emails[u.DefaultEmailID]; ok && strings.TrimSpace(e.Address) != "" {
			return strings.TrimSpace(e.Address), false
		}
	}
	return fmt.Sprintf("unknown-%d@imported.invalid", tk.ID), true
}

// ticketNumber keeps the source number when present and unique.
func ticketNumber(tk SrcTicket, taken map[string]bool) (string, bool) {
	n := strings.TrimSpace(tk.Number)
	changed := false
	if n == "" {
		n = fmt.Sprintf("%06d", tk.ID)
		changed = true
	}
	if taken[n] {
		n = fmt.Sprintf("%s-%d", n, tk.ID)
		changed = true
	}
	taken[n] = true
	return n, changed
}

// ticketExtra keeps the dropped columns as JSON so nothing is lost.
func ticketExtra(tk SrcTicket) ([]byte, error) {
	m := map[string]any{}
	if tk.SLAID != 0 {
		m["sla_id"] = tk.SLAID
	}
	if tk.TeamID != 0 {
		m["team_id"] = tk.TeamID
	}
	if tk.Flags != 0 {
		m["flags"] = tk.Flags
	}
	if tk.IPAddress != "" {
		m["ip_address"] = tk.IPAddress
	}
	if tk.SourceExtra != "" {
		m["source_extra"] = tk.SourceExtra
	}
	if tk.Source != "" {
		m["source"] = tk.Source
	}
	if tk.EmailID != 0 {
		m["email_id"] = tk.EmailID
	}
	if tk.UserID != 0 {
		m["user_id"] = tk.UserID
	}
	return json.Marshal(m)
}

func importTickets(ctx context.Context, src *Source, w *Writer, lk *Lookup, rep *Report) error {
	users, err := src.Users(ctx)
	if err != nil {
		return err
	}
	emails, err := src.UserEmails(ctx)
	if err != nil {
		return err
	}
	forms, err := src.FormAnswers(ctx)
	if err != nil {
		return err
	}
	// Target topic → default priority, for tickets without a priority answer.
	topicPriority := map[int64]int64{}
	rows, err := w.Query(ctx, "SELECT id, priority_id FROM help_topic WHERE priority_id IS NOT NULL")
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, p int64
		if err := rows.Scan(&id, &p); err != nil {
			rows.Close()
			return err
		}
		topicPriority[id] = p
	}
	rows.Close()
	rows, err = w.Query(ctx, "SELECT number FROM ticket")
	if err != nil {
		return err
	}
	taken := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return err
		}
		taken[n] = true
	}
	rows.Close()

	now := time.Now()
	batch := w.NewBatcher("ticket", []string{
		"id", "number", "subject", "status_id", "dept_id", "topic_id", "priority_id", "assigned_staff_id",
		"requester_name", "requester_email", "source", "is_answered", "due_at", "closed_at", "last_message_at",
		"extra", "created_at", "updated_at",
	})
	err = src.Tickets(ctx, func(tk SrcTicket) error {
		rep.Read(EntityTickets)
		if lk.DeletedStatus[tk.StatusID] {
			rep.Skip(EntityTickets, tk.ID, ReasonDeletedStatus)
			return nil
		}
		status, ok := lk.Statuses[tk.StatusID]
		if !ok {
			status = lk.DefaultStatus
			rep.Note(EntityTickets, tk.ID, "unknown status, using Open")
		}
		dept, ok := lk.Departments[tk.DeptID]
		if !ok {
			dept = lk.DefaultDept
			rep.Note(EntityTickets, tk.ID, "unknown department, using seed department")
		}
		var topic *int64
		if t, ok := lk.Topics[tk.TopicID]; ok {
			topic = &t
		}
		var assignee *int64
		if s, ok := lk.Staff[tk.StaffID]; ok {
			assignee = &s
		}
		answers := forms[tk.ID]
		var tc textCleaner
		tk.Number = tc.clean(tk.Number)
		subject := strings.TrimSpace(tc.clean(answers["subject"].Value))
		if subject == "" {
			subject = "(no subject)"
			rep.Note(EntityTickets, tk.ID, "empty subject")
		}
		priority := lk.DefaultPriority
		resolved := false
		if a, ok := answers["priority"]; ok && a.ValueID != nil {
			if p, ok := lk.Priorities[*a.ValueID]; ok {
				priority = p
				resolved = true
			}
		}
		if !resolved && topic != nil {
			if p, ok := topicPriority[*topic]; ok {
				priority = p
			}
		}
		number, changed := ticketNumber(tk, taken)
		if changed {
			rep.Note(EntityTickets, tk.ID, "number set to "+number)
		}
		email, placeholder := requesterEmail(tk, users, emails)
		if placeholder {
			rep.Note(EntityTickets, tk.ID, "requester email replaced with "+email)
		}
		email = tc.clean(email)
		name := ""
		if u, ok := users[tk.UserID]; ok {
			name = tc.clean(u.Name)
		}
		// extra is jsonb, which also rejects NUL.
		tk.IPAddress, tk.SourceExtra = tc.clean(tk.IPAddress), tc.clean(tk.SourceExtra)
		tc.note(rep, EntityTickets, tk.ID)
		extra, err := ticketExtra(tk)
		if err != nil {
			return err
		}
		id := allocIDNoted(lk, rep, EntityTickets, "ticket", tk.ID)
		lk.Tickets[tk.ID] = id
		created := orZero(tk.Created, now)
		rep.Written(EntityTickets)
		return batch.Add(ctx, []any{
			id, number, subject, status, dept, topic, priority, assignee, name, email, mapSource(tk.Source),
			tk.IsAnswered, nullTime(tk.DueDate), nullTime(tk.Closed), orZero(tk.LastUpdate, created), extra,
			created, orZero(tk.Updated, created),
		})
	})
	if err != nil {
		return err
	}
	return batch.Flush(ctx)
}
