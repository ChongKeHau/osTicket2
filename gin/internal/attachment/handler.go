package attachment

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/httpx"
)

type Handler struct {
	svc      Service
	maxBytes int64
}

func NewHandler(svc Service, maxBytes int64) *Handler { return &Handler{svc: svc, maxBytes: maxBytes} }

func (h *Handler) Mount(private *gin.RouterGroup) {
	private.POST("/files", h.upload)
	private.GET("/files/:id", h.download)
}

func (h *Handler) upload(c *gin.Context) {
	p, ok := auth.FromContext(c)
	if !ok {
		httpx.Fail(c, apperr.ErrUnauthorized)
		return
	}
	// multipart framing overhead: allow 1 MiB beyond the file cap
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxBytes+1<<20)
	fh, err := c.FormFile("file")
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			httpx.Fail(c, apperr.ErrPayloadTooLarge)
			return
		}
		httpx.Fail(c, apperr.Validation("file", "multipart field 'file' is required"))
		return
	}
	src, err := fh.Open()
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	defer src.Close()
	out, err := h.svc.Upload(c.Request.Context(), p, fh.Filename, fh.Header.Get("Content-Type"), src)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}

func (h *Handler) download(c *gin.Context) {
	p, ok := auth.FromContext(c)
	if !ok {
		httpx.Fail(c, apperr.ErrUnauthorized)
		return
	}
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	meta, rc, err := h.svc.Download(c.Request.Context(), p, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	defer rc.Close()
	name := filepath.Base(strings.ReplaceAll(meta.Name, "\\", "/"))
	name = strings.ReplaceAll(name, `"`, "")
	c.Header("X-Content-Type-Options", "nosniff")
	c.DataFromReader(http.StatusOK, meta.Size, meta.Mime, rc, map[string]string{
		"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, name),
	})
}
