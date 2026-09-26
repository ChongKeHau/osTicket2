package client

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/apperr"
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
	SetPassword(ctx context.Context, p Principal, in PasswordInput) error
}

// Handler serves the portal auth and profile routes.
type Handler struct {
	svc     IdentityService
	limiter *Limiter
}

// NewHandler builds the handler; limiter budgets login, link, reset and access
// requests per address and per client IP.
func NewHandler(svc IdentityService, limiter *Limiter) *Handler {
	return &Handler{svc: svc, limiter: limiter}
}

// MountAuth registers the portal auth routes on public (the unauthenticated
// portal group) and builds the signed-in groups itself from tokens:
//   - public: POST auth/login, auth/link, auth/reset, access, auth/exchange,
//     auth/register, auth/refresh
//   - RequireUser: POST auth/logout; RequireUser + RequireAccount: PATCH me
//   - RequireUser(AllowPasswordReset) + RequireAccount: GET me, POST me/password
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

	password := public.Group("", RequireUser(tokens, h.svc, AllowPasswordReset()), RequireAccount())
	password.GET("/me", h.me)
	password.POST("/me/password", h.setPassword)
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
	c.JSON(http.StatusOK, profile)
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
	if err := h.svc.SetPassword(c.Request.Context(), p, in); err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
