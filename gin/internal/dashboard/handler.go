package dashboard

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/httpx"
)

// StatsService is what the handler needs from the service.
type StatsService interface {
	Stats(ctx context.Context, p auth.Principal, start time.Time, period int) (*Stats, error)
}

type Handler struct{ svc StatsService }

func NewHandler(svc StatsService) *Handler { return &Handler{svc: svc} }

func (h *Handler) Mount(private *gin.RouterGroup) {
	private.GET("/dashboard/stats", h.stats)
}

func (h *Handler) stats(c *gin.Context) {
	p, ok := auth.FromContext(c)
	if !ok {
		httpx.Fail(c, apperr.ErrUnauthorized)
		return
	}
	period := DefaultPeriod
	if raw := c.Query("period"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			httpx.Fail(c, apperr.Validation("period", "must be a number of days"))
			return
		}
		period = n
	}
	start := time.Now().UTC().AddDate(0, 0, -(period - 1))
	if raw := c.Query("start"); raw != "" {
		t, err := time.Parse("2006-01-02", raw)
		if err != nil {
			httpx.Fail(c, apperr.Validation("start", "must be YYYY-MM-DD"))
			return
		}
		start = t
	}
	out, err := h.svc.Stats(c.Request.Context(), p, start, period)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}
