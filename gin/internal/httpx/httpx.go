// Package httpx holds HTTP conventions shared by all handlers: the error
// envelope, JSON binding with field names, id and pagination parsing.
package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/grandpine/ticket-api/internal/apperr"
)

func init() {
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		v.RegisterTagNameFunc(func(fld reflect.StructField) string {
			name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
			if name == "-" {
				return ""
			}
			return name
		})
	}
}

type errorBody struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// List is the offset-paginated list envelope.
type List[T any] struct {
	Items    []T   `json:"items"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Total    int64 `json:"total"`
}

// Fail writes the error envelope for err and aborts the request.
func Fail(c *gin.Context, err error) {
	var ve *apperr.ValidationError
	var rl *apperr.RateLimitedError
	switch {
	case errors.As(err, &ve):
		write(c, http.StatusBadRequest, "validation_failed", "request validation failed", ve.Fields)
	case errors.Is(err, apperr.ErrUnauthorized):
		write(c, http.StatusUnauthorized, "unauthorized", "authentication required", nil)
	case errors.Is(err, apperr.ErrGuestSession):
		write(c, http.StatusForbidden, "guest_session", "sign in to an account to do this", nil)
	case errors.Is(err, apperr.ErrResetSession):
		write(c, http.StatusForbidden, "reset_session", "finish setting your password first", nil)
	case errors.Is(err, apperr.ErrForbidden):
		write(c, http.StatusForbidden, "forbidden", err.Error(), nil)
	case errors.Is(err, apperr.ErrNotFound):
		write(c, http.StatusNotFound, "not_found", err.Error(), nil)
	case errors.Is(err, apperr.ErrConflict):
		write(c, http.StatusConflict, "conflict", err.Error(), nil)
	case errors.Is(err, apperr.ErrPayloadTooLarge):
		write(c, http.StatusRequestEntityTooLarge, "payload_too_large", err.Error(), nil)
	case errors.Is(err, apperr.ErrTokenInvalid):
		write(c, http.StatusGone, "token_invalid", "this link has expired or was already used", nil)
	case errors.As(err, &rl):
		secs := int(math.Ceil(rl.RetryAfter.Seconds()))
		write(c, http.StatusTooManyRequests, "rate_limited", "too many attempts, try again later",
			map[string]string{"retry_after": strconv.Itoa(secs)})
	case errors.Is(err, apperr.ErrRateLimited):
		write(c, http.StatusTooManyRequests, "rate_limited", "too many attempts, try again later", nil)
	default:
		slog.Error("internal error", "err", err, "request_id", c.GetString("request_id"),
			"method", c.Request.Method, "path", c.Request.URL.Path)
		write(c, http.StatusInternalServerError, "internal", "internal server error", nil)
	}
}

func write(c *gin.Context, status int, code, msg string, fields map[string]string) {
	c.AbortWithStatusJSON(status, gin.H{"error": errorBody{Code: code, Message: msg, Fields: fields}})
}

// BindJSON binds the body into dst. On failure it writes a 400 (413 for an
// oversized body) and returns false.
func BindJSON(c *gin.Context, dst any) bool {
	err := c.ShouldBindJSON(dst)
	if err == nil {
		return true
	}
	var mbErr *http.MaxBytesError
	if errors.As(err, &mbErr) {
		Fail(c, apperr.ErrPayloadTooLarge)
		return false
	}
	var verrs validator.ValidationErrors
	if errors.As(err, &verrs) {
		fields := make(map[string]string, len(verrs))
		for _, fe := range verrs {
			fields[fe.Field()] = fe.Tag()
		}
		Fail(c, &apperr.ValidationError{Fields: fields})
		return false
	}
	Fail(c, apperr.Validation("body", "malformed json"))
	return false
}

// ReadRawJSON reads the whole request body and restores it onto the request
// so a subsequent BindJSON can still bind it. It's used by handlers that need
// to distinguish an explicit JSON null from an absent key, which a bound
// struct alone can't tell apart (both decode to a nil pointer). On read
// failure it writes the error response and returns ok=false.
func ReadRawJSON(c *gin.Context) (json.RawMessage, bool) {
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		var mbErr *http.MaxBytesError
		if errors.As(err, &mbErr) {
			Fail(c, apperr.ErrPayloadTooLarge)
		} else {
			Fail(c, apperr.Validation("body", "malformed json"))
		}
		return nil, false
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	return raw, true
}

// ParseID reads a numeric path parameter.
func ParseID(c *gin.Context, param string) (int64, error) {
	id, err := strconv.ParseInt(c.Param(param), 10, 64)
	if err != nil || id <= 0 {
		return 0, apperr.Validation(param, "must be a positive integer")
	}
	return id, nil
}

// Page is a parsed page request.
type Page struct {
	Page     int
	PageSize int
}

func (p Page) Limit() int32  { return int32(p.PageSize) }
func (p Page) Offset() int32 { return int32((p.Page - 1) * p.PageSize) }

// maxPage bounds the page query param. int32((p.Page - 1) * p.PageSize) in
// Offset would otherwise wrap for a very large page, producing a negative
// OFFSET and a database error instead of a clean validation error.
const maxPage = 1_000_000

// ParsePage reads page and page_size query params with defaults 1 and 25.
func ParsePage(c *gin.Context) (Page, error) {
	p := Page{Page: 1, PageSize: 25}
	if v := c.Query("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxPage {
			return Page{}, apperr.Validation("page", "must be a positive integer no greater than 1000000")
		}
		p.Page = n
	}
	if v := c.Query("page_size"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			return Page{}, apperr.Validation("page_size", "must be between 1 and 100")
		}
		p.PageSize = n
	}
	return p, nil
}
