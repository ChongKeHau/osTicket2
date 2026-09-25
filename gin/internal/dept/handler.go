package dept

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/httpx"
)

type Handler struct{ svc Service }

func NewHandler(svc Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Mount(private *gin.RouterGroup) {
	private.GET("/departments", h.list)
	private.GET("/departments/:id", h.get)
	admin := private.Group("", auth.RequireAdmin())
	admin.POST("/departments", h.create)
	admin.PATCH("/departments/:id", h.update)
	admin.DELETE("/departments/:id", h.delete)
}

func (h *Handler) list(c *gin.Context) {
	out, err := h.svc.List(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func (h *Handler) get(c *gin.Context) {
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) create(c *gin.Context) {
	var in CreateInput
	if !httpx.BindJSON(c, &in) {
		return
	}
	out, err := h.svc.Create(c.Request.Context(), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}

func (h *Handler) update(c *gin.Context) {
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	raw, ok := httpx.ReadRawJSON(c)
	if !ok {
		return
	}
	var in UpdateInput
	if !httpx.BindJSON(c, &in) {
		return
	}
	// manager_id: an explicit JSON null clears the manager; an absent key
	// leaves it unchanged (BindJSON alone can't tell the two apart, since
	// both decode to a nil pointer).
	var keys map[string]json.RawMessage
	_ = json.Unmarshal(raw, &keys)
	if v, present := keys["manager_id"]; present && string(v) == "null" {
		in.ClearManager = true
	}
	out, err := h.svc.Update(c.Request.Context(), id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) delete(c *gin.Context) {
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
