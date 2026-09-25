package ticket

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/httpx"
)

type Handler struct{ svc Service }

func NewHandler(svc Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Mount(private *gin.RouterGroup) {
	private.GET("/priorities", h.priorities)
	private.GET("/statuses", h.statuses)
	private.GET("/tickets", h.list)
	private.POST("/tickets", h.create)
	private.GET("/tickets/:id", h.get)
	private.PATCH("/tickets/:id", h.update)
	h.mountThread(private) // Task 10
}

func principal(c *gin.Context) (auth.Principal, bool) {
	p, ok := auth.FromContext(c)
	if !ok {
		httpx.Fail(c, apperr.ErrUnauthorized)
	}
	return p, ok
}

func optionalID(c *gin.Context, name string) (*int64, error) {
	v := c.Query(name)
	if v == "" {
		return nil, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return nil, apperr.Validation(name, "must be a positive integer")
	}
	return &n, nil
}

func (h *Handler) priorities(c *gin.Context) {
	out, err := h.svc.ListPriorities(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func (h *Handler) statuses(c *gin.Context) {
	out, err := h.svc.ListStatuses(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func (h *Handler) list(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	page, err := httpx.ParsePage(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	statusID, err := optionalID(c, "status")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	deptID, err := optionalID(c, "dept_id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.List(c.Request.Context(), p, ListFilter{
		StatusID: statusID, State: c.Query("state"), DeptID: deptID, AssignedTo: c.Query("assigned_to"),
		Q: c.Query("q"), Sort: c.Query("sort"), Page: page,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) create(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	var in CreateInput
	if !httpx.BindJSON(c, &in) {
		return
	}
	out, err := h.svc.Create(c.Request.Context(), p, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}

func (h *Handler) get(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Get(c.Request.Context(), p, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) update(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	raw, ok := readRawJSON(c)
	if !ok {
		return
	}
	var in UpdateInput
	if !httpx.BindJSON(c, &in) {
		return
	}
	// due_at/topic_id: an explicit JSON null clears the field; an absent key
	// leaves it unchanged (BindJSON alone can't tell the two apart, since
	// both decode to a nil pointer). extra: an explicit null means
	// "unchanged" too (not a validation error), so it's reset to nil here in
	// case some other zero-length-but-non-nil value slipped through binding.
	var keys map[string]json.RawMessage
	_ = json.Unmarshal(raw, &keys)
	if v, present := keys["due_at"]; present && string(v) == "null" {
		in.ClearDueAt = true
	}
	if v, present := keys["topic_id"]; present && string(v) == "null" {
		in.ClearTopic = true
	}
	if v, present := keys["extra"]; present && string(v) == "null" {
		in.Extra = nil
	}
	out, err := h.svc.Update(c.Request.Context(), p, id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// readRawJSON reads the whole request body and restores it onto the request
// so a subsequent httpx.BindJSON can still bind it. It's used by handlers
// that need to distinguish an explicit JSON null from an absent key, which a
// bound struct alone can't tell apart (both decode to a nil pointer). On
// read failure it writes the error response and returns ok=false.
func readRawJSON(c *gin.Context) (json.RawMessage, bool) {
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		var mbErr *http.MaxBytesError
		if errors.As(err, &mbErr) {
			httpx.Fail(c, apperr.ErrPayloadTooLarge)
		} else {
			httpx.Fail(c, apperr.Validation("body", "malformed json"))
		}
		return nil, false
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	return raw, true
}
