package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/httpx"
	"github.com/jackc/pgx/v5"
)

type ReplyInput struct {
	Body     string  `json:"body" binding:"required"`
	Format   string  `json:"format" binding:"omitempty,oneof=html text"`
	StatusID *int64  `json:"status_id"`
	FileIDs  []int64 `json:"file_ids"`
}

type NoteInput struct {
	Title   string  `json:"title" binding:"max=255"`
	Body    string  `json:"body" binding:"required"`
	Format  string  `json:"format" binding:"omitempty,oneof=html text"`
	FileIDs []int64 `json:"file_ids"`
}

type StatusInput struct {
	StatusID int64 `json:"status_id" binding:"required"`
}

type AssignInput struct {
	StaffID *int64 `json:"staff_id"`
}

type TransferInput struct {
	DeptID int64 `json:"dept_id" binding:"required"`
}

type AttachmentRef struct {
	FileID int64  `json:"file_id"`
	Name   string `json:"name"`
	Mime   string `json:"mime"`
	Size   int64  `json:"size"`
}

type Entry struct {
	ID          int64           `json:"id"`
	TicketID    int64           `json:"ticket_id"`
	Type        string          `json:"type"`
	StaffID     *int64          `json:"staff_id"`
	Poster      string          `json:"poster"`
	Title       *string         `json:"title"`
	Body        string          `json:"body"`
	Format      string          `json:"format"`
	ParentID    *int64          `json:"parent_id"`
	Attachments []AttachmentRef `json:"attachments"`
	CreatedAt   time.Time       `json:"created_at"`
}

type Thread struct {
	Items     []Entry `json:"items"`
	NextAfter *int64  `json:"next_after"`
}

type Event struct {
	ID        int64           `json:"id"`
	TicketID  int64           `json:"ticket_id"`
	Staff     *Ref            `json:"staff"`
	Kind      string          `json:"kind"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
}

func (s *service) Reply(ctx context.Context, p auth.Principal, id int64, in ReplyInput) (*Entry, error) {
	var out *Entry
	err := db.WithTx(ctx, s.db, func(q *db.Queries) error {
		row, err := loadVisible(ctx, q, p, id)
		if err != nil {
			return err
		}
		st, err := q.GetStaff(ctx, p.StaffID)
		if err != nil {
			return err
		}
		entry, err := q.CreateThreadEntry(ctx, db.CreateThreadEntryParams{
			TicketID: id, Type: db.ThreadEntryTypeResponse, StaffID: &p.StaffID,
			Poster: fullName(&st.FirstName, &st.LastName), Body: in.Body, Format: bodyFormat(in.Format),
		})
		if err != nil {
			return err
		}
		if err := attachFiles(ctx, q, entry.ID, in.FileIDs); err != nil {
			return err
		}
		if err := q.MarkTicketAnswered(ctx, id); err != nil {
			return err
		}
		if in.StatusID != nil {
			if err := applyStatus(ctx, q, p, row, *in.StatusID); err != nil {
				return err
			}
		}
		out, err = entryWithAttachments(ctx, q, entry)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *service) Note(ctx context.Context, p auth.Principal, id int64, in NoteInput) (*Entry, error) {
	var out *Entry
	err := db.WithTx(ctx, s.db, func(q *db.Queries) error {
		if _, err := loadVisible(ctx, q, p, id); err != nil {
			return err
		}
		st, err := q.GetStaff(ctx, p.StaffID)
		if err != nil {
			return err
		}
		var title *string
		if in.Title != "" {
			title = &in.Title
		}
		entry, err := q.CreateThreadEntry(ctx, db.CreateThreadEntryParams{
			TicketID: id, Type: db.ThreadEntryTypeNote, StaffID: &p.StaffID, Title: title,
			Poster: fullName(&st.FirstName, &st.LastName), Body: in.Body, Format: bodyFormat(in.Format),
		})
		if err != nil {
			return err
		}
		if err := attachFiles(ctx, q, entry.ID, in.FileIDs); err != nil {
			return err
		}
		out, err = entryWithAttachments(ctx, q, entry)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *service) Thread(ctx context.Context, p auth.Principal, id int64, after int64, limit int) (*Thread, error) {
	q := db.New(s.db)
	if _, err := loadVisible(ctx, q, p, id); err != nil {
		return nil, err
	}
	rows, err := q.ListThreadEntries(ctx, db.ListThreadEntriesParams{TicketID: id, After: after, PageLimit: int32(limit + 1)})
	if err != nil {
		return nil, err
	}
	var next *int64
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1].ID
		next = &last
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	atts, err := q.ListAttachmentsForEntries(ctx, ids)
	if err != nil {
		return nil, err
	}
	byEntry := map[int64][]AttachmentRef{}
	for _, a := range atts {
		byEntry[a.ThreadEntryID] = append(byEntry[a.ThreadEntryID], AttachmentRef{FileID: a.FileID, Name: a.Name, Mime: a.Mime, Size: a.Size})
	}
	items := make([]Entry, 0, len(rows))
	for _, r := range rows {
		e := toEntry(r)
		if list := byEntry[r.ID]; list != nil {
			e.Attachments = list
		}
		items = append(items, e)
	}
	return &Thread{Items: items, NextAfter: next}, nil
}

func (s *service) SetStatus(ctx context.Context, p auth.Principal, id int64, statusID int64) (*Ticket, error) {
	return s.mutate(ctx, p, id, func(q *db.Queries, row db.GetTicketRow) error {
		return applyStatus(ctx, q, p, row, statusID)
	})
}

func (s *service) Assign(ctx context.Context, p auth.Principal, id int64, staffID *int64) (*Ticket, error) {
	return s.mutate(ctx, p, id, func(q *db.Queries, row db.GetTicketRow) error {
		if staffID == nil {
			if err := q.SetTicketAssignee(ctx, db.SetTicketAssigneeParams{ID: id, AssignedStaffID: nil}); err != nil {
				return err
			}
			return event(ctx, q, id, &p.StaffID, db.TicketEventKindUnassigned, nil)
		}
		st, err := q.GetStaff(ctx, *staffID)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Validation("staff_id", "unknown staff")
		}
		if err != nil {
			return err
		}
		if !st.IsActive {
			return apperr.Validation("staff_id", "staff is inactive")
		}
		can, err := q.StaffCanSeeDept(ctx, db.StaffCanSeeDeptParams{StaffID: *staffID, DeptID: row.DeptID})
		if err != nil {
			return err
		}
		if !can {
			return apperr.Validation("staff_id", "staff cannot see the ticket's department")
		}
		if err := q.SetTicketAssignee(ctx, db.SetTicketAssigneeParams{ID: id, AssignedStaffID: staffID}); err != nil {
			return err
		}
		return event(ctx, q, id, &p.StaffID, db.TicketEventKindAssigned, map[string]any{"staff_id": *staffID})
	})
}

func (s *service) Transfer(ctx context.Context, p auth.Principal, id int64, deptID int64) (*Ticket, error) {
	return s.mutate(ctx, p, id, func(q *db.Queries, row db.GetTicketRow) error {
		if _, err := q.GetDepartment(ctx, deptID); errors.Is(err, pgx.ErrNoRows) {
			return apperr.Validation("dept_id", "unknown department")
		} else if err != nil {
			return err
		}
		if !p.CanSeeDept(deptID) {
			return fmt.Errorf("%w: cannot transfer to that department", apperr.ErrForbidden)
		}
		if deptID == row.DeptID {
			return nil
		}
		if err := q.SetTicketDept(ctx, db.SetTicketDeptParams{ID: id, DeptID: deptID}); err != nil {
			return err
		}
		if row.AssignedStaffID != nil {
			can, err := q.StaffCanSeeDept(ctx, db.StaffCanSeeDeptParams{StaffID: *row.AssignedStaffID, DeptID: deptID})
			if err != nil {
				return err
			}
			if !can {
				if err := q.SetTicketAssignee(ctx, db.SetTicketAssigneeParams{ID: id, AssignedStaffID: nil}); err != nil {
					return err
				}
				if err := event(ctx, q, id, &p.StaffID, db.TicketEventKindUnassigned, map[string]any{"reason": "transfer"}); err != nil {
					return err
				}
			}
		}
		return event(ctx, q, id, &p.StaffID, db.TicketEventKindTransferred, map[string]any{"from": row.DeptID, "to": deptID})
	})
}

func (s *service) Events(ctx context.Context, p auth.Principal, id int64) ([]Event, error) {
	q := db.New(s.db)
	if _, err := loadVisible(ctx, q, p, id); err != nil {
		return nil, err
	}
	rows, err := q.ListTicketEvents(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(rows))
	for _, r := range rows {
		e := Event{ID: r.ID, TicketID: r.TicketID, Kind: string(r.Kind), Data: json.RawMessage(r.Data), CreatedAt: r.CreatedAt.UTC()}
		if r.StaffID != nil {
			e.Staff = &Ref{ID: *r.StaffID, Name: fullName(r.StaffFirstName, r.StaffLastName)}
		}
		out = append(out, e)
	}
	return out, nil
}

// mutate loads the visible ticket, runs fn in a transaction, and returns the fresh ticket.
func (s *service) mutate(ctx context.Context, p auth.Principal, id int64, fn func(q *db.Queries, row db.GetTicketRow) error) (*Ticket, error) {
	var out *Ticket
	err := db.WithTx(ctx, s.db, func(q *db.Queries) error {
		row, err := loadVisible(ctx, q, p, id)
		if err != nil {
			return err
		}
		if err := fn(q, row); err != nil {
			return err
		}
		out, err = get(ctx, q, p, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func applyStatus(ctx context.Context, q *db.Queries, p auth.Principal, row db.GetTicketRow, statusID int64) error {
	if statusID == row.StatusID {
		return nil
	}
	ns, err := q.GetStatus(ctx, statusID)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.Validation("status_id", "unknown status")
	}
	if err != nil {
		return err
	}
	closedAt := row.ClosedAt
	kind := db.TicketEventKindStatusChanged
	switch {
	case row.StatusState == db.TicketStateOpen && ns.State != db.TicketStateOpen:
		now := time.Now().UTC()
		closedAt = &now
		kind = db.TicketEventKindClosed
	case row.StatusState != db.TicketStateOpen && ns.State == db.TicketStateOpen:
		closedAt = nil
		kind = db.TicketEventKindReopened
	}
	if err := q.SetTicketStatus(ctx, db.SetTicketStatusParams{ID: row.ID, StatusID: statusID, ClosedAt: closedAt}); err != nil {
		return err
	}
	return event(ctx, q, row.ID, &p.StaffID, kind, map[string]any{"from": row.StatusID, "to": statusID})
}

func attachFiles(ctx context.Context, q *db.Queries, entryID int64, fileIDs []int64) error {
	for _, fid := range fileIDs {
		if _, err := q.GetFile(ctx, fid); errors.Is(err, pgx.ErrNoRows) {
			return apperr.Validation("file_ids", "unknown file "+strconv.FormatInt(fid, 10))
		} else if err != nil {
			return err
		}
		attached, err := q.IsFileAttached(ctx, fid)
		if err != nil {
			return err
		}
		if attached {
			return fmt.Errorf("%w: file %d is already attached", apperr.ErrConflict, fid)
		}
		if err := q.CreateAttachment(ctx, db.CreateAttachmentParams{ThreadEntryID: entryID, FileID: fid}); err != nil {
			return err
		}
	}
	return nil
}

func entryWithAttachments(ctx context.Context, q *db.Queries, r db.ThreadEntry) (*Entry, error) {
	e := toEntry(r)
	atts, err := q.ListAttachmentsForEntries(ctx, []int64{r.ID})
	if err != nil {
		return nil, err
	}
	for _, a := range atts {
		e.Attachments = append(e.Attachments, AttachmentRef{FileID: a.FileID, Name: a.Name, Mime: a.Mime, Size: a.Size})
	}
	return &e, nil
}

func toEntry(r db.ThreadEntry) Entry {
	return Entry{
		ID: r.ID, TicketID: r.TicketID, Type: string(r.Type), StaffID: r.StaffID, Poster: r.Poster,
		Title: r.Title, Body: r.Body, Format: string(r.Format), ParentID: r.ParentID,
		Attachments: []AttachmentRef{}, CreatedAt: r.CreatedAt.UTC(),
	}
}

// Handlers

func (h *Handler) mountThread(private *gin.RouterGroup) {
	private.POST("/tickets/:id/reply", h.reply)
	private.POST("/tickets/:id/notes", h.note)
	private.GET("/tickets/:id/thread", h.thread)
	private.POST("/tickets/:id/status", h.setStatus)
	private.POST("/tickets/:id/assign", h.assign)
	private.POST("/tickets/:id/transfer", h.transfer)
	private.GET("/tickets/:id/events", h.events)
}

func (h *Handler) reply(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in ReplyInput
	if !httpx.BindJSON(c, &in) {
		return
	}
	out, err := h.svc.Reply(c.Request.Context(), p, id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}

func (h *Handler) note(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in NoteInput
	if !httpx.BindJSON(c, &in) {
		return
	}
	out, err := h.svc.Note(c.Request.Context(), p, id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}

func (h *Handler) thread(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	after, limit := int64(0), 50
	if v := c.Query("after"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			httpx.Fail(c, apperr.Validation("after", "must be a non-negative integer"))
			return
		}
		after = n
	}
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			httpx.Fail(c, apperr.Validation("limit", "must be between 1 and 200"))
			return
		}
		limit = n
	}
	out, err := h.svc.Thread(c.Request.Context(), p, id, after, limit)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) setStatus(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in StatusInput
	if !httpx.BindJSON(c, &in) {
		return
	}
	out, err := h.svc.SetStatus(c.Request.Context(), p, id, in.StatusID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) assign(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in AssignInput
	if !httpx.BindJSON(c, &in) {
		return
	}
	out, err := h.svc.Assign(c.Request.Context(), p, id, in.StaffID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) transfer(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in TransferInput
	if !httpx.BindJSON(c, &in) {
		return
	}
	out, err := h.svc.Transfer(c.Request.Context(), p, id, in.DeptID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) events(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Events(c.Request.Context(), p, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}
