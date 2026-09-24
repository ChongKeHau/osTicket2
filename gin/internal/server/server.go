// Package server assembles the Gin engine: middleware, health, route groups.
package server

import (
	"context"
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
	// RequireAuth guards the private group. Nil means no auth (tests only).
	RequireAuth gin.HandlerFunc
	// Mount registers feature routes on the public and private /api/v1 groups.
	Mount func(public, private *gin.RouterGroup)
}

func New(o Options) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(RequestID(), Logger(), Recovery(), CORS(o.CORSOrigins), MaxBodyBytes())
	r.GET("/health", health(o.Pinger))
	api := r.Group("/api/v1")
	public := api.Group("")
	private := api.Group("")
	if o.RequireAuth != nil {
		private.Use(o.RequireAuth)
	}
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
