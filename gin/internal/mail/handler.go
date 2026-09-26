package mail

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/httpx"
	"github.com/jackc/pgx/v5"
)

// Handler exposes the admin mail API.
type Handler struct {
	db db.Beginner
	r  *Renderer
}

func NewHandler(b db.Beginner, r *Renderer) *Handler { return &Handler{db: b, r: r} }

func (h *Handler) Mount(private *gin.RouterGroup) {
	admin := private.Group("", auth.RequireAdmin())
	admin.GET("/email/templates", h.listTemplates)
	admin.PATCH("/email/templates/:key", h.patchTemplate)
	admin.GET("/email/outbox", h.listOutbox)
	admin.GET("/email/outbox/:id", h.getOutbox)
	admin.POST("/email/outbox/:id/retry", h.retry)
	admin.GET("/email/inbound", h.listInbound)
}

type templateJSON struct {
	Key       string `json:"key"`
	Subject   string `json:"subject"`
	BodyHTML  string `json:"body_html"`
	BodyText  string `json:"body_text"`
	UpdatedAt string `json:"updated_at"`
}

func toTemplateJSON(t db.EmailTemplate) templateJSON {
	return templateJSON{Key: t.Key, Subject: t.Subject, BodyHTML: t.BodyHtml, BodyText: t.BodyText, UpdatedAt: t.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")}
}

func (h *Handler) listTemplates(c *gin.Context) {
	rows, err := db.New(h.db).ListEmailTemplates(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out := make([]templateJSON, 0, len(rows))
	for _, r := range rows {
		out = append(out, toTemplateJSON(r))
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

type templatePatch struct {
	Subject  *string `json:"subject"`
	BodyHTML *string `json:"body_html"`
	BodyText *string `json:"body_text"`
}

func (h *Handler) patchTemplate(c *gin.Context) {
	key := c.Param("key")
	var in templatePatch
	if !httpx.BindJSON(c, &in) {
		return
	}
	if in.Subject == nil && in.BodyHTML == nil && in.BodyText == nil {
		httpx.Fail(c, apperr.Validation("body", "supply subject, body_html or body_text"))
		return
	}
	ctx := c.Request.Context()
	q := db.New(h.db)
	cur, err := q.GetEmailTemplate(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(c, apperr.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	subject, bodyHTML, bodyText := cur.Subject, cur.BodyHtml, cur.BodyText
	if in.Subject != nil {
		subject = *in.Subject
	}
	if in.BodyHTML != nil {
		bodyHTML = *in.BodyHTML
	}
	if in.BodyText != nil {
		bodyText = *in.BodyText
	}
	if err := ValidateTemplate(subject, bodyHTML, bodyText); err != nil {
		part, msg := splitTemplateError(err)
		httpx.Fail(c, apperr.Validation(part, msg))
		return
	}
	row, err := q.UpdateEmailTemplate(ctx, db.UpdateEmailTemplateParams{Key: key, Subject: in.Subject, BodyHtml: in.BodyHTML, BodyText: in.BodyText})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.r.Invalidate(key)
	c.JSON(http.StatusOK, toTemplateJSON(row))
}

// splitTemplateError turns "body_html: template: ..." into ("body_html", "template: ...").
func splitTemplateError(err error) (string, string) {
	s := err.Error()
	for _, part := range []string{"subject", "body_html", "body_text"} {
		if len(s) > len(part)+2 && s[:len(part)+2] == part+": " {
			return part, s[len(part)+2:]
		}
	}
	return "body", s
}

type outboxJSON struct {
	ID          int64   `json:"id"`
	TicketID    *int64  `json:"ticket_id"`
	EntryID     *int64  `json:"entry_id"`
	TemplateKey string  `json:"template_key"`
	ToAddress   string  `json:"to_address"`
	ToName      string  `json:"to_name"`
	Subject     string  `json:"subject"`
	Status      string  `json:"status"`
	Attempts    int32   `json:"attempts"`
	LastError   *string `json:"last_error"`
	NextAttempt string  `json:"next_attempt_at"`
	SentAt      *string `json:"sent_at"`
	CreatedAt   string  `json:"created_at"`
}

func (h *Handler) listOutbox(c *gin.Context) {
	page, err := httpx.ParsePage(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var status *db.EmailStatus
	if s := c.Query("status"); s != "" {
		switch db.EmailStatus(s) {
		case db.EmailStatusPending, db.EmailStatusSent, db.EmailStatusFailed:
			v := db.EmailStatus(s)
			status = &v
		default:
			httpx.Fail(c, apperr.Validation("status", "must be pending, sent or failed"))
			return
		}
	}
	ctx := c.Request.Context()
	q := db.New(h.db)
	rows, err := q.ListOutbox(ctx, db.ListOutboxParams{Status: status, Lim: page.Limit(), Off: page.Offset()})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	total, err := q.CountOutbox(ctx, status)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	items := make([]outboxJSON, 0, len(rows))
	for _, r := range rows {
		items = append(items, toOutboxJSON(r))
	}
	c.JSON(http.StatusOK, httpx.List[outboxJSON]{Items: items, Page: page.Page, PageSize: page.PageSize, Total: total})
}

func toOutboxJSON(r db.ListOutboxRow) outboxJSON {
	j := outboxJSON{ID: r.ID, TicketID: r.TicketID, EntryID: r.EntryID, TemplateKey: r.TemplateKey, ToAddress: r.ToAddress, ToName: r.ToName, Subject: r.Subject, Status: string(r.Status), Attempts: r.Attempts, LastError: r.LastError, NextAttempt: r.NextAttemptAt.UTC().Format("2006-01-02T15:04:05Z07:00"), CreatedAt: r.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")}
	if r.SentAt != nil {
		s := r.SentAt.UTC().Format("2006-01-02T15:04:05Z07:00")
		j.SentAt = &s
	}
	return j
}

// outboxDetailJSON is one outbox row with its rendered bodies, for admins
// inspecting a queued message.
type outboxDetailJSON struct {
	outboxJSON
	BodyText string `json:"body_text"`
	BodyHTML string `json:"body_html"`
}

func (h *Handler) getOutbox(c *gin.Context) {
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	row, err := db.New(h.db).GetOutbox(c.Request.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(c, apperr.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, outboxDetailJSON{outboxJSON: toOutboxJSON(db.ListOutboxRow{ID: row.ID, TicketID: row.TicketID, EntryID: row.EntryID, TemplateKey: row.TemplateKey, ToAddress: row.ToAddress, ToName: row.ToName, Subject: row.Subject, Status: row.Status, Attempts: row.Attempts, LastError: row.LastError, NextAttemptAt: row.NextAttemptAt, SentAt: row.SentAt, CreatedAt: row.CreatedAt}), BodyText: row.BodyText, BodyHTML: row.BodyHtml})
}

func (h *Handler) retry(c *gin.Context) {
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	q := db.New(h.db)
	row, err := q.GetOutbox(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(c, apperr.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if row.Status == db.EmailStatusSent {
		httpx.Fail(c, apperr.ErrConflict)
		return
	}
	if _, err := q.RetryOutbox(ctx, id); err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "status": "pending"})
}

func (h *Handler) listInbound(c *gin.Context) {
	page, err := httpx.ParsePage(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	q := db.New(h.db)
	rows, err := q.ListInbound(ctx, db.ListInboundParams{Limit: page.Limit(), Offset: page.Offset()})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	total, err := q.CountInbound(ctx)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	type inboundJSON struct {
		ID          int64  `json:"id"`
		MessageID   string `json:"message_id"`
		FromAddress string `json:"from_address"`
		FromName    string `json:"from_name"`
		Subject     string `json:"subject"`
		TicketID    *int64 `json:"ticket_id"`
		EntryID     *int64 `json:"entry_id"`
		Outcome     string `json:"outcome"`
		Reason      string `json:"reason"`
		ReceivedAt  string `json:"received_at"`
	}
	items := make([]inboundJSON, 0, len(rows))
	for _, r := range rows {
		items = append(items, inboundJSON{ID: r.ID, MessageID: r.MessageID, FromAddress: r.FromAddress, FromName: r.FromName, Subject: r.Subject, TicketID: r.TicketID, EntryID: r.EntryID, Outcome: string(r.Outcome), Reason: r.Reason, ReceivedAt: r.ReceivedAt.UTC().Format("2006-01-02T15:04:05Z07:00")})
	}
	c.JSON(http.StatusOK, httpx.List[inboundJSON]{Items: items, Page: page.Page, PageSize: page.PageSize, Total: total})
}
