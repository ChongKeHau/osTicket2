package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/attachment"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/httpx"
	"github.com/grandpine/ticket-api/internal/mail"
	"github.com/grandpine/ticket-api/internal/ticket"
	"github.com/jackc/pgx/v5"
)

type Ref struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Reference is what the portal's open-a-ticket form offers.
type Reference struct {
	SiteName    string `json:"site_name"`
	Departments []Ref  `json:"departments"`
	Topics      []Ref  `json:"topics"`
}

type Opened struct {
	ID     int64  `json:"id"`
	Number string `json:"number"`
}

type TicketRow struct {
	ID            int64      `json:"id"`
	Number        string     `json:"number"`
	Subject       string     `json:"subject"`
	Status        Ref        `json:"status"`
	State         string     `json:"state"`
	Department    string     `json:"department"`
	CreatedAt     time.Time  `json:"created_at"`
	LastMessageAt time.Time  `json:"last_message_at"`
	ClosedAt      *time.Time `json:"closed_at"`
}

// TicketView is one ticket as its customer sees it: messages and responses, never notes.
type TicketView struct {
	TicketRow
	Topic     *string     `json:"topic"`
	UpdatedAt time.Time   `json:"updated_at"`
	Entries   []EntryView `json:"entries"`
}

type EntryView struct {
	ID          int64                  `json:"id"`
	Type        string                 `json:"type"`
	Poster      string                 `json:"poster"`
	Body        string                 `json:"body"`
	Format      string                 `json:"format"`
	CreatedAt   time.Time              `json:"created_at"`
	Attachments []ticket.AttachmentRef `json:"attachments"`
}

// OpenInput opens a ticket from the portal. Email is required when there is
// no session and ignored when there is one.
type OpenInput struct {
	Name    string  `json:"name" binding:"max=128"`
	Email   string  `json:"email" binding:"omitempty,email,max=255"`
	Subject string  `json:"subject" binding:"required,max=255"`
	Message string  `json:"message" binding:"required"`
	Format  string  `json:"format" binding:"omitempty,oneof=html text"`
	TopicID *int64  `json:"topic_id"`
	DeptID  *int64  `json:"dept_id"`
	FileIDs []int64 `json:"file_ids"`
	// FileTokens pairs with FileIDs: the token each portal upload returned.
	FileTokens []string `json:"file_tokens"`
}

type ReplyInput struct {
	Body       string   `json:"body" binding:"required"`
	Format     string   `json:"format" binding:"omitempty,oneof=html text"`
	FileIDs    []int64  `json:"file_ids"`
	FileTokens []string `json:"file_tokens"`
}

// PortalService is the customer side of tickets: open, list, view, reply,
// close, reopen and attachments, always scoped to the signed-in end user.
type PortalService struct {
	b         db.Beginner
	notifier  mail.Notifier
	files     attachment.Service
	ident     *Service
	openLimit *Limiter
	siteName  string
}

// NewPortalService builds the service. notifier is handed to the ticket
// service it builds inside each transaction; openLimit budgets anonymous
// ticket creation per address and per IP.
func NewPortalService(b db.Beginner, notifier mail.Notifier, files attachment.Service, ident *Service, openLimit *Limiter, siteName string) *PortalService {
	return &PortalService{b: b, notifier: notifier, files: files, ident: ident, openLimit: openLimit, siteName: siteName}
}

// ticketsIn returns a ticket service bound to q's transaction, the same way
// inbound mail processing does, so its work commits or rolls back with ours.
func (s *PortalService) ticketsIn(q *db.Queries) (ticket.Service, error) {
	b, ok := q.DB().(db.Beginner)
	if !ok {
		return nil, errors.New("portal: query connection cannot begin a transaction")
	}
	return ticket.NewService(b, ticket.WithNotifier(s.notifier)), nil
}

func notFound(id int64) error { return fmt.Errorf("ticket %d: %w", id, apperr.ErrNotFound) }

func (s *PortalService) Reference(ctx context.Context) (*Reference, error) {
	q := db.New(s.b)
	depts, err := q.ListPublicDepartments(ctx)
	if err != nil {
		return nil, err
	}
	topics, err := q.ListActiveTopics(ctx)
	if err != nil {
		return nil, err
	}
	out := &Reference{SiteName: s.siteName, Departments: make([]Ref, 0, len(depts)), Topics: make([]Ref, 0, len(topics))}
	for _, d := range depts {
		out.Departments = append(out.Departments, Ref{ID: d.ID, Name: d.Name})
	}
	for _, t := range topics {
		out.Topics = append(out.Topics, Ref{ID: t.ID, Name: t.Name})
	}
	return out, nil
}

// OpenTicket opens a ticket for the session's user (p non-nil) or, anonymously,
// for the address in the body, creating the end user if needed. Every open is
// rate-limited per address (the session's, when signed in) and per IP, so a
// session does not lift the budget.
func (s *PortalService) OpenTicket(ctx context.Context, p *Principal, ip string, in OpenInput) (*Opened, error) {
	email, name := strings.TrimSpace(in.Email), strings.TrimSpace(in.Name)
	// The portal form sends 0 for "no choice" (its select's empty option).
	in.DeptID, in.TopicID = nonZero(in.DeptID), nonZero(in.TopicID)
	if p != nil {
		email = p.Email
	}
	if email == "" {
		return nil, apperr.Validation("email", "required")
	}
	if err := s.openLimit.Check(email, ip); err != nil {
		return nil, err
	}
	var out *Opened
	err := db.WithTx(ctx, s.b, func(q *db.Queries) error {
		if err := checkChoices(ctx, q, in.DeptID, in.TopicID); err != nil {
			return err
		}
		if err := checkFiles(ctx, q, in.FileIDs, in.FileTokens); err != nil {
			return err
		}
		u, err := s.ident.UpsertByEmail(ctx, q, email, name)
		if err != nil {
			return err
		}
		svc, err := s.ticketsIn(q)
		if err != nil {
			return err
		}
		requester := u.Name
		if requester == "" {
			requester = name
		}
		var dept int64
		if in.DeptID != nil {
			dept = *in.DeptID
		}
		t, err := svc.CreateExternal(ctx, ticket.ExternalCreateInput{
			Subject: in.Subject, Body: in.Message, Format: in.Format,
			RequesterName: requester, RequesterEmail: u.Email, DeptID: dept, TopicID: in.TopicID,
			FileIDs: in.FileIDs, Source: "web", UserID: &u.ID,
		})
		if err != nil {
			return err
		}
		out = &Opened{ID: t.ID, Number: t.Number}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// nonZero maps a pointer to 0 to nil.
func nonZero(v *int64) *int64 {
	if v != nil && *v == 0 {
		return nil
	}
	return v
}

// checkChoices limits the portal to public departments and active topics.
func checkChoices(ctx context.Context, q *db.Queries, deptID, topicID *int64) error {
	fields := map[string]string{}
	if deptID != nil {
		depts, err := q.ListPublicDepartments(ctx)
		if err != nil {
			return err
		}
		found := false
		for _, d := range depts {
			found = found || d.ID == *deptID
		}
		if !found {
			fields["dept_id"] = "unknown department"
		}
	}
	if topicID != nil {
		topics, err := q.ListActiveTopics(ctx)
		if err != nil {
			return err
		}
		found := false
		for _, t := range topics {
			found = found || t.ID == *topicID
		}
		if !found {
			fields["topic_id"] = "unknown topic"
		}
	}
	if len(fields) > 0 {
		return &apperr.ValidationError{Fields: fields}
	}
	return nil
}

// checkFiles admits only unattached portal uploads whose access token the
// caller presents, pairwise with the ids. The ticket service attaches as
// SystemPrincipal, which would otherwise accept any unattached file: a staff
// member's pending upload (which has no token) or another customer's upload
// found by guessing its id.
func checkFiles(ctx context.Context, q *db.Queries, ids []int64, tokens []string) error {
	if len(ids) == 0 {
		return nil
	}
	bad := apperr.Validation("file_ids", "unknown or already used file")
	if len(tokens) != len(ids) {
		return bad
	}
	for i, id := range ids {
		if tokens[i] == "" {
			return bad
		}
		_, err := q.GetFileForPortalAttach(ctx, db.GetFileForPortalAttachParams{ID: id, AccessToken: tokens[i]})
		if errors.Is(err, pgx.ErrNoRows) {
			return bad
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func validState(s string) bool {
	switch s {
	case "", "open", "resolved", "closed":
		return true
	}
	return false
}

func (s *PortalService) ListTickets(ctx context.Context, p Principal, state string, page httpx.Page) (*httpx.List[TicketRow], error) {
	if p.IsGuest() {
		return nil, apperr.ErrGuestSession
	}
	if !validState(state) {
		return nil, apperr.Validation("state", "must be open, resolved or closed")
	}
	var st *string
	if state != "" {
		st = &state
	}
	q := db.New(s.b)
	rows, err := q.ListPortalTickets(ctx, db.ListPortalTicketsParams{UserID: &p.UserID, State: st, Lim: page.Limit(), Off: page.Offset()})
	if err != nil {
		return nil, err
	}
	total, err := q.CountPortalTickets(ctx, db.CountPortalTicketsParams{UserID: &p.UserID, State: st})
	if err != nil {
		return nil, err
	}
	items := make([]TicketRow, 0, len(rows))
	for _, r := range rows {
		items = append(items, TicketRow{
			ID: r.ID, Number: r.Number, Subject: r.Subject, Status: Ref{ID: r.StatusID, Name: r.StatusName},
			State: string(r.State), Department: r.DeptName, CreatedAt: r.CreatedAt.UTC(),
			LastMessageAt: r.LastMessageAt.UTC(), ClosedAt: utcPtr(r.ClosedAt),
		})
	}
	return &httpx.List[TicketRow]{Items: items, Page: page.Page, PageSize: page.PageSize, Total: total}, nil
}

// own loads a ticket the principal may see: their own, and for a guest
// session only the ticket the session is scoped to. Anything else is 404.
func own(ctx context.Context, q *db.Queries, p Principal, id int64) (db.GetPortalTicketRow, error) {
	if p.TicketID != nil && *p.TicketID != id {
		return db.GetPortalTicketRow{}, notFound(id)
	}
	row, err := q.GetPortalTicket(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.GetPortalTicketRow{}, notFound(id)
	}
	if err != nil {
		return db.GetPortalTicketRow{}, err
	}
	if row.UserID == nil || *row.UserID != p.UserID {
		return db.GetPortalTicketRow{}, notFound(id)
	}
	return row, nil
}

func (s *PortalService) GetTicket(ctx context.Context, p Principal, id int64) (*TicketView, error) {
	q := db.New(s.b)
	row, err := own(ctx, q, p, id)
	if err != nil {
		return nil, err
	}
	return view(ctx, q, row)
}

func view(ctx context.Context, q *db.Queries, row db.GetPortalTicketRow) (*TicketView, error) {
	entries, err := q.ListPortalThread(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.ID)
	}
	atts, err := q.ListAttachmentsForEntries(ctx, ids)
	if err != nil {
		return nil, err
	}
	byEntry := map[int64][]ticket.AttachmentRef{}
	for _, a := range atts {
		byEntry[a.ThreadEntryID] = append(byEntry[a.ThreadEntryID], ticket.AttachmentRef{FileID: a.FileID, Name: a.Name, Mime: a.Mime, Size: a.Size})
	}
	v := &TicketView{
		TicketRow: TicketRow{
			ID: row.ID, Number: row.Number, Subject: row.Subject, Status: Ref{ID: row.StatusID, Name: row.StatusName},
			State: string(row.State), Department: row.DeptName, CreatedAt: row.CreatedAt.UTC(),
			LastMessageAt: row.LastMessageAt.UTC(), ClosedAt: utcPtr(row.ClosedAt),
		},
		Topic: row.TopicName, UpdatedAt: row.UpdatedAt.UTC(), Entries: make([]EntryView, 0, len(entries)),
	}
	for _, e := range entries {
		list := byEntry[e.ID]
		if list == nil {
			list = []ticket.AttachmentRef{}
		}
		v.Entries = append(v.Entries, EntryView{
			ID: e.ID, Type: string(e.Type), Poster: posterName(e), Body: e.Body, Format: string(e.Format),
			CreatedAt: e.CreatedAt.UTC(), Attachments: list,
		})
	}
	return v, nil
}

// posterName is the customer's name for their entries, the agent's display
// name for responses, and otherwise the name stored on the entry.
func posterName(e db.ListPortalThreadRow) string {
	if e.UserID != nil && e.UserName != nil && *e.UserName != "" {
		return *e.UserName
	}
	if e.StaffID != nil {
		var parts []string
		for _, s := range []*string{e.FirstName, e.LastName} {
			if s != nil && strings.TrimSpace(*s) != "" {
				parts = append(parts, strings.TrimSpace(*s))
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, " ")
		}
		if e.Username != nil && *e.Username != "" {
			return *e.Username
		}
	}
	return e.Poster
}

// Reply appends a message from the customer. A closed ticket is first moved
// to the first open-state status; that status event carries the user id.
func (s *PortalService) Reply(ctx context.Context, p Principal, id int64, in ReplyInput) error {
	return db.WithTx(ctx, s.b, func(q *db.Queries) error {
		row, err := s.lock(ctx, q, p, id)
		if err != nil {
			return err
		}
		if err := checkFiles(ctx, q, in.FileIDs, in.FileTokens); err != nil {
			return err
		}
		svc, err := s.ticketsIn(q)
		if err != nil {
			return err
		}
		if closedLike(row.State) {
			if err := transition(ctx, q, svc, p, id, db.TicketStateOpen); err != nil {
				return err
			}
		}
		u, err := q.GetEndUser(ctx, p.UserID)
		if err != nil {
			return err
		}
		poster := u.Name
		if poster == "" {
			poster = u.Email
		}
		_, err = svc.AppendMessage(ctx, id, ticket.MessageInput{
			Poster: poster, Body: in.Body, Format: in.Format, FileIDs: in.FileIDs, UserID: &p.UserID,
		})
		return err
	})
}

// closedLike reports whether the portal shows a ticket as closed: it has no
// Resolved tab or action, so a resolved ticket counts as closed (listed under
// Closed, reopened by a reply or Reopen, and not closable again).
func closedLike(st db.TicketState) bool {
	return st == db.TicketStateClosed || st == db.TicketStateResolved
}

// Close moves the ticket to the first closed-state status; 409 when it is
// already closed or resolved.
func (s *PortalService) Close(ctx context.Context, p Principal, id int64) (*TicketView, error) {
	return s.setState(ctx, p, id, db.TicketStateClosed)
}

// Reopen moves a closed or resolved ticket to the first open-state status; 409
// when it is neither.
func (s *PortalService) Reopen(ctx context.Context, p Principal, id int64) (*TicketView, error) {
	return s.setState(ctx, p, id, db.TicketStateOpen)
}

// A guest session may change its own ticket's state; lock's ownership check
// returns 404 for any other ticket.
func (s *PortalService) setState(ctx context.Context, p Principal, id int64, target db.TicketState) (*TicketView, error) {
	var out *TicketView
	err := db.WithTx(ctx, s.b, func(q *db.Queries) error {
		row, err := s.lock(ctx, q, p, id)
		if err != nil {
			return err
		}
		switch {
		case target == db.TicketStateClosed && closedLike(row.State):
			return fmt.Errorf("%w: ticket is already closed", apperr.ErrConflict)
		case target == db.TicketStateOpen && !closedLike(row.State):
			return fmt.Errorf("%w: ticket is not closed", apperr.ErrConflict)
		}
		svc, err := s.ticketsIn(q)
		if err != nil {
			return err
		}
		if err := transition(ctx, q, svc, p, id, target); err != nil {
			return err
		}
		fresh, err := q.GetPortalTicket(ctx, id)
		if err != nil {
			return err
		}
		out, err = view(ctx, q, fresh)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// lock checks ownership and then holds the ticket row for the transaction,
// re-reading it so the state cannot change between the check and the update.
func (s *PortalService) lock(ctx context.Context, q *db.Queries, p Principal, id int64) (db.GetPortalTicketRow, error) {
	if _, err := own(ctx, q, p, id); err != nil {
		return db.GetPortalTicketRow{}, err
	}
	if err := q.LockTicket(ctx, id); err != nil {
		return db.GetPortalTicketRow{}, err
	}
	return own(ctx, q, p, id)
}

// transition sets the first status in state through the ticket service (as
// SystemPrincipal, so the event has no staff_id) and then adds the end user
// to that event's data with a follow-up UPDATE in the same transaction: the
// ticket service records the event and knows nothing of end users.
func transition(ctx context.Context, q *db.Queries, svc ticket.Service, p Principal, id int64, state db.TicketState) error {
	st, err := q.FirstStatusInState(ctx, state)
	if err != nil {
		return err
	}
	if _, err := svc.SetStatus(ctx, ticket.SystemPrincipal, id, st.ID); err != nil {
		return err
	}
	return q.AnnotateLatestTicketEvent(ctx, db.AnnotateLatestTicketEventParams{UserID: p.UserID, TicketID: id})
}

// Download opens an attachment of one of the principal's tickets.
func (s *PortalService) Download(ctx context.Context, p Principal, ticketID, fileID int64) (*attachment.File, io.ReadCloser, error) {
	if _, err := own(ctx, db.New(s.b), p, ticketID); err != nil {
		return nil, nil, err
	}
	return s.files.DownloadForTicket(ctx, ticketID, fileID)
}

// Upload stores a portal upload with no staff uploader; it becomes the
// customer's when attached to their ticket on open or reply.
func (s *PortalService) Upload(ctx context.Context, name, mime string, r io.Reader) (*attachment.File, error) {
	return s.files.UploadAnonymous(ctx, name, mime, r)
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
