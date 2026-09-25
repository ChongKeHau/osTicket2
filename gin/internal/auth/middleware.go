package auth

import (
	"context"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/httpx"
)

type PrincipalLoader interface {
	LoadPrincipal(ctx context.Context, staffID int64) (Principal, error)
}

// RequireAuth verifies the bearer token and loads the principal from the database.
func RequireAuth(tokens *Tokens, loader PrincipalLoader) gin.HandlerFunc {
	return func(c *gin.Context) {
		const prefix = "Bearer "
		h := c.GetHeader("Authorization")
		// The scheme ("Bearer") is case-insensitive per RFC 6750/7235; only
		// the token that follows it is compared exactly.
		if len(h) < len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
			httpx.Fail(c, apperr.ErrUnauthorized)
			return
		}
		claims, err := tokens.ParseAccess(strings.TrimSpace(h[len(prefix):]))
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		id, err := strconv.ParseInt(claims.Subject, 10, 64)
		if err != nil {
			httpx.Fail(c, apperr.ErrUnauthorized)
			return
		}
		p, err := loader.LoadPrincipal(c.Request.Context(), id)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		WithPrincipal(c, p)
		c.Next()
	}
}

// RequireAdmin must run after RequireAuth.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := FromContext(c)
		if !ok {
			httpx.Fail(c, apperr.ErrUnauthorized)
			return
		}
		if !p.IsAdmin {
			httpx.Fail(c, apperr.ErrForbidden)
			return
		}
		c.Next()
	}
}
