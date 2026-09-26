package attachment

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
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

// FormFile reads the multipart field "file" of an upload capped at maxBytes
// (plus 1 MiB of multipart framing). On failure it has already written the
// error response (413 when over the cap, 400 when the field is missing) and
// returns ok false. The caller closes the returned file.
func FormFile(c *gin.Context, maxBytes int64) (src multipart.File, fh *multipart.FileHeader, ok bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes+1<<20)
	fh, err := c.FormFile("file")
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			httpx.Fail(c, apperr.ErrPayloadTooLarge)
			return nil, nil, false
		}
		httpx.Fail(c, apperr.Validation("file", "multipart field 'file' is required"))
		return nil, nil, false
	}
	src, err = fh.Open()
	if err != nil {
		httpx.Fail(c, err)
		return nil, nil, false
	}
	return src, fh, true
}

// ServeFile streams a stored file as a download with a sanitised filename.
func ServeFile(c *gin.Context, meta *File, rc io.ReadCloser) {
	defer rc.Close()
	name := filepath.Base(strings.ReplaceAll(meta.Name, "\\", "/"))
	name = strings.ReplaceAll(name, `"`, "")
	c.Header("X-Content-Type-Options", "nosniff")
	c.DataFromReader(http.StatusOK, meta.Size, meta.Mime, rc, map[string]string{
		"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, name),
	})
}

func (h *Handler) upload(c *gin.Context) {
	p, ok := auth.FromContext(c)
	if !ok {
		httpx.Fail(c, apperr.ErrUnauthorized)
		return
	}
	src, fh, ok := FormFile(c, h.maxBytes)
	if !ok {
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
	ServeFile(c, meta, rc)
}
