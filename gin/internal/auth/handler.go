package auth

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/httpx"
)

type SessionService interface {
	Login(ctx context.Context, username, password string) (*Session, error)
	Refresh(ctx context.Context, raw string) (*Session, error)
	Logout(ctx context.Context, staffID int64, raw string) error
	Me(ctx context.Context, staffID int64) (*StaffProfile, error)
}

type Handler struct {
	svc     SessionService
	limiter *RateLimiter
}

func NewHandler(svc SessionService) *Handler {
	return &Handler{svc: svc, limiter: newLoginRateLimiter()}
}

func (h *Handler) Mount(public, private *gin.RouterGroup) {
	public.POST("/auth/login", h.login)
	public.POST("/auth/refresh", h.refresh)
	private.POST("/auth/logout", h.logout)
	private.GET("/me", h.me)
}

type loginRequest struct {
	Username string `json:"username" binding:"required,max=64"`
	Password string `json:"password" binding:"required,max=256"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

func (h *Handler) login(c *gin.Context) {
	var in loginRequest
	if !httpx.BindJSON(c, &in) {
		return
	}
	key := in.Username + "|" + c.ClientIP()
	// allow reserves the attempt slot atomically with the check (see its
	// doc comment): a failed login leaves the reservation in place, and
	// reset below undoes it on success, so only failures count.
	if !h.limiter.Allow(key) {
		httpx.Fail(c, apperr.ErrRateLimited)
		return
	}
	sess, err := h.svc.Login(c.Request.Context(), in.Username, in.Password)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.limiter.Reset(key)
	c.JSON(http.StatusOK, sess)
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

func (h *Handler) logout(c *gin.Context) {
	p, ok := FromContext(c)
	if !ok {
		httpx.Fail(c, apperr.ErrUnauthorized)
		return
	}
	var in refreshRequest
	if !httpx.BindJSON(c, &in) {
		return
	}
	if err := h.svc.Logout(c.Request.Context(), p.StaffID, in.RefreshToken); err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) me(c *gin.Context) {
	p, ok := FromContext(c)
	if !ok {
		httpx.Fail(c, apperr.ErrUnauthorized)
		return
	}
	profile, err := h.svc.Me(c.Request.Context(), p.StaffID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, profile)
}
