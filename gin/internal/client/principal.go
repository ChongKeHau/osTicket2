package client

import (
	"context"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/httpx"
)

// Principal is the authenticated end user of a portal request.
type Principal struct {
	UserID   int64
	Email    string
	Verified bool
	// TicketID scopes a guest session to one ticket; nil for an account session.
	TicketID *int64
	// PasswordReset marks a session opened from a password-reset link.
	PasswordReset bool
}

// IsGuest reports whether the session is limited to a single ticket.
func (p Principal) IsGuest() bool { return p.TicketID != nil }

const principalKey = "client.principal"

func WithPrincipal(c *gin.Context, p Principal) { c.Set(principalKey, p) }

func FromContext(c *gin.Context) (Principal, bool) {
	v, ok := c.Get(principalKey)
	if !ok {
		return Principal{}, false
	}
	p, ok := v.(Principal)
	return p, ok
}

// UserLoader loads an end user by id for the middleware.
type UserLoader interface {
	LoadClient(ctx context.Context, userID int64) (Principal, error)
}

// Option configures RequireUser.
type Option func(*requireOpts)

type requireOpts struct{ allowPasswordReset bool }

// AllowPasswordReset admits sessions opened from a password-reset link. Only
// the routes that show the account and set the new password use it.
func AllowPasswordReset() Option {
	return func(o *requireOpts) { o.allowPasswordReset = true }
}

// RequireUser verifies a client bearer token and loads the principal.
// Staff tokens are rejected because they lack the client audience, and
// password-reset sessions get 403 reset_session unless AllowPasswordReset is given.
func RequireUser(tokens *Tokens, loader UserLoader, opts ...Option) gin.HandlerFunc {
	var o requireOpts
	for _, opt := range opts {
		opt(&o)
	}
	return func(c *gin.Context) {
		const prefix = "Bearer "
		h := c.GetHeader("Authorization")
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
		if claims.PasswordReset && !o.allowPasswordReset {
			httpx.Fail(c, apperr.ErrResetSession)
			return
		}
		p, err := loader.LoadClient(c.Request.Context(), id)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		p.TicketID = claims.TicketID
		p.PasswordReset = claims.PasswordReset
		WithPrincipal(c, p)
		c.Next()
	}
}

// OptionalUser admits anonymous requests: with no Authorization header it
// continues without a principal; with one it behaves exactly like RequireUser,
// so an invalid, expired, staff or password-reset token is still rejected
// rather than silently treated as anonymous.
func OptionalUser(tokens *Tokens, loader UserLoader) gin.HandlerFunc {
	require := RequireUser(tokens, loader)
	return func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			c.Next()
			return
		}
		require(c)
	}
}

// RequireAccount must run after RequireUser; it turns guest sessions away with 403 guest_session.
func RequireAccount() gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := FromContext(c)
		if !ok {
			httpx.Fail(c, apperr.ErrUnauthorized)
			return
		}
		if p.IsGuest() {
			httpx.Fail(c, apperr.ErrGuestSession)
			return
		}
		c.Next()
	}
}
