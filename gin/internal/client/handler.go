package client

import (
	"context"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/attachment"
	"github.com/grandpine/ticket-api/internal/httpx"
)

// IdentityService is what the portal auth handler needs; *Service implements it.
type IdentityService interface {
	UserLoader
	// Register, RequestLink, RequestReset and RequestAccess only schedule
	// their work and report nothing, so the response never depends on
	// whether the address or ticket exists.
	Register(in RegisterInput)
	Login(ctx context.Context, email, password string) (*Session, error)
	RequestLink(email string)
	RequestReset(email string)
	RequestAccess(email, number string)
	Exchange(ctx context.Context, raw string) (*Session, error)
	Refresh(ctx context.Context, raw string) (*Session, error)
	Logout(ctx context.Context, userID int64, raw string) error
	Me(ctx context.Context, userID int64) (*Profile, error)
	UpdateName(ctx context.Context, userID int64, name string) (*Profile, error)
	SetPassword(ctx context.Context, p Principal, in PasswordInput) (*Session, error)
}

// Portal is what the portal ticket routes need; *PortalService implements it.
type Portal interface {
	Reference(ctx context.Context) (*Reference, error)
	OpenTicket(ctx context.Context, p *Principal, ip string, in OpenInput) (*Opened, error)
	ListTickets(ctx context.Context, p Principal, state string, page httpx.Page) (*httpx.List[TicketRow], error)
	GetTicket(ctx context.Context, p Principal, id int64) (*TicketView, error)
	Reply(ctx context.Context, p Principal, id int64, in ReplyInput) error
	Close(ctx context.Context, p Principal, id int64) (*TicketView, error)
	Reopen(ctx context.Context, p Principal, id int64) (*TicketView, error)
	Download(ctx context.Context, p Principal, ticketID, fileID int64) (*attachment.File, io.ReadCloser, error)
	Upload(ctx context.Context, name, mime string, r io.Reader) (*attachment.File, error)
}

// Handler serves the portal auth, profile and ticket routes.
type Handler struct {
	svc         IdentityService
	limiter     *Limiter
	portal      Portal
	uploadLimit *Limiter
	maxBytes    int64
}

// NewHandler builds the handler. limiter budgets login, link, reset, access
// and register requests per address and per client IP; portal serves the
// ticket routes; uploadLimit budgets anonymous uploads per client IP (only
// its IP side is used); maxBytes caps one upload, as for staff uploads.
func NewHandler(svc IdentityService, limiter *Limiter, portal Portal, uploadLimit *Limiter, maxBytes int64) *Handler {
	return &Handler{svc: svc, limiter: limiter, portal: portal, uploadLimit: uploadLimit, maxBytes: maxBytes}
}

// MountAuth registers the portal auth routes on public (the unauthenticated
// portal group) and builds the signed-in groups itself from tokens:
//   - public: POST auth/login, auth/link, auth/reset, access, auth/exchange,
//     auth/register, auth/refresh
//   - RequireUser: POST auth/logout; RequireUser + RequireAccount: PATCH me
//   - RequireUser(AllowPasswordReset): GET me (guests too, so a reload restores them)
//   - RequireUser(AllowPasswordReset) + RequireAccount: POST me/password
func (h *Handler) MountAuth(public *gin.RouterGroup, tokens *Tokens) {
	public.POST("/auth/login", h.login)
	public.POST("/auth/link", h.requestLink)
	public.POST("/auth/reset", h.requestReset)
	public.POST("/access", h.requestAccess)
	public.POST("/auth/exchange", h.exchange)
	public.POST("/auth/register", h.register)
	public.POST("/auth/refresh", h.refresh)

	user := public.Group("", RequireUser(tokens, h.svc))
	user.POST("/auth/logout", h.logout)
	user.PATCH("/me", RequireAccount(), h.updateMe)

	withReset := public.Group("", RequireUser(tokens, h.svc, AllowPasswordReset()))
	withReset.GET("/me", h.me)
	withReset.POST("/me/password", RequireAccount(), h.setPassword)
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,max=255"`
	Password string `json:"password" binding:"required,max=256"`
}

type emailRequest struct {
	Email string `json:"email" binding:"required,email,max=255"`
}

type accessRequest struct {
	Email  string `json:"email" binding:"required,email,max=255"`
	Number string `json:"number" binding:"required,max=32"`
}

type exchangeRequest struct {
	Token string `json:"token" binding:"required,max=128"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type nameRequest struct {
	Name string `json:"name" binding:"required,max=128"`
}

func (h *Handler) login(c *gin.Context) {
	var in loginRequest
	if !httpx.BindJSON(c, &in) {
		return
	}
	if err := h.limiter.Check(in.Email, c.ClientIP()); err != nil {
		httpx.Fail(c, err)
		return
	}
	sess, err := h.svc.Login(c.Request.Context(), in.Email, in.Password)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, sess)
}

// accepted applies the limiter, hands the work to the service (which runs it
// after the response) and answers status with an empty object, identical
// whether or not the address or ticket exists.
func (h *Handler) accepted(c *gin.Context, status int, email string, schedule func()) {
	if err := h.limiter.Check(email, c.ClientIP()); err != nil {
		httpx.Fail(c, err)
		return
	}
	schedule()
	c.JSON(status, gin.H{})
}

func (h *Handler) requestLink(c *gin.Context) {
	var in emailRequest
	if !httpx.BindJSON(c, &in) {
		return
	}
	h.accepted(c, http.StatusAccepted, in.Email, func() { h.svc.RequestLink(in.Email) })
}

func (h *Handler) requestReset(c *gin.Context) {
	var in emailRequest
	if !httpx.BindJSON(c, &in) {
		return
	}
	h.accepted(c, http.StatusAccepted, in.Email, func() { h.svc.RequestReset(in.Email) })
}

func (h *Handler) requestAccess(c *gin.Context) {
	var in accessRequest
	if !httpx.BindJSON(c, &in) {
		return
	}
	h.accepted(c, http.StatusAccepted, in.Email, func() { h.svc.RequestAccess(in.Email, in.Number) })
}

func (h *Handler) exchange(c *gin.Context) {
	var in exchangeRequest
	if !httpx.BindJSON(c, &in) {
		return
	}
	sess, err := h.svc.Exchange(c.Request.Context(), in.Token)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, sess)
}

func (h *Handler) register(c *gin.Context) {
	var in RegisterInput
	if !httpx.BindJSON(c, &in) {
		return
	}
	h.accepted(c, http.StatusCreated, in.Email, func() { h.svc.Register(in) })
}

func (h *Handler) refresh(c *gin.Context) {
	var in refreshRequest
	if !httpx.BindJSON(c, &in) {
		return
	}
	sess, err := h.svc.Refresh(c.Request.Context(), in.RefreshToken)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, sess)
}

func principal(c *gin.Context) (Principal, bool) {
	p, ok := FromContext(c)
	if !ok {
		httpx.Fail(c, apperr.ErrUnauthorized)
	}
	return p, ok
}

func (h *Handler) logout(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	var in refreshRequest
	if !httpx.BindJSON(c, &in) {
		return
	}
	if err := h.svc.Logout(c.Request.Context(), p.UserID, in.RefreshToken); err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// meResponse is the profile plus the ticket a guest session is scoped to
// (null for an account session).
type meResponse struct {
	*Profile
	TicketID *int64 `json:"ticket_id"`
}

func (h *Handler) me(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	profile, err := h.svc.Me(c.Request.Context(), p.UserID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, meResponse{Profile: profile, TicketID: p.TicketID})
}

func (h *Handler) updateMe(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	var in nameRequest
	if !httpx.BindJSON(c, &in) {
		return
	}
	profile, err := h.svc.UpdateName(c.Request.Context(), p.UserID, in.Name)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, profile)
}

func (h *Handler) setPassword(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	var in PasswordInput
	if !httpx.BindJSON(c, &in) {
		return
	}
	sess, err := h.svc.SetPassword(c.Request.Context(), p, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, sess)
}

// MountPortal registers the portal ticket routes on public (the
// unauthenticated portal group) and builds the signed-in groups from tokens:
//   - public: GET reference; OptionalUser: POST tickets, POST files
//   - RequireUser (guests limited to their ticket): GET tickets/:id,
//     POST tickets/:id/reply, GET tickets/:id/files/:fileId
//   - RequireUser + RequireAccount: GET tickets
//
// Close and reopen admit guest sessions (spec §4: signed in, guests limited
// to their ticket); the service's ownership check scopes a guest to its ticket.
func (h *Handler) MountPortal(public *gin.RouterGroup, tokens *Tokens) {
	public.GET("/reference", h.reference)
	optional := public.Group("", OptionalUser(tokens, h.svc))
	optional.POST("/tickets", h.openTicket)
	optional.POST("/files", h.upload)

	user := public.Group("", RequireUser(tokens, h.svc))
	user.GET("/tickets/:id", h.getTicket)
	user.POST("/tickets/:id/reply", h.reply)
	user.GET("/tickets/:id/files/:fileId", h.download)
	user.POST("/tickets/:id/close", h.closeTicket)
	user.POST("/tickets/:id/reopen", h.reopenTicket)

	account := user.Group("", RequireAccount())
	account.GET("/tickets", h.listTickets)
}

func (h *Handler) reference(c *gin.Context) {
	out, err := h.portal.Reference(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) openTicket(c *gin.Context) {
	var in OpenInput
	if !httpx.BindJSON(c, &in) {
		return
	}
	var pp *Principal
	if p, ok := FromContext(c); ok {
		pp = &p
	}
	out, err := h.portal.OpenTicket(c.Request.Context(), pp, c.ClientIP(), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}

// upload takes the same multipart form as the staff upload. Anonymous callers
// are budgeted per IP before the body is read.
func (h *Handler) upload(c *gin.Context) {
	if _, ok := FromContext(c); !ok {
		if err := h.uploadLimit.CheckIP(c.ClientIP()); err != nil {
			httpx.Fail(c, err)
			return
		}
	}
	src, fh, ok := attachment.FormFile(c, h.maxBytes)
	if !ok {
		return
	}
	defer src.Close()
	out, err := h.portal.Upload(c.Request.Context(), fh.Filename, fh.Header.Get("Content-Type"), src)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}

// ticketParam reads the principal and the :id param, failing the request when either is missing.
func ticketParam(c *gin.Context) (Principal, int64, bool) {
	p, ok := principal(c)
	if !ok {
		return Principal{}, 0, false
	}
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return Principal{}, 0, false
	}
	return p, id, true
}

func (h *Handler) listTickets(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	page, err := httpx.ParsePage(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.portal.ListTickets(c.Request.Context(), p, c.Query("state"), page)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) getTicket(c *gin.Context) {
	p, id, ok := ticketParam(c)
	if !ok {
		return
	}
	out, err := h.portal.GetTicket(c.Request.Context(), p, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) reply(c *gin.Context) {
	p, id, ok := ticketParam(c)
	if !ok {
		return
	}
	var in ReplyInput
	if !httpx.BindJSON(c, &in) {
		return
	}
	if err := h.portal.Reply(c.Request.Context(), p, id, in); err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) closeTicket(c *gin.Context) {
	p, id, ok := ticketParam(c)
	if !ok {
		return
	}
	out, err := h.portal.Close(c.Request.Context(), p, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) reopenTicket(c *gin.Context) {
	p, id, ok := ticketParam(c)
	if !ok {
		return
	}
	out, err := h.portal.Reopen(c.Request.Context(), p, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) download(c *gin.Context) {
	p, id, ok := ticketParam(c)
	if !ok {
		return
	}
	fileID, err := httpx.ParseID(c, "fileId")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	meta, rc, err := h.portal.Download(c.Request.Context(), p, id, fileID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	attachment.ServeFile(c, meta, rc)
}
