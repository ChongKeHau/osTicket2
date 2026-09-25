// Package server assembles the Gin engine: middleware, health, route groups.
package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/httpx"
)

// Pinger is satisfied by *pgxpool.Pool.
type Pinger interface {
	Ping(ctx context.Context) error
}

type Options struct {
	Pinger      Pinger
	CORSOrigins []string
	// TrustedProxies are passed straight to (*gin.Engine).SetTrustedProxies.
	// Nil/empty (the default) trusts no proxy, so gin.Context.ClientIP()
	// always returns the socket address rather than an X-Forwarded-For /
	// X-Real-IP value the client controls -- important for anything keyed by
	// client IP, such as the login rate limiter.
	TrustedProxies []string
	// RequireAuth guards the private group. Nil fails closed: every private
	// route returns 401, rather than silently admitting unauthenticated
	// requests. Tests that need an open private group must pass an explicit
	// pass-through middleware.
	RequireAuth gin.HandlerFunc
	// Mount registers feature routes on the public and private /api/v1 groups.
	Mount func(public, private *gin.RouterGroup)
}

func New(o Options) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	// SetTrustedProxies(nil) for an empty/nil o.TrustedProxies: gin treats a
	// nil trustedCIDRs list as "trust nothing", which is what we want by
	// default (see the Options.TrustedProxies doc comment).
	trusted := o.TrustedProxies
	if len(trusted) == 0 {
		trusted = nil
	}
	if err := r.SetTrustedProxies(trusted); err != nil {
		panic(fmt.Errorf("invalid TrustedProxies: %w", err))
	}
	r.Use(RequestID(), Logger(), Recovery(), CORS(o.CORSOrigins), MaxBodyBytes())
	r.GET("/health", health(o.Pinger))
	api := r.Group("/api/v1")
	public := api.Group("")
	private := api.Group("")
	requireAuth := o.RequireAuth
	if requireAuth == nil {
		requireAuth = func(c *gin.Context) { httpx.Fail(c, apperr.ErrUnauthorized) }
	}
	private.Use(requireAuth)
	if o.Mount != nil {
		o.Mount(public, private)
	}
	r.NoRoute(func(c *gin.Context) { httpx.Fail(c, apperr.ErrNotFound) })
	return r
}

func health(p Pinger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
		defer cancel()
		if p == nil || p.Ping(ctx) != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}
