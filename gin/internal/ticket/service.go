package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/httpx"
	"github.com/grandpine/ticket-api/internal/mail"
	"github.com/jackc/pgx/v5"
)

type Service interface {
	Create(ctx context.Context, p auth.Principal, in CreateInput) (*Ticket, error)
	Get(ctx context.Context, p auth.Principal, id int64) (*Ticket, error)
	List(ctx context.Context, p auth.Principal, f ListFilter) (*httpx.List[Ticket], error)
	Update(ctx context.Context, p auth.Principal, id int64, in UpdateInput) (*Ticket, error)
	ListPriorities(ctx context.Context) ([]Priority, error)
	ListStatuses(ctx context.Context) ([]Status, error)

	Reply(ctx context.Context, p auth.Principal, id int64, in ReplyInput) (*Entry, error)
	Note(ctx context.Context, p auth.Principal, id int64, in NoteInput) (*Entry, error)
	Thread(ctx context.Context, p auth.Principal, id int64, after int64, limit int) (*Thread, error)
	SetStatus(ctx context.Context, p auth.Principal, id int64, statusID int64) (*Ticket, error)
	Assign(ctx context.Context, p auth.Principal, id int64, staffID *int64) (*Ticket, error)
	Transfer(ctx context.Context, p auth.Principal, id int64, deptID int64) (*Ticket, error)
	Events(ctx context.Context, p auth.Principal, id int64) ([]Event, error)

	CreateExternal(ctx context.Context, in ExternalCreateInput) (*Ticket, error)
	AppendMessage(ctx context.Context, ticketID int64, in MessageInput) (*Entry, error)
}

// SystemPrincipal acts for changes that no staff member made (inbound mail).
var SystemPrincipal = auth.Principal{IsAdmin: true}

// ExternalCreateInput opens a ticket on behalf of an outside requester (inbound mail).
type ExternalCreateInput struct {
	Subject, Body, Format         string
	RequesterName, RequesterEmail string
	DeptID                        int64
	FileIDs                       []int64
	AutoSubmitted                 bool
}

// MessageInput appends a requester message (inbound mail) to a ticket.
type MessageInput struct {
	Poster, Body, Format string
	FileIDs              []int64
}

type service struct {
	db       db.Beginner
	notifier mail.Notifier
}

// Option configures NewService.
type Option func(*service)

// WithNotifier makes the service queue email notifications.
func WithNotifier(n mail.Notifier) Option { return func(s *service) { s.notifier = n } }

func NewService(b db.Beginner, opts ...Option) Service {
	s := &service{db: b, notifier: mail.Disabled{}}
	for _, o := range opts {
		o(s)
	}
	return s
}

func notFound(id int64) error { return fmt.Errorf("ticket %d: %w", id, apperr.ErrNotFound) }

// ticketVars fills the ticket-derived template variables; the notifier adds site and link.
// format is the resolved (stored) body format, so the mail renders the body
// exactly as the thread entry does.
func ticketVars(row db.GetTicketRow, agent, body string, format db.BodyFormat) mail.Vars {
	v := mail.Vars{Number: row.Number, Subject: row.Subject, RequesterName: row.RequesterName, RequesterEmail: row.RequesterEmail, AgentName: agent}
	if v.RequesterName == "" {
		v.RequesterName = row.RequesterEmail
	}
	v.Message, v.MessageHTML = mail.BodyVars(body, string(format))
	return v
}

func (s *service) Create(ctx context.Context, p auth.Principal, in CreateInput) (*Ticket, error) {
	return s.create(ctx, p, in, &p.StaffID, "")
}

func (s *service) CreateExternal(ctx context.Context, in ExternalCreateInput) (*Ticket, error) {
	dept := in.DeptID
	return s.create(ctx, SystemPrincipal, CreateInput{
		Subject: in.Subject, Message: in.Body, MessageFormat: in.Format,
		RequesterName: in.RequesterName, RequesterEmail: in.RequesterEmail,
		DeptID: &dept, Source: "email", FileIDs: in.FileIDs, AutoSubmitted: in.AutoSubmitted,
	}, nil, "email")
}

// via records how the ticket was created (e.g. "email" for inbound mail) on
// the created event's data; empty means the normal staff/API path.
func (s *service) create(ctx context.Context, p auth.Principal, in CreateInput, actor *int64, via string) (*Ticket, error) {
	if err := validateExtra(in.Extra); err != nil {
		return nil, err
	}
	var out *Ticket
	err := db.WithTx(ctx, s.db, func(q *db.Queries) error {
		fields := map[string]string{}
		deptID, priorityID := in.DeptID, in.PriorityID
		if in.TopicID != nil {
			topic, err := q.GetTopic(ctx, *in.TopicID)
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				fields["topic_id"] = "unknown topic"
			case err != nil:
				return err
			default:
				if deptID == nil {
					deptID = topic.DeptID
				}
				if priorityID == nil {
					priorityID = topic.PriorityID
				}
			}
		}
		if deptID == nil {
			fields["dept_id"] = "required when no topic supplies a department"
		} else {
			// Visibility runs before existence: an agent probing a
			// nonexistent department id must get the same 403 as a real
			// but invisible one, so the 400/403 split can't be used to
			// enumerate department ids.
			if !p.CanSeeDept(*deptID) {
				return fmt.Errorf("%w: cannot create tickets in that department", apperr.ErrForbidden)
			}
			if _, err := q.GetDepartment(ctx, *deptID); errors.Is(err, pgx.ErrNoRows) {
				fields["dept_id"] = "unknown department"
			} else if err != nil {
				return err
			}
		}
		if priorityID == nil {
			def, err := q.DefaultPriority(ctx)
			if err != nil {
				return err
			}
			priorityID = &def.ID
		} else if _, err := q.GetPriority(ctx, *priorityID); errors.Is(err, pgx.ErrNoRows) {
			fields["priority_id"] = "unknown priority"
		} else if err != nil {
			return err
		}
		if len(fields) > 0 {
			return &apperr.ValidationError{Fields: fields}
		}
		status, err := q.DefaultStatus(ctx)
		if err != nil {
			return err
		}
		n, err := q.NextTicketNumber(ctx)
		if err != nil {
			return err
		}
		number := fmt.Sprintf("%06d", n)
		source := db.TicketSourceWeb
		if in.Source != "" {
			source = db.TicketSource(in.Source)
		}
		extra := []byte("{}")
		if len(in.Extra) > 0 {
			extra = in.Extra
		}
		id, err := q.CreateTicket(ctx, db.CreateTicketParams{
			Number: number, Subject: in.Subject, StatusID: status.ID, DeptID: *deptID, TopicID: in.TopicID,
			PriorityID: *priorityID, RequesterName: in.RequesterName, RequesterEmail: in.RequesterEmail,
			Source: source, DueAt: in.DueAt, Extra: extra,
		})
		if err != nil {
			return err
		}
		poster := in.RequesterName
		if poster == "" {
			poster = in.RequesterEmail
		}
		format := bodyFormat(in.MessageFormat)
		entry, err := q.CreateThreadEntry(ctx, db.CreateThreadEntryParams{
			TicketID: id, Type: db.ThreadEntryTypeMessage, Poster: poster, Body: in.Message, Format: format,
		})
		if err != nil {
			return err
		}
		if err := attachFiles(ctx, q, p, entry.ID, in.FileIDs); err != nil {
			return err
		}
		data := map[string]any{"number": number}
		if via != "" {
			data["via"] = via
		}
		if err := event(ctx, q, id, actor, db.TicketEventKindCreated, data); err != nil {
			return err
		}
		if in.RequesterEmail != "" && !in.AutoSubmitted {
			row, err := q.GetTicket(ctx, id)
			if err != nil {
				return err
			}
			eid := entry.ID
			if err := s.notifier.Enqueue(ctx, q, mail.Notification{
				TemplateKey: "ticket_autoresp", TicketID: &id, EntryID: &eid, AutoSubmitted: true,
				To:   []mail.Recipient{{Name: in.RequesterName, Address: in.RequesterEmail}},
				Vars: ticketVars(row, "", in.Message, format),
			}); err != nil {
				return err
			}
		}
		out, err = get(ctx, q, p, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *service) Get(ctx context.Context, p auth.Principal, id int64) (*Ticket, error) {
	return get(ctx, db.New(s.db), p, id)
}

var sortKeys = map[string]bool{
	"created_at": true, "-created_at": true, "last_message_at": true, "-last_message_at": true,
	"priority": true, "-priority": true,
}

func validState(s string) bool {
	switch s {
	case "open", "resolved", "closed":
		return true
	default:
		return false
	}
}

func (s *service) List(ctx context.Context, p auth.Principal, f ListFilter) (*httpx.List[Ticket], error) {
	fields := map[string]string{}
	sort := f.Sort
	if sort == "" {
		sort = "-last_message_at"
	}
	if !sortKeys[sort] {
		fields["sort"] = "unknown sort key"
	}
	var state *string
	if f.State != "" {
		if !validState(f.State) {
			fields["state"] = "must be open, resolved or closed"
		}
		st := f.State
		state = &st
	}
	var assigned *int64
	unassigned := false
	switch f.AssignedTo {
	case "":
	case "me":
		id := p.StaffID
		assigned = &id
	case "none":
		unassigned = true
	default:
		n, err := strconv.ParseInt(f.AssignedTo, 10, 64)
		if err != nil || n <= 0 {
			fields["assigned_to"] = "must be a staff id, me or none"
		} else {
			assigned = &n
		}
	}
	if len(fields) > 0 {
		return nil, &apperr.ValidationError{Fields: fields}
	}
	deptIDs := p.DeptIDs
	if deptIDs == nil {
		deptIDs = []int64{}
	}
	q := db.New(s.db)
	rows, err := q.ListTickets(ctx, db.ListTicketsParams{
		AllDepts: p.IsAdmin, DeptIds: deptIDs, StatusID: f.StatusID, State: state, FilterDeptID: f.DeptID,
		AssignedStaffID: assigned, Unassigned: unassigned, Q: f.Q, Sort: sort,
		PageSize: f.Page.Limit(), PageOffset: f.Page.Offset(),
	})
	if err != nil {
		return nil, err
	}
	total, err := q.CountTickets(ctx, db.CountTicketsParams{
		AllDepts: p.IsAdmin, DeptIds: deptIDs, StatusID: f.StatusID, State: state, FilterDeptID: f.DeptID,
		AssignedStaffID: assigned, Unassigned: unassigned, Q: f.Q,
	})
	if err != nil {
		return nil, err
	}
	items := make([]Ticket, 0, len(rows))
	for _, r := range rows {
		items = append(items, fromRow(r))
	}
	return &httpx.List[Ticket]{Items: items, Page: f.Page.Page, PageSize: f.Page.PageSize, Total: total}, nil
}

func (s *service) Update(ctx context.Context, p auth.Principal, id int64, in UpdateInput) (*Ticket, error) {
	if err := validateExtra(in.Extra); err != nil {
		return nil, err
	}
	var out *Ticket
	err := db.WithTx(ctx, s.db, func(q *db.Queries) error {
		if err := q.LockTicket(ctx, id); err != nil {
			return err
		}
		if _, err := loadVisible(ctx, q, p, id); err != nil {
			return err
		}
		fields := map[string]string{}
		if in.PriorityID != nil {
			if _, err := q.GetPriority(ctx, *in.PriorityID); errors.Is(err, pgx.ErrNoRows) {
				fields["priority_id"] = "unknown priority"
			} else if err != nil {
				return err
			}
		}
		if in.TopicID != nil {
			if _, err := q.GetTopic(ctx, *in.TopicID); errors.Is(err, pgx.ErrNoRows) {
				fields["topic_id"] = "unknown topic"
			} else if err != nil {
				return err
			}
		}
		if len(fields) > 0 {
			return &apperr.ValidationError{Fields: fields}
		}
		var extra []byte
		if len(in.Extra) > 0 {
			extra = in.Extra
		}
		if err := q.UpdateTicket(ctx, db.UpdateTicketParams{
			ID: id, Subject: in.Subject, PriorityID: in.PriorityID, TopicID: in.TopicID, DueAt: in.DueAt,
			ClearTopic: in.ClearTopic, ClearDueAt: in.ClearDueAt,
			Extra: extra, RequesterName: in.RequesterName, RequesterEmail: in.RequesterEmail,
		}); err != nil {
			return err
		}
		if err := event(ctx, q, id, &p.StaffID, db.TicketEventKindEdited, map[string]any{"fields": changedFields(in)}); err != nil {
			return err
		}
		var err error
		out, err = get(ctx, q, p, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *service) ListPriorities(ctx context.Context) ([]Priority, error) {
	rows, err := db.New(s.db).ListPriorities(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Priority, 0, len(rows))
	for _, r := range rows {
		out = append(out, Priority{ID: r.ID, Name: r.Name, Urgency: r.Urgency, Color: r.Color})
	}
	return out, nil
}

func (s *service) ListStatuses(ctx context.Context) ([]Status, error) {
	rows, err := db.New(s.db).ListStatuses(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Status, 0, len(rows))
	for _, r := range rows {
		out = append(out, Status{ID: r.ID, Name: r.Name, State: string(r.State), SortOrder: r.SortOrder})
	}
	return out, nil
}

// loadVisible returns the raw ticket row or ErrNotFound when missing or invisible.
func loadVisible(ctx context.Context, q *db.Queries, p auth.Principal, id int64) (db.GetTicketRow, error) {
	row, err := q.GetTicket(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.GetTicketRow{}, notFound(id)
	}
	if err != nil {
		return db.GetTicketRow{}, err
	}
	if !p.CanSeeDept(row.DeptID) {
		return db.GetTicketRow{}, notFound(id)
	}
	return row, nil
}

func get(ctx context.Context, q *db.Queries, p auth.Principal, id int64) (*Ticket, error) {
	row, err := loadVisible(ctx, q, p, id)
	if err != nil {
		return nil, err
	}
	t := fromRow(db.ListTicketsRow(row))
	return &t, nil
}

func fromRow(r db.ListTicketsRow) Ticket {
	t := Ticket{
		ID: r.ID, Number: r.Number, Subject: r.Subject,
		Status: Ref{ID: r.StatusID, Name: r.StatusName}, State: string(r.StatusState),
		Department:    Ref{ID: r.DeptID, Name: r.DeptName},
		Priority:      Ref{ID: r.PriorityID, Name: r.PriorityName},
		RequesterName: r.RequesterName, RequesterEmail: r.RequesterEmail, Source: string(r.Source),
		IsAnswered: r.IsAnswered, DueAt: utcPtr(r.DueAt), ClosedAt: utcPtr(r.ClosedAt),
		LastMessageAt: r.LastMessageAt.UTC(), LastResponseAt: utcPtr(r.LastResponseAt),
		Extra: json.RawMessage(r.Extra), CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
	if r.TopicID != nil && r.TopicName != nil {
		t.Topic = &Ref{ID: *r.TopicID, Name: *r.TopicName}
	}
	if r.AssignedStaffID != nil {
		t.Assignee = &Ref{ID: *r.AssignedStaffID, Name: fullName(r.AssigneeFirstName, r.AssigneeLastName)}
	}
	return t
}

func fullName(first, last *string) string {
	var parts []string
	for _, s := range []*string{first, last} {
		if s != nil && *s != "" {
			parts = append(parts, *s)
		}
	}
	return strings.Join(parts, " ")
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func bodyFormat(s string) db.BodyFormat {
	if s == "text" {
		return db.BodyFormatText
	}
	return db.BodyFormatHtml
}

func validateExtra(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if !json.Valid(raw) || !strings.HasPrefix(trimmed, "{") {
		return apperr.Validation("extra", "must be a JSON object")
	}
	return nil
}

func changedFields(in UpdateInput) []string {
	var out []string
	if in.Subject != nil {
		out = append(out, "subject")
	}
	if in.PriorityID != nil {
		out = append(out, "priority_id")
	}
	if in.TopicID != nil || in.ClearTopic {
		out = append(out, "topic_id")
	}
	if in.DueAt != nil || in.ClearDueAt {
		out = append(out, "due_at")
	}
	if len(in.Extra) > 0 {
		out = append(out, "extra")
	}
	if in.RequesterName != nil {
		out = append(out, "requester_name")
	}
	if in.RequesterEmail != nil {
		out = append(out, "requester_email")
	}
	if out == nil {
		out = []string{}
	}
	return out
}

func event(ctx context.Context, q *db.Queries, ticketID int64, staffID *int64, kind db.TicketEventKind, data map[string]any) error {
	if data == nil {
		data = map[string]any{}
	}
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return q.CreateTicketEvent(ctx, db.CreateTicketEventParams{TicketID: ticketID, StaffID: staffID, Kind: kind, Data: b})
}
