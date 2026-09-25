# Go Ticket API (core slice) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go + Gin JSON API under `gin/` that lets internal agents authenticate, manage departments, help topics and staff, and work tickets (create, list, thread, reply, note, attach files, assign, transfer, change status) on a PostgreSQL schema owned by Flyway.

**Architecture:** Feature packages under `gin/internal/` (`auth`, `dept`, `topic`, `staff`, `ticket`, `attachment`), each with a `handler.go` (Gin) and `service.go` (rules). Services call `sqlc`-generated typed queries in `internal/db` and run multi-row writes through `db.WithTx`. Cross-cutting packages: `config`, `apperr` (typed errors), `httpx` (error envelope, binding, pagination), `server` (engine, middleware, health).

**Tech Stack:** Go 1.26, `github.com/gin-gonic/gin`, `github.com/jackc/pgx/v5`, `sqlc` (run with `go run`), `github.com/golang-jwt/jwt/v5`, `golang.org/x/crypto/bcrypt`, Flyway 10 (Docker image), PostgreSQL 16, `github.com/testcontainers/testcontainers-go` for DB tests.

**Spec:** `docs/superpowers/specs/2026-09-24-go-ticket-api-design.md`

## Global Constraints

- Module path: `github.com/grandpine/ticket-api`; module root is `gin/` at the repository root. All `go`, `make` commands below run from `gin/` unless stated.
- Go 1.26 or newer. PostgreSQL 16. Flyway 10 owns the schema: nothing else creates or alters tables.
- Base path `/api/v1`. Ids numeric in paths. Timestamps RFC 3339 UTC. JSON fields `snake_case`.
- Error envelope: `{"error": {"code", "message", "fields"}}`. Codes: `validation_failed` 400, `unauthorized` 401, `forbidden` 403, `not_found` 404, `conflict` 409, `payload_too_large` 413, `internal` 500.
- List envelope `{items, page, page_size, total}`; thread cursor envelope `{items, next_after}`. `page` default 1, `page_size` default 25 max 100; thread `limit` default 50 max 200.
- Access token 15 minutes, refresh token 14 days. `JWT_SECRET` at least 32 bytes. HS256 only.
- Roles `admin` and `agent`. Agent visibility = `primary_dept_id` plus `staff_department` rows, enforced inside the ticket service. Invisible ticket returns 404.
- Handlers never import `internal/db`. Services never import `gin`.
- Test coverage across `internal/` must be strictly greater than 75% statements, with generated sqlc files excluded. `make test` fails otherwise.
- Every commit message ends with the trailer line `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`.
- Work on a feature branch (create with `superpowers:using-git-worktrees` at execution time), never directly on `main` or `develop`.

## Review Focus

1. **JWT with `alg: none` or signed with another secret** must be rejected with 401, never accepted. Test added to Task 5 (token tests).
2. **Two concurrent refresh calls with the same refresh token** must not both succeed. The revoke must be an atomic `UPDATE ... RETURNING`, and a second use returns 401. Test added to Task 5 (service tests, sequential reuse pins the atomic query).
3. **`page_size=0`, negative, or non-numeric** must return 400 `validation_failed`, not silently return everything or panic. Test added to Task 1 (httpx tests).
4. **Uploaded filename containing path separators** such as `../../etc/passwd` must be stored under a random key and served with only the base name in `Content-Disposition`. Test added to Task 11.
5. **Replying to a closed ticket without `status_id`** must add the response and leave the status closed; it must not silently reopen. Test added to Task 10.

---

## File structure

```
gin/
  go.mod, go.sum
  Makefile, docker-compose.yml, flyway.conf, sqlc.yaml, .gitignore, README.md
  cmd/api/main.go                 subcommands: serve, create-admin, gc-files
  db/migrations/V1__init.sql      schema
  db/migrations/V2__seed.sql      reference data
  db/migrate_test.go              Flyway-in-Docker migration test
  db/queries/*.sql                sqlc inputs, one file per feature
  internal/config/config.go       Load(getenv) with validation
  internal/apperr/apperr.go       sentinel errors, ValidationError
  internal/httpx/httpx.go         Fail, BindJSON, ParseID, ParsePage, List[T]
  internal/server/server.go       New(Options): middleware, /health, groups
  internal/server/middleware.go   RequestID, Logger, Recovery, CORS
  internal/db/pool.go, tx.go      NewPool, Beginner, WithTx, IsUniqueViolation
  internal/db/*.go (generated)    sqlc output: db.go, models.go, *.sql.go
  internal/db/testutil/testutil.go  Pool(t), Tx(t) with migrations applied
  internal/auth/{password,token,principal,service,middleware,handler,admin}.go
  internal/dept/{service,handler}.go
  internal/topic/{service,handler}.go
  internal/staff/{service,handler}.go
  internal/ticket/{types,service,thread,handler}.go
  internal/attachment/{storage,service,handler}.go
```

---

### Task 1: Scaffold, config, apperr, httpx

**Files:**
- Create: `gin/go.mod`, `gin/.gitignore`
- Create: `gin/internal/config/config.go`, `gin/internal/config/config_test.go`
- Create: `gin/internal/apperr/apperr.go`
- Create: `gin/internal/httpx/httpx.go`, `gin/internal/httpx/httpx_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `config.Load(getenv func(string) string) (config.Config, error)`; `apperr.ErrNotFound`, `ErrForbidden`, `ErrUnauthorized`, `ErrConflict`, `ErrPayloadTooLarge`, `*apperr.ValidationError{Fields map[string]string}`, `apperr.Validation(field, msg string) *ValidationError`; `httpx.Fail(c *gin.Context, err error)`, `httpx.BindJSON(c, dst any) bool`, `httpx.ParseID(c, param string) (int64, error)`, `httpx.ParsePage(c) (httpx.Page, error)`, `httpx.Page{Page, PageSize int}` with `Limit() int32` and `Offset() int32`, `httpx.List[T]{Items []T; Page, PageSize int; Total int64}`.

- [ ] **Step 1: Create the module and ignore file**

```bash
mkdir -p gin && cd gin
go mod init github.com/grandpine/ticket-api
go get github.com/gin-gonic/gin@latest github.com/go-playground/validator/v10@latest
cat > .gitignore <<'EOG'
storage/
coverage*.out
bin/
EOG
```

- [ ] **Step 2: Write the failing config test**

`gin/internal/config/config_test.go`:

```go
package config

import (
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"DATABASE_URL": "postgres://x",
		"JWT_SECRET":   strings.Repeat("s", 32),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != 8080 || cfg.StorageDir != "./storage" || cfg.MaxUploadBytes != 10<<20 {
		t.Fatalf("defaults wrong: %+v", cfg)
	}
	if len(cfg.AllowedMIME) == 0 || len(cfg.CORSOrigins) != 0 {
		t.Fatalf("list defaults wrong: %+v", cfg)
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"DATABASE_URL":     "postgres://x",
		"JWT_SECRET":       strings.Repeat("s", 40),
		"PORT":             "9090",
		"STORAGE_DIR":      "/data",
		"CORS_ORIGINS":     "http://a.test, http://b.test,",
		"MAX_UPLOAD_BYTES": "1024",
		"ALLOWED_MIME":     "image/png,text/plain",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != 9090 || cfg.StorageDir != "/data" || cfg.MaxUploadBytes != 1024 {
		t.Fatalf("overrides wrong: %+v", cfg)
	}
	if len(cfg.CORSOrigins) != 2 || cfg.CORSOrigins[1] != "http://b.test" {
		t.Fatalf("cors wrong: %v", cfg.CORSOrigins)
	}
	if len(cfg.AllowedMIME) != 2 {
		t.Fatalf("mime wrong: %v", cfg.AllowedMIME)
	}
}

func TestLoadValidation(t *testing.T) {
	cases := map[string]map[string]string{
		"missing db":      {"JWT_SECRET": strings.Repeat("s", 32)},
		"short secret":    {"DATABASE_URL": "x", "JWT_SECRET": "short"},
		"bad port":        {"DATABASE_URL": "x", "JWT_SECRET": strings.Repeat("s", 32), "PORT": "abc"},
		"bad upload size": {"DATABASE_URL": "x", "JWT_SECRET": strings.Repeat("s", 32), "MAX_UPLOAD_BYTES": "-1"},
	}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(env(m)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
```

- [ ] **Step 3: Run it to verify it fails**

Run: `go test ./internal/config/ -run TestLoad -v`
Expected: FAIL, "undefined: Load".

- [ ] **Step 4: Implement config**

`gin/internal/config/config.go`:

```go
// Package config loads service configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type Config struct {
	DatabaseURL    string
	JWTSecret      string
	Port           int
	StorageDir     string
	CORSOrigins    []string
	MaxUploadBytes int64
	AllowedMIME    []string
}

var defaultMIME = []string{
	"image/png", "image/jpeg", "image/gif", "application/pdf", "text/plain", "text/csv",
	"application/zip", "application/msword",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
}

// Load reads configuration through getenv (usually os.Getenv) and validates it.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		DatabaseURL:    getenv("DATABASE_URL"),
		JWTSecret:      getenv("JWT_SECRET"),
		Port:           8080,
		StorageDir:     "./storage",
		MaxUploadBytes: 10 << 20,
		AllowedMIME:    defaultMIME,
	}
	var errs []error
	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if len(cfg.JWTSecret) < 32 {
		errs = append(errs, errors.New("JWT_SECRET must be at least 32 bytes"))
	}
	if v := getenv("PORT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 65535 {
			errs = append(errs, fmt.Errorf("PORT %q is not a valid port", v))
		} else {
			cfg.Port = n
		}
	}
	if v := getenv("STORAGE_DIR"); v != "" {
		cfg.StorageDir = v
	}
	if v := getenv("CORS_ORIGINS"); v != "" {
		cfg.CORSOrigins = splitList(v)
	}
	if v := getenv("MAX_UPLOAD_BYTES"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			errs = append(errs, fmt.Errorf("MAX_UPLOAD_BYTES %q must be a positive integer", v))
		} else {
			cfg.MaxUploadBytes = n
		}
	}
	if v := getenv("ALLOWED_MIME"); v != "" {
		cfg.AllowedMIME = splitList(v)
	}
	return cfg, errors.Join(errs...)
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
```

- [ ] **Step 5: Run config tests**

Run: `go test ./internal/config/ -v`
Expected: PASS (3 tests).

- [ ] **Step 6: Write apperr**

`gin/internal/apperr/apperr.go`:

```go
// Package apperr defines the error kinds services return and handlers map to HTTP.
package apperr

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrForbidden       = errors.New("forbidden")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrConflict        = errors.New("conflict")
	ErrPayloadTooLarge = errors.New("payload too large")
)

// ValidationError carries per-field messages for a 400 response.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s: %s", k, e.Fields[k]))
	}
	return "validation failed: " + strings.Join(parts, ", ")
}

// Validation builds a single-field ValidationError.
func Validation(field, msg string) *ValidationError {
	return &ValidationError{Fields: map[string]string{field: msg}}
}
```

- [ ] **Step 7: Write the failing httpx tests**

`gin/internal/httpx/httpx_test.go`:

```go
package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/apperr"
)

func init() { gin.SetMode(gin.TestMode) }

type envelope struct {
	Error struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Fields  map[string]string `json:"fields"`
	} `json:"error"`
}

func do(t *testing.T, r *gin.Engine, method, path, body string) (*httptest.ResponseRecorder, envelope) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var e envelope
	_ = json.Unmarshal(w.Body.Bytes(), &e)
	return w, e
}

func TestFailMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{apperr.Validation("subject", "required"), 400, "validation_failed"},
		{apperr.ErrUnauthorized, 401, "unauthorized"},
		{apperr.ErrForbidden, 403, "forbidden"},
		{errors.Join(errors.New("wrapped"), apperr.ErrNotFound), 404, "not_found"},
		{apperr.ErrConflict, 409, "conflict"},
		{apperr.ErrPayloadTooLarge, 413, "payload_too_large"},
		{errors.New("boom"), 500, "internal"},
	}
	for _, tc := range cases {
		r := gin.New()
		r.GET("/x", func(c *gin.Context) { Fail(c, tc.err) })
		w, e := do(t, r, http.MethodGet, "/x", "")
		if w.Code != tc.status || e.Error.Code != tc.code {
			t.Errorf("%v: got %d %q want %d %q", tc.err, w.Code, e.Error.Code, tc.status, tc.code)
		}
		if tc.status == 500 && e.Error.Message != "internal server error" {
			t.Errorf("internal error leaked message %q", e.Error.Message)
		}
		if tc.status == 400 && e.Error.Fields["subject"] != "required" {
			t.Errorf("fields missing: %+v", e.Error.Fields)
		}
	}
}

func TestBindJSONReportsJSONFieldNames(t *testing.T) {
	type in struct {
		Subject string `json:"subject" binding:"required,max=5"`
		Email   string `json:"requester_email" binding:"required,email"`
	}
	r := gin.New()
	r.POST("/x", func(c *gin.Context) {
		var v in
		if !BindJSON(c, &v) {
			return
		}
		c.Status(204)
	})
	w, e := do(t, r, http.MethodPost, "/x", `{"subject":"toolong","requester_email":"nope"}`)
	if w.Code != 400 {
		t.Fatalf("status %d", w.Code)
	}
	if e.Error.Fields["subject"] != "max" || e.Error.Fields["requester_email"] != "email" {
		t.Fatalf("fields: %+v", e.Error.Fields)
	}
	w, e = do(t, r, http.MethodPost, "/x", `{not json`)
	if w.Code != 400 || e.Error.Fields["body"] == "" {
		t.Fatalf("malformed json: %d %+v", w.Code, e.Error.Fields)
	}
	w, _ = do(t, r, http.MethodPost, "/x", `{"subject":"ok","requester_email":"a@b.test"}`)
	if w.Code != 204 {
		t.Fatalf("valid body status %d", w.Code)
	}
}

func TestParsePage(t *testing.T) {
	cases := []struct {
		query    string
		page     int
		size     int
		wantErr  bool
	}{
		{"", 1, 25, false},
		{"page=3&page_size=10", 3, 10, false},
		{"page_size=500", 1, 100, true},
		{"page_size=0", 0, 0, true},
		{"page_size=-5", 0, 0, true},
		{"page_size=abc", 0, 0, true},
		{"page=0", 0, 0, true},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/x?"+tc.query, nil)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = req
		p, err := ParsePage(c)
		if tc.wantErr {
			var ve *apperr.ValidationError
			if !errors.As(err, &ve) {
				t.Errorf("%q: expected validation error, got %v", tc.query, err)
			}
			continue
		}
		if err != nil || p.Page != tc.page || p.PageSize != tc.size {
			t.Errorf("%q: got %+v %v", tc.query, p, err)
		}
	}
	p := Page{Page: 3, PageSize: 10}
	if p.Offset() != 20 || p.Limit() != 10 {
		t.Fatalf("offset/limit: %d %d", p.Offset(), p.Limit())
	}
}

func TestParseID(t *testing.T) {
	r := gin.New()
	r.GET("/x/:id", func(c *gin.Context) {
		id, err := ParseID(c, "id")
		if err != nil {
			Fail(c, err)
			return
		}
		c.JSON(200, gin.H{"id": id})
	})
	w, _ := do(t, r, http.MethodGet, "/x/42", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "42") {
		t.Fatalf("numeric id: %d %s", w.Code, w.Body.String())
	}
	w, e := do(t, r, http.MethodGet, "/x/abc", "")
	if w.Code != 400 || e.Error.Fields["id"] == "" {
		t.Fatalf("non-numeric id: %d %+v", w.Code, e.Error)
	}
}
```

- [ ] **Step 8: Run to verify it fails**

Run: `go test ./internal/httpx/ -v`
Expected: FAIL to compile, "undefined: Fail".

- [ ] **Step 9: Implement httpx**

`gin/internal/httpx/httpx.go`:

```go
// Package httpx holds HTTP conventions shared by all handlers: the error
// envelope, JSON binding with field names, id and pagination parsing.
package httpx

import (
	"errors"
	"log/slog"
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
	switch {
	case errors.As(err, &ve):
		write(c, http.StatusBadRequest, "validation_failed", "request validation failed", ve.Fields)
	case errors.Is(err, apperr.ErrUnauthorized):
		write(c, http.StatusUnauthorized, "unauthorized", "authentication required", nil)
	case errors.Is(err, apperr.ErrForbidden):
		write(c, http.StatusForbidden, "forbidden", err.Error(), nil)
	case errors.Is(err, apperr.ErrNotFound):
		write(c, http.StatusNotFound, "not_found", err.Error(), nil)
	case errors.Is(err, apperr.ErrConflict):
		write(c, http.StatusConflict, "conflict", err.Error(), nil)
	case errors.Is(err, apperr.ErrPayloadTooLarge):
		write(c, http.StatusRequestEntityTooLarge, "payload_too_large", err.Error(), nil)
	default:
		slog.Error("internal error", "err", err, "request_id", c.GetString("request_id"),
			"method", c.Request.Method, "path", c.Request.URL.Path)
		write(c, http.StatusInternalServerError, "internal", "internal server error", nil)
	}
}

func write(c *gin.Context, status int, code, msg string, fields map[string]string) {
	c.AbortWithStatusJSON(status, gin.H{"error": errorBody{Code: code, Message: msg, Fields: fields}})
}

// BindJSON binds the body into dst. On failure it writes a 400 and returns false.
func BindJSON(c *gin.Context, dst any) bool {
	err := c.ShouldBindJSON(dst)
	if err == nil {
		return true
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

// ParsePage reads page and page_size query params with defaults 1 and 25.
func ParsePage(c *gin.Context) (Page, error) {
	p := Page{Page: 1, PageSize: 25}
	if v := c.Query("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return Page{}, apperr.Validation("page", "must be a positive integer")
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
```

- [ ] **Step 10: Run httpx tests**

Run: `go test ./internal/httpx/ -v`
Expected: PASS (4 tests).

- [ ] **Step 11: Commit**

```bash
cd gin && go mod tidy && cd ..
git add gin
git commit -m "feat(gin): scaffold module with config, apperr and httpx packages" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 2: Server skeleton, middleware, health, local tooling

**Files:**
- Create: `gin/internal/server/server.go`, `gin/internal/server/middleware.go`, `gin/internal/server/server_test.go`
- Create: `gin/cmd/api/main.go` (serve only; extended in Task 12)
- Create: `gin/Makefile`, `gin/docker-compose.yml`, `gin/flyway.conf`

**Interfaces:**
- Consumes: `httpx.Fail`, `apperr.ErrNotFound`.
- Produces: `server.Options{Pinger server.Pinger; CORSOrigins []string; RequireAuth gin.HandlerFunc; Mount func(public, private *gin.RouterGroup)}`, `server.New(o Options) *gin.Engine`, `server.Pinger` interface `Ping(ctx context.Context) error`. `RequestID` stores the id under context key `"request_id"`.

- [ ] **Step 1: Write the failing server tests**

`gin/internal/server/server_test.go`:

```go
package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type pinger struct{ err error }

func (p pinger) Ping(context.Context) error { return p.err }

func TestHealth(t *testing.T) {
	r := New(Options{Pinger: pinger{}})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	if w.Code != 200 {
		t.Fatalf("healthy: %d", w.Code)
	}
	r = New(Options{Pinger: pinger{err: errors.New("down")}})
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	if w.Code != 503 {
		t.Fatalf("unhealthy: %d", w.Code)
	}
}

func TestRequestIDAndRecovery(t *testing.T) {
	r := New(Options{Pinger: pinger{}, Mount: func(public, private *gin.RouterGroup) {
		public.GET("/boom", func(c *gin.Context) { panic("kaboom") })
		public.GET("/id", func(c *gin.Context) { c.String(200, c.GetString("request_id")) })
	}})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/boom", nil))
	if w.Code != 500 || !strings.Contains(w.Body.String(), `"internal"`) {
		t.Fatalf("recovery: %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("X-Request-Id") == "" {
		t.Fatal("missing generated request id")
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/id", nil)
	req.Header.Set("X-Request-Id", "abc-123")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Body.String() != "abc-123" || w.Header().Get("X-Request-Id") != "abc-123" {
		t.Fatalf("request id passthrough: %q %q", w.Body.String(), w.Header().Get("X-Request-Id"))
	}
	req.Header.Set("X-Request-Id", strings.Repeat("x", 100))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Header().Get("X-Request-Id") == strings.Repeat("x", 100) {
		t.Fatal("overlong request id should be replaced")
	}
}

func TestCORSAndAuthGroup(t *testing.T) {
	called := false
	r := New(Options{
		Pinger:      pinger{},
		CORSOrigins: []string{"http://app.test"},
		RequireAuth: func(c *gin.Context) { called = true; c.AbortWithStatus(401) },
		Mount: func(public, private *gin.RouterGroup) {
			private.GET("/secret", func(c *gin.Context) { c.Status(200) })
		},
	})
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/secret", nil)
	req.Header.Set("Origin", "http://app.test")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "http://app.test" {
		t.Fatalf("preflight: %d %v", w.Code, w.Header())
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/secret", nil)
	req.Header.Set("Origin", "http://evil.test")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("unknown origin must not be allowed")
	}
	if !called || w.Code != 401 {
		t.Fatalf("auth middleware not applied: called=%v code=%d", called, w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/nope", nil))
	if w.Code != 404 || !strings.Contains(w.Body.String(), `"not_found"`) {
		t.Fatalf("no route: %d %s", w.Code, w.Body.String())
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/server/ -v`
Expected: FAIL to compile, "undefined: New".

- [ ] **Step 3: Implement middleware**

`gin/internal/server/middleware.go`:

```go
package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/httpx"
)

const requestIDKey = "request_id"

// RequestID accepts a sane inbound X-Request-Id or generates one.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-Id")
		if id == "" || len(id) > 64 {
			var b [16]byte
			_, _ = rand.Read(b[:])
			id = hex.EncodeToString(b[:])
		}
		c.Set(requestIDKey, id)
		c.Header("X-Request-Id", id)
		c.Next()
	}
}

// Logger writes one structured line per request.
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.Info("request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", c.GetString(requestIDKey),
		)
	}
}

// Recovery turns panics into the 500 envelope.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				httpx.Fail(c, fmt.Errorf("panic: %v", r))
			}
		}()
		c.Next()
	}
}

// CORS allows the configured origins and answers preflight requests.
func CORS(origins []string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(origins))
	for _, o := range origins {
		allowed[o] = true
	}
	return func(c *gin.Context) {
		if origin := c.GetHeader("Origin"); origin != "" && allowed[origin] {
			h := c.Writer.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Vary", "Origin")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-Id")
			h.Set("Access-Control-Expose-Headers", "X-Request-Id, Content-Disposition")
			h.Set("Access-Control-Max-Age", "600")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
```

- [ ] **Step 4: Implement server**

`gin/internal/server/server.go`:

```go
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
	r.Use(RequestID(), Logger(), Recovery(), CORS(o.CORSOrigins))
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
```

- [ ] **Step 5: Run server tests**

Run: `go test ./internal/server/ -v`
Expected: PASS (3 tests).

- [ ] **Step 6: Write the initial main.go (serve only)**

`gin/cmd/api/main.go`:

```go
// Command api runs the ticket API server and its maintenance subcommands.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/grandpine/ticket-api/internal/config"
	"github.com/grandpine/ticket-api/internal/server"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
		args = args[1:]
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()
	switch cmd {
	case "serve":
		return serve(ctx, cfg, pool)
	default:
		return fmt.Errorf("unknown command %q (expected serve)", cmd)
	}
}

func serve(ctx context.Context, cfg config.Config, pool *pgxpool.Pool) error {
	engine := server.New(server.Options{Pinger: pool, CORSOrigins: cfg.CORSOrigins})
	srv := &http.Server{Addr: fmt.Sprintf(":%d", cfg.Port), Handler: engine, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	slog.Info("listening", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
```

Run: `go get github.com/jackc/pgx/v5@latest && go build ./...`
Expected: builds cleanly.

- [ ] **Step 7: Add Makefile, docker-compose, flyway.conf**

`gin/Makefile`:

```make
SQLC_VERSION ?= latest
COVER_MIN    ?= 75

.PHONY: run migrate migrate-info sqlc test cover db-up db-down

db-up:
	docker compose up -d postgres

db-down:
	docker compose down

migrate: db-up
	docker compose run --rm flyway migrate

migrate-info: db-up
	docker compose run --rm flyway info

sqlc:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) generate

run:
	DATABASE_URL=$${DATABASE_URL:-postgres://ticket:ticket@localhost:5432/ticket?sslmode=disable} \
	JWT_SECRET=$${JWT_SECRET:-dev-secret-change-me-dev-secret-change-me} \
	go run ./cmd/api serve

test:
	go test ./... -coverprofile=coverage.out -covermode=atomic -coverpkg=./internal/...
	grep -vE 'internal/db/(models|db|[a-z_]+\.sql)\.go' coverage.out > coverage.filtered.out
	@go tool cover -func=coverage.filtered.out | tail -1 | awk -v min=$(COVER_MIN) '{ pct=$$3; sub("%","",pct); if (pct+0 <= min) { printf "coverage %s%% is not above %d%%\n", pct, min; exit 1 } else printf "coverage %s%% (minimum %d%%)\n", pct, min }'

cover: test
	go tool cover -html=coverage.filtered.out
```

`gin/docker-compose.yml`:

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: ticket
      POSTGRES_USER: ticket
      POSTGRES_PASSWORD: ticket
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ticket"]
      interval: 2s
      timeout: 3s
      retries: 20
  flyway:
    image: flyway/flyway:10
    profiles: ["tools"]
    depends_on:
      postgres:
        condition: service_healthy
    volumes:
      - ./db/migrations:/flyway/sql:ro
    command: -url=jdbc:postgresql://postgres:5432/ticket -user=ticket -password=ticket -connectRetries=10 migrate
volumes:
  pgdata: {}
```

`gin/flyway.conf` (for anyone with a local Flyway CLI):

```
flyway.url=jdbc:postgresql://localhost:5432/ticket
flyway.user=ticket
flyway.password=ticket
flyway.locations=filesystem:db/migrations
```

- [ ] **Step 8: Commit**

```bash
git add gin
git commit -m "feat(gin): server skeleton with middleware, health endpoint and local tooling" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 3: Migrations, sqlc, db package, test database helper

**Files:**
- Create: `gin/db/migrations/V1__init.sql`, `gin/db/migrations/V2__seed.sql`
- Create: `gin/sqlc.yaml`, `gin/db/queries/ref.sql`
- Create: `gin/internal/db/pool.go`, `gin/internal/db/tx.go`
- Create: `gin/internal/db/testutil/testutil.go`
- Create: `gin/internal/db/db_test.go`
- Generated: `gin/internal/db/db.go`, `gin/internal/db/models.go`, `gin/internal/db/ref.sql.go` (committed)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: generated `db.Queries`, `db.New(DBTX) *Queries`, models `db.Department`, `db.Staff`, `db.TicketPriority`, `db.TicketStatus`, `db.HelpTopic`, `db.ThreadEntry`, `db.File`, `db.RefreshToken`, enums `db.TicketState` (`TicketStateOpen`, `TicketStateResolved`, `TicketStateClosed`), `db.TicketSource`, `db.ThreadEntryType`, `db.BodyFormat`, `db.TicketEventKind`; hand-written `db.Beginner` interface, `db.WithTx(ctx, b Beginner, fn func(*Queries) error) error`, `db.NewPool(ctx, url) (*pgxpool.Pool, error)`, `db.IsUniqueViolation(err) bool`; `testutil.Pool(t) *pgxpool.Pool`, `testutil.Tx(t) pgx.Tx` (rolled back on cleanup), `testutil.MigrationsDir() (string, error)`. Queries: `ListPriorities`, `ListStatuses`, `GetPriority(id)`, `GetStatus(id)`, `DefaultPriority`, `DefaultStatus`.

Note on the spec: the spec describes a generated `tsvector` column on `ticket`. This plan uses an expression GIN index on `to_tsvector('english', subject)` instead, which is equivalent for subject search and keeps `SELECT *` on `ticket` free of a `tsvector` column that sqlc would type as `interface{}`.

- [ ] **Step 1: Write V1 migration**

`gin/db/migrations/V1__init.sql`:

```sql
CREATE TYPE ticket_state AS ENUM ('open', 'resolved', 'closed');
CREATE TYPE ticket_source AS ENUM ('web', 'api', 'phone', 'other');
CREATE TYPE thread_entry_type AS ENUM ('message', 'response', 'note');
CREATE TYPE body_format AS ENUM ('html', 'text');
CREATE TYPE ticket_event_kind AS ENUM (
  'created', 'assigned', 'unassigned', 'status_changed', 'transferred', 'closed', 'reopened', 'edited'
);

CREATE TABLE department (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name        text NOT NULL UNIQUE,
  is_public   boolean NOT NULL DEFAULT true,
  manager_id  bigint,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE staff (
  id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  username         text NOT NULL UNIQUE,
  email            text NOT NULL UNIQUE,
  password_hash    text NOT NULL,
  first_name       text NOT NULL DEFAULT '',
  last_name        text NOT NULL DEFAULT '',
  is_admin         boolean NOT NULL DEFAULT false,
  is_active        boolean NOT NULL DEFAULT true,
  primary_dept_id  bigint NOT NULL REFERENCES department(id),
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE department
  ADD CONSTRAINT department_manager_fk FOREIGN KEY (manager_id) REFERENCES staff(id);

CREATE TABLE staff_department (
  staff_id  bigint NOT NULL REFERENCES staff(id) ON DELETE CASCADE,
  dept_id   bigint NOT NULL REFERENCES department(id),
  PRIMARY KEY (staff_id, dept_id)
);

CREATE TABLE refresh_token (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  token_hash  text NOT NULL UNIQUE,
  staff_id    bigint NOT NULL REFERENCES staff(id) ON DELETE CASCADE,
  expires_at  timestamptz NOT NULL,
  revoked_at  timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX refresh_token_staff_idx ON refresh_token (staff_id);

CREATE TABLE ticket_priority (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name        text NOT NULL UNIQUE,
  urgency     int NOT NULL,
  color       text NOT NULL DEFAULT '',
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE ticket_status (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name        text NOT NULL UNIQUE,
  state       ticket_state NOT NULL,
  sort_order  int NOT NULL DEFAULT 0,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE help_topic (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name         text NOT NULL,
  dept_id      bigint REFERENCES department(id),
  priority_id  bigint REFERENCES ticket_priority(id),
  is_active    boolean NOT NULL DEFAULT true,
  sort_order   int NOT NULL DEFAULT 0,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE SEQUENCE ticket_number_seq START 1;

CREATE TABLE ticket (
  id                 bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  number             text NOT NULL UNIQUE,
  subject            text NOT NULL,
  status_id          bigint NOT NULL REFERENCES ticket_status(id),
  dept_id            bigint NOT NULL REFERENCES department(id),
  topic_id           bigint REFERENCES help_topic(id),
  priority_id        bigint NOT NULL REFERENCES ticket_priority(id),
  assigned_staff_id  bigint REFERENCES staff(id),
  requester_name     text NOT NULL DEFAULT '',
  requester_email    text NOT NULL,
  source             ticket_source NOT NULL DEFAULT 'web',
  is_answered        boolean NOT NULL DEFAULT false,
  due_at             timestamptz,
  closed_at          timestamptz,
  last_message_at    timestamptz NOT NULL DEFAULT now(),
  last_response_at   timestamptz,
  extra              jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ticket_status_idx ON ticket (status_id);
CREATE INDEX ticket_dept_idx ON ticket (dept_id);
CREATE INDEX ticket_assignee_idx ON ticket (assigned_staff_id);
CREATE INDEX ticket_last_message_idx ON ticket (last_message_at DESC);
CREATE INDEX ticket_subject_search_idx ON ticket USING GIN (to_tsvector('english', subject));

CREATE TABLE thread_entry (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  ticket_id   bigint NOT NULL REFERENCES ticket(id) ON DELETE CASCADE,
  type        thread_entry_type NOT NULL,
  staff_id    bigint REFERENCES staff(id),
  poster      text NOT NULL DEFAULT '',
  title       text,
  body        text NOT NULL,
  format      body_format NOT NULL DEFAULT 'html',
  parent_id   bigint REFERENCES thread_entry(id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX thread_entry_ticket_idx ON thread_entry (ticket_id, id);

CREATE TABLE ticket_event (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  ticket_id   bigint NOT NULL REFERENCES ticket(id) ON DELETE CASCADE,
  staff_id    bigint REFERENCES staff(id),
  kind        ticket_event_kind NOT NULL,
  data        jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ticket_event_ticket_idx ON ticket_event (ticket_id, id);

CREATE TABLE file (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  key          text NOT NULL UNIQUE,
  name         text NOT NULL,
  mime         text NOT NULL,
  size         bigint NOT NULL,
  sha256       text NOT NULL,
  backend      text NOT NULL DEFAULT 'local',
  uploaded_by  bigint REFERENCES staff(id),
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE attachment (
  thread_entry_id  bigint NOT NULL REFERENCES thread_entry(id) ON DELETE CASCADE,
  file_id          bigint NOT NULL REFERENCES file(id),
  inline           boolean NOT NULL DEFAULT false,
  PRIMARY KEY (thread_entry_id, file_id)
);
CREATE INDEX attachment_file_idx ON attachment (file_id);
```

- [ ] **Step 2: Write V2 seed**

`gin/db/migrations/V2__seed.sql`:

```sql
INSERT INTO ticket_priority (name, urgency, color) VALUES
  ('low', 1, '#DDFFDD'),
  ('normal', 2, '#FFFFFF'),
  ('high', 3, '#FEE7E7'),
  ('emergency', 4, '#FF0000');

INSERT INTO ticket_status (name, state, sort_order) VALUES
  ('Open', 'open', 1),
  ('Resolved', 'resolved', 2),
  ('Closed', 'closed', 3);

INSERT INTO department (name, is_public) VALUES ('Support', true);

INSERT INTO help_topic (name, dept_id, priority_id, is_active, sort_order) VALUES (
  'General Inquiry',
  (SELECT id FROM department WHERE name = 'Support'),
  (SELECT id FROM ticket_priority WHERE name = 'normal'),
  true, 1
);
```

- [ ] **Step 3: Write sqlc config and the first queries**

`gin/sqlc.yaml`:

```yaml
version: "2"
sql:
  - engine: "postgresql"
    schema: "db/migrations"
    queries: "db/queries"
    gen:
      go:
        package: "db"
        out: "internal/db"
        sql_package: "pgx/v5"
        emit_pointers_for_null_types: true
        emit_empty_slices: true
        overrides:
          - db_type: "pg_catalog.timestamptz"
            go_type: "time.Time"
          - db_type: "pg_catalog.timestamptz"
            nullable: true
            go_type:
              import: "time"
              type: "Time"
              pointer: true
```

`gin/db/queries/ref.sql`:

```sql
-- name: ListPriorities :many
SELECT * FROM ticket_priority ORDER BY urgency;

-- name: GetPriority :one
SELECT * FROM ticket_priority WHERE id = $1;

-- name: DefaultPriority :one
SELECT * FROM ticket_priority ORDER BY (name = 'normal') DESC, urgency LIMIT 1;

-- name: ListStatuses :many
SELECT * FROM ticket_status ORDER BY sort_order;

-- name: GetStatus :one
SELECT * FROM ticket_status WHERE id = $1;

-- name: DefaultStatus :one
SELECT * FROM ticket_status WHERE state = 'open' ORDER BY sort_order LIMIT 1;
```

Run: `make sqlc`
Expected: `internal/db/db.go`, `models.go`, `ref.sql.go` generated. Check `models.go` has `type TicketState string` with constants `TicketStateOpen` etc., and `Ticket.DueAt *time.Time`, `Ticket.Extra []byte`.

- [ ] **Step 4: Hand-written db helpers**

`gin/internal/db/pool.go`:

```go
package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool connects and verifies the connection.
func NewPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("db pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db ping: %w", err)
	}
	return pool, nil
}

// IsUniqueViolation reports whether err is a Postgres unique constraint error.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// IsForeignKeyViolation reports whether err is a Postgres FK constraint error.
func IsForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
```

`gin/internal/db/tx.go`:

```go
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Beginner is satisfied by *pgxpool.Pool and by pgx.Tx (nested Begin creates a
// savepoint), which lets tests wrap a service in a rolled-back transaction.
type Beginner interface {
	DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// WithTx runs fn inside a transaction and commits if fn returns nil.
func WithTx(ctx context.Context, b Beginner, fn func(q *Queries) error) error {
	tx, err := b.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	if err := fn(New(tx)); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Test database helper**

`gin/internal/db/testutil/testutil.go`:

```go
// Package testutil starts one Postgres container per test binary, applies the
// Flyway migration files in order, and hands each test a rolled-back transaction.
package testutil

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	once     sync.Once
	pool     *pgxpool.Pool
	startErr error
)

// Pool returns the shared migrated pool, starting the container on first use.
func Pool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	once.Do(func() { pool, startErr = start(context.Background()) })
	if startErr != nil {
		t.Fatalf("test database: %v", startErr)
	}
	return pool
}

// Tx begins a transaction that is rolled back when the test ends.
func Tx(t testing.TB) pgx.Tx {
	t.Helper()
	tx, err := Pool(t).Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}

func start(ctx context.Context) (*pgxpool.Pool, error) {
	ctr, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("ticket"),
		postgres.WithUsername("ticket"),
		postgres.WithPassword("ticket"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(90*time.Second)),
	)
	if err != nil {
		return nil, fmt.Errorf("start postgres: %w", err)
	}
	url, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return nil, err
	}
	p, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := applyMigrations(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

var versionRe = regexp.MustCompile(`^V(\d+)__.*\.sql$`)

func applyMigrations(ctx context.Context, p *pgxpool.Pool) error {
	dir, err := MigrationsDir()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	type mig struct {
		v    int
		name string
	}
	var migs []mig
	for _, e := range entries {
		m := versionRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		v, _ := strconv.Atoi(m[1])
		migs = append(migs, mig{v, e.Name()})
	}
	sort.Slice(migs, func(i, j int) bool { return migs[i].v < migs[j].v })
	for _, m := range migs {
		sql, err := os.ReadFile(filepath.Join(dir, m.name))
		if err != nil {
			return err
		}
		if _, err := p.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("apply %s: %w", m.name, err)
		}
	}
	return nil
}

// MigrationsDir walks up from the working directory to find db/migrations.
func MigrationsDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		cand := filepath.Join(wd, "db", "migrations")
		if st, err := os.Stat(cand); err == nil && st.IsDir() {
			return cand, nil
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			return "", errors.New("db/migrations not found above working directory")
		}
		wd = parent
	}
}
```

Run: `go get github.com/testcontainers/testcontainers-go@latest github.com/testcontainers/testcontainers-go/modules/postgres@latest && go mod tidy`

- [ ] **Step 6: Write the db test**

`gin/internal/db/db_test.go`:

```go
package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/db/testutil"
)

func TestSeedData(t *testing.T) {
	ctx := context.Background()
	q := db.New(testutil.Tx(t))
	statuses, err := q.ListStatuses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 3 || statuses[0].State != db.TicketStateOpen || statuses[2].State != db.TicketStateClosed {
		t.Fatalf("statuses: %+v", statuses)
	}
	prios, err := q.ListPriorities(ctx)
	if err != nil || len(prios) != 4 || prios[0].Name != "low" {
		t.Fatalf("priorities: %+v %v", prios, err)
	}
	def, err := q.DefaultPriority(ctx)
	if err != nil || def.Name != "normal" {
		t.Fatalf("default priority: %+v %v", def, err)
	}
	ds, err := q.DefaultStatus(ctx)
	if err != nil || ds.State != db.TicketStateOpen {
		t.Fatalf("default status: %+v %v", ds, err)
	}
}

func TestWithTxRollsBackOnError(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	boom := errors.New("boom")
	err := db.WithTx(ctx, tx, func(q *db.Queries) error {
		if _, err := tx.Exec(ctx, `INSERT INTO department (name) VALUES ('Rollback Me')`); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected boom, got %v", err)
	}
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM department WHERE name = 'Rollback Me'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("row survived rollback")
	}
	err = db.WithTx(ctx, tx, func(q *db.Queries) error {
		_, err := tx.Exec(ctx, `INSERT INTO department (name) VALUES ('Keep Me')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM department WHERE name = 'Keep Me'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("committed savepoint missing: %d %v", n, err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO department (name) VALUES ('Keep Me')`); !db.IsUniqueViolation(err) {
		t.Fatalf("expected unique violation, got %v", err)
	}
}
```

Note: inside `WithTx`, the closure uses the outer `tx` for raw `Exec`. Statements on the outer tx while a savepoint is open run inside that savepoint, so the rollback assertion holds. Service code will use `q` (bound to the savepoint) instead.

- [ ] **Step 7: Run db tests (Docker required)**

Run: `go test ./internal/db/ -v`
Expected: PASS (2 tests). The first run pulls `postgres:16-alpine`.

- [ ] **Step 8: Commit**

```bash
git add gin
git commit -m "feat(gin): Flyway schema, sqlc setup, db helpers and test database" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 4: Flyway migration test

**Files:**
- Create: `gin/db/migrate_test.go`

**Interfaces:**
- Consumes: `gin/db/migrations/*.sql`.
- Produces: nothing; proves the spec's "second apply is a no-op" requirement with the real Flyway image.

- [ ] **Step 1: Write the test**

`gin/db/migrate_test.go`:

```go
package migrations

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestFlywayMigrateTwice(t *testing.T) {
	ctx := context.Background()
	net, err := network.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = net.Remove(ctx) })

	pg, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("ticket"), postgres.WithUsername("ticket"), postgres.WithPassword("ticket"),
		network.WithNetwork([]string{"pg"}, net),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).WithStartupTimeout(90*time.Second)),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	dir, err := filepath.Abs("migrations")
	if err != nil {
		t.Fatal(err)
	}
	runFlyway := func() string {
		c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image: "flyway/flyway:10",
				Cmd: []string{"-url=jdbc:postgresql://pg:5432/ticket", "-user=ticket", "-password=ticket",
					"-locations=filesystem:/flyway/sql", "-connectRetries=10", "migrate"},
				Networks: []string{net.Name},
				HostConfigModifier: func(hc *container.HostConfig) {
					hc.Binds = append(hc.Binds, dir+":/flyway/sql:ro")
				},
				WaitingFor: wait.ForExit().WithExitTimeout(3 * time.Minute),
			},
			Started: true,
		})
		if err != nil {
			t.Fatalf("flyway: %v", err)
		}
		defer func() { _ = c.Terminate(ctx) }()
		rc, err := c.Logs(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		b, _ := io.ReadAll(rc)
		return string(b)
	}

	first := runFlyway()
	if !strings.Contains(first, "Successfully applied 2 migrations") {
		t.Fatalf("first run:\n%s", first)
	}
	second := runFlyway()
	if !strings.Contains(second, "No migration necessary") {
		t.Fatalf("second run:\n%s", second)
	}

	url, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	var applied int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM flyway_schema_history WHERE success`).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied != 2 {
		t.Fatalf("expected 2 successful migrations in history, got %d", applied)
	}
	var tables int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name IN ('ticket','staff','department','thread_entry','file','attachment')`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 6 {
		t.Fatalf("expected 6 core tables, got %d", tables)
	}
}
```

- [ ] **Step 2: Run it**

Run: `go mod tidy && go test ./db/ -v -run TestFlywayMigrateTwice`
Expected: PASS. First run pulls `flyway/flyway:10`. If the `docker/docker` import is missing, `go mod tidy` adds it because testcontainers already depends on it.

- [ ] **Step 3: Commit**

```bash
git add gin
git commit -m "test(gin): verify Flyway applies migrations and second run is a no-op" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 5: Auth (passwords, JWT, refresh tokens, middleware, handlers, create-admin)

**Files:**
- Create: `gin/db/queries/staff.sql`, `gin/db/queries/refresh_token.sql`, `gin/db/queries/dept.sql` (first queries; more added in Tasks 6 and 8)
- Create: `gin/internal/auth/password.go`, `token.go`, `principal.go`, `service.go`, `middleware.go`, `handler.go`, `admin.go`
- Create: `gin/internal/auth/token_test.go`, `service_test.go`, `handler_test.go`

**Interfaces:**
- Consumes: `db.Queries`, `db.WithTx`, `db.Beginner`, `httpx.*`, `apperr.*`.
- Produces:
  - `auth.HashPassword(pw string) (string, error)`, `auth.CheckPassword(hash, pw string) bool`.
  - `auth.NewTokens(secret string, accessTTL time.Duration) *Tokens`; `(*Tokens).IssueAccess(staffID int64, isAdmin bool) (token string, expiresAt time.Time, err error)`; `(*Tokens).ParseAccess(raw string) (Claims, error)`; `auth.NewRefreshToken() (raw, hash string, err error)`; `auth.HashRefreshToken(raw string) string`.
  - `auth.Principal{StaffID int64; IsAdmin bool; DeptIDs []int64}` with `CanSeeDept(id int64) bool`; `auth.WithPrincipal(c *gin.Context, p Principal)`; `auth.FromContext(c) (Principal, bool)`.
  - `auth.RequireAuth(tokens *Tokens, loader PrincipalLoader) gin.HandlerFunc`; `auth.RequireAdmin() gin.HandlerFunc`; `auth.PrincipalLoader` interface `LoadPrincipal(ctx, staffID int64) (Principal, error)`.
  - `auth.NewService(b db.Beginner, tokens *Tokens, refreshTTL time.Duration) *Service`; methods `Login(ctx, username, password string) (*Session, error)`, `Refresh(ctx, raw string) (*Session, error)`, `Logout(ctx, raw string) error`, `Me(ctx, staffID int64) (*StaffProfile, error)`, `LoadPrincipal`. `Session{AccessToken, RefreshToken string; ExpiresIn int; Staff StaffProfile}`; `StaffProfile{ID int64; Username, Email, FirstName, LastName string; IsAdmin bool; DepartmentIDs []int64}`.
  - `auth.NewHandler(svc SessionService) *Handler`; `(*Handler).Mount(public, private *gin.RouterGroup)`.
  - `auth.CreateAdmin(ctx, b db.Beginner, username, email, password string) (int64, error)`.
  - Queries: `CreateStaff`, `GetStaff`, `GetStaffByUsername`, `ListStaffDepartmentIDs`, `AddStaffDepartment`, `CreateRefreshToken`, `ConsumeRefreshToken`, `RevokeRefreshToken`, `RevokeStaffRefreshTokens`, `FirstDepartment`, `GetDepartment`.

- [ ] **Step 1: Add queries and regenerate**

`gin/db/queries/staff.sql`:

```sql
-- name: CreateStaff :one
INSERT INTO staff (username, email, password_hash, first_name, last_name, is_admin, is_active, primary_dept_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetStaff :one
SELECT * FROM staff WHERE id = $1;

-- name: GetStaffByUsername :one
SELECT * FROM staff WHERE username = $1;

-- name: ListStaffDepartmentIDs :many
SELECT dept_id FROM staff_department WHERE staff_id = $1 ORDER BY dept_id;

-- name: AddStaffDepartment :exec
INSERT INTO staff_department (staff_id, dept_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;
```

`gin/db/queries/refresh_token.sql`:

```sql
-- name: CreateRefreshToken :exec
INSERT INTO refresh_token (token_hash, staff_id, expires_at) VALUES ($1, $2, $3);

-- name: ConsumeRefreshToken :one
UPDATE refresh_token SET revoked_at = now()
WHERE token_hash = $1 AND revoked_at IS NULL
RETURNING *;

-- name: RevokeRefreshToken :exec
UPDATE refresh_token SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: RevokeStaffRefreshTokens :exec
UPDATE refresh_token SET revoked_at = now() WHERE staff_id = $1 AND revoked_at IS NULL;
```

`gin/db/queries/dept.sql` (initial):

```sql
-- name: GetDepartment :one
SELECT * FROM department WHERE id = $1;

-- name: FirstDepartment :one
SELECT * FROM department ORDER BY id LIMIT 1;
```

Run: `make sqlc && go get github.com/golang-jwt/jwt/v5@latest golang.org/x/crypto@latest && go build ./...`

- [ ] **Step 2: Write the failing token tests**

`gin/internal/auth/token_test.go`:

```go
package auth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/grandpine/ticket-api/internal/apperr"
)

const secret = "0123456789abcdef0123456789abcdef"

func TestAccessTokenRoundTrip(t *testing.T) {
	tk := NewTokens(secret, 15*time.Minute)
	raw, exp, err := tk.IssueAccess(42, true)
	if err != nil {
		t.Fatal(err)
	}
	if time.Until(exp) < 14*time.Minute {
		t.Fatalf("expiry too soon: %v", exp)
	}
	claims, err := tk.ParseAccess(raw)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "42" || claims.Role != "admin" || claims.ID == "" {
		t.Fatalf("claims: %+v", claims)
	}
	if _, _, err := NewTokens(secret, time.Minute).IssueAccess(7, false); err != nil {
		t.Fatal(err)
	}
}

func TestAccessTokenRejections(t *testing.T) {
	tk := NewTokens(secret, 15*time.Minute)
	raw, _, _ := tk.IssueAccess(1, false)

	expired := NewTokens(secret, 15*time.Minute)
	expired.now = func() time.Time { return time.Now().Add(-time.Hour) }
	old, _, _ := expired.IssueAccess(1, false)

	other, _, _ := NewTokens(strings.Repeat("x", 32), 15*time.Minute).IssueAccess(1, false)

	none := jwt.NewWithClaims(jwt.SigningMethodNone, Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "1"}})
	noneRaw, err := none.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}

	for name, tok := range map[string]string{
		"expired":     old,
		"wrong key":   other,
		"alg none":    noneRaw,
		"garbage":     "not.a.jwt",
		"tampered":    raw[:len(raw)-3] + "abc",
	} {
		if _, err := tk.ParseAccess(tok); !errors.Is(err, apperr.ErrUnauthorized) {
			t.Errorf("%s: expected ErrUnauthorized, got %v", name, err)
		}
	}
}

func TestRefreshTokenHash(t *testing.T) {
	raw, hash, err := NewRefreshToken()
	if err != nil || len(raw) != 64 || hash != HashRefreshToken(raw) {
		t.Fatalf("raw=%q hash=%q err=%v", raw, hash, err)
	}
	raw2, _, _ := NewRefreshToken()
	if raw2 == raw {
		t.Fatal("refresh tokens must be random")
	}
}

func TestPassword(t *testing.T) {
	h, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(h, "correct horse") || CheckPassword(h, "wrong") {
		t.Fatal("password check wrong")
	}
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("short password must be rejected")
	}
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./internal/auth/ -run 'Token|Password' -v`
Expected: FAIL to compile, "undefined: NewTokens".

- [ ] **Step 4: Implement password and tokens**

`gin/internal/auth/password.go`:

```go
package auth

import (
	"github.com/grandpine/ticket-api/internal/apperr"
	"golang.org/x/crypto/bcrypt"
)

// bcryptCost is a variable so tests can lower it.
var bcryptCost = bcrypt.DefaultCost

const minPasswordLen = 8

func HashPassword(pw string) (string, error) {
	if len(pw) < minPasswordLen {
		return "", apperr.Validation("password", "must be at least 8 characters")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}
```

`gin/internal/auth/token.go`:

```go
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/grandpine/ticket-api/internal/apperr"
)

type Claims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// Tokens issues and verifies HS256 access tokens.
type Tokens struct {
	secret    []byte
	accessTTL time.Duration
	now       func() time.Time
}

func NewTokens(secret string, accessTTL time.Duration) *Tokens {
	return &Tokens{secret: []byte(secret), accessTTL: accessTTL, now: time.Now}
}

func (t *Tokens) AccessTTL() time.Duration { return t.accessTTL }

func (t *Tokens) IssueAccess(staffID int64, isAdmin bool) (string, time.Time, error) {
	role := "agent"
	if isAdmin {
		role = "admin"
	}
	now := t.now()
	exp := now.Add(t.accessTTL)
	claims := Claims{
		Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(staffID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
			ID:        randomHex(16),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, exp, nil
}

func (t *Tokens) ParseAccess(raw string) (Claims, error) {
	var claims Claims
	_, err := jwt.ParseWithClaims(raw, &claims, func(tok *jwt.Token) (any, error) {
		if tok.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method %v", tok.Header["alg"])
		}
		return t.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if err != nil {
		return Claims{}, fmt.Errorf("%w: %v", apperr.ErrUnauthorized, err)
	}
	return claims, nil
}

// NewRefreshToken returns a random opaque token and its storage hash.
func NewRefreshToken() (raw, hash string, err error) {
	raw = randomHex(32)
	if raw == "" {
		return "", "", fmt.Errorf("random source unavailable")
	}
	return raw, HashRefreshToken(raw), nil
}

func HashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}
```

- [ ] **Step 5: Run token tests**

Run: `go test ./internal/auth/ -run 'Token|Password' -v`
Expected: PASS (4 tests).

- [ ] **Step 6: Principal and middleware**

`gin/internal/auth/principal.go`:

```go
package auth

import "github.com/gin-gonic/gin"

// Principal is the authenticated agent for one request.
type Principal struct {
	StaffID int64
	IsAdmin bool
	DeptIDs []int64
}

func (p Principal) CanSeeDept(id int64) bool {
	if p.IsAdmin {
		return true
	}
	for _, d := range p.DeptIDs {
		if d == id {
			return true
		}
	}
	return false
}

const principalKey = "auth.principal"

func WithPrincipal(c *gin.Context, p Principal) { c.Set(principalKey, p) }

func FromContext(c *gin.Context) (Principal, bool) {
	v, ok := c.Get(principalKey)
	if !ok {
		return Principal{}, false
	}
	p, ok := v.(Principal)
	return p, ok
}
```

`gin/internal/auth/middleware.go`:

```go
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
		if !strings.HasPrefix(h, prefix) {
			httpx.Fail(c, apperr.ErrUnauthorized)
			return
		}
		claims, err := tokens.ParseAccess(strings.TrimSpace(strings.TrimPrefix(h, prefix)))
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
```

- [ ] **Step 7: Write the failing service tests**

`gin/internal/auth/service_test.go`:

```go
package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/db/testutil"
	"golang.org/x/crypto/bcrypt"
)

func init() { bcryptCost = bcrypt.MinCost }

type fixture struct {
	ctx   context.Context
	q     *db.Queries
	svc   *Service
	dept  db.Department
	dept2 db.Department
	agent db.Staff
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	tx := testutil.Tx(t)
	q := db.New(tx)
	dept, err := q.FirstDepartment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var dept2 db.Department
	if err := tx.QueryRow(ctx, `INSERT INTO department (name) VALUES ('Billing') RETURNING id, name, is_public, manager_id, created_at, updated_at`).
		Scan(&dept2.ID, &dept2.Name, &dept2.IsPublic, &dept2.ManagerID, &dept2.CreatedAt, &dept2.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	hash, _ := HashPassword("password1")
	agent, err := q.CreateStaff(ctx, db.CreateStaffParams{
		Username: "agent", Email: "agent@example.test", PasswordHash: hash,
		FirstName: "Ann", LastName: "Agent", IsAdmin: false, IsActive: true, PrimaryDeptID: dept.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.AddStaffDepartment(ctx, db.AddStaffDepartmentParams{StaffID: agent.ID, DeptID: dept2.ID}); err != nil {
		t.Fatal(err)
	}
	svc := NewService(tx, NewTokens("0123456789abcdef0123456789abcdef", 15*time.Minute), 14*24*time.Hour)
	return &fixture{ctx: ctx, q: q, svc: svc, dept: dept, dept2: dept2, agent: agent}
}

func TestLogin(t *testing.T) {
	f := newFixture(t)
	sess, err := f.svc.Login(f.ctx, "agent", "password1")
	if err != nil {
		t.Fatal(err)
	}
	if sess.AccessToken == "" || sess.RefreshToken == "" || sess.ExpiresIn != 900 {
		t.Fatalf("session: %+v", sess)
	}
	if sess.Staff.Username != "agent" || len(sess.Staff.DepartmentIDs) != 2 {
		t.Fatalf("profile: %+v", sess.Staff)
	}
	for name, cred := range map[string][2]string{
		"wrong password": {"agent", "nope"},
		"unknown user":   {"ghost", "password1"},
	} {
		if _, err := f.svc.Login(f.ctx, cred[0], cred[1]); !errors.Is(err, apperr.ErrUnauthorized) {
			t.Errorf("%s: got %v", name, err)
		}
	}
	if _, err := f.q.DB().Exec(f.ctx, `UPDATE staff SET is_active = false WHERE id = $1`, f.agent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Login(f.ctx, "agent", "password1"); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatalf("inactive: got %v", err)
	}
}

func TestRefreshRotatesAndRejectsReuse(t *testing.T) {
	f := newFixture(t)
	sess, err := f.svc.Login(f.ctx, "agent", "password1")
	if err != nil {
		t.Fatal(err)
	}
	next, err := f.svc.Refresh(f.ctx, sess.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if next.RefreshToken == sess.RefreshToken {
		t.Fatal("refresh token must rotate")
	}
	if _, err := f.svc.Refresh(f.ctx, sess.RefreshToken); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatalf("reuse of consumed token: got %v", err)
	}
	if _, err := f.svc.Refresh(f.ctx, "unknown"); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatalf("unknown token: got %v", err)
	}
	if err := f.svc.Logout(f.ctx, next.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Refresh(f.ctx, next.RefreshToken); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatalf("after logout: got %v", err)
	}
}

func TestRefreshExpired(t *testing.T) {
	f := newFixture(t)
	f.svc.now = func() time.Time { return time.Now().Add(-30 * 24 * time.Hour) }
	sess, err := f.svc.Login(f.ctx, "agent", "password1")
	if err != nil {
		t.Fatal(err)
	}
	f.svc.now = time.Now
	if _, err := f.svc.Refresh(f.ctx, sess.RefreshToken); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatalf("expired refresh: got %v", err)
	}
}

func TestLoadPrincipalAndMe(t *testing.T) {
	f := newFixture(t)
	p, err := f.svc.LoadPrincipal(f.ctx, f.agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if p.IsAdmin || !p.CanSeeDept(f.dept.ID) || !p.CanSeeDept(f.dept2.ID) || p.CanSeeDept(999999) {
		t.Fatalf("principal: %+v", p)
	}
	if _, err := f.svc.LoadPrincipal(f.ctx, 999999); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatalf("unknown staff: %v", err)
	}
	me, err := f.svc.Me(f.ctx, f.agent.ID)
	if err != nil || me.Email != "agent@example.test" || len(me.DepartmentIDs) != 2 {
		t.Fatalf("me: %+v %v", me, err)
	}
}

func TestCreateAdmin(t *testing.T) {
	f := newFixture(t)
	id, err := CreateAdmin(f.ctx, f.q.DB().(db.Beginner), "root", "root@example.test", "rootpassword")
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.svc.LoadPrincipal(f.ctx, id)
	if err != nil || !p.IsAdmin {
		t.Fatalf("admin principal: %+v %v", p, err)
	}
	if _, err := CreateAdmin(f.ctx, f.q.DB().(db.Beginner), "root", "other@example.test", "rootpassword"); !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("duplicate admin: %v", err)
	}
}
```

Note: `q.DB()` is not generated by sqlc. Add it in `gin/internal/db/tx.go`:

```go
// DB exposes the underlying connection for tests and raw statements.
func (q *Queries) DB() DBTX { return q.db }
```

- [ ] **Step 8: Run to verify it fails**

Run: `go test ./internal/auth/ -run 'Login|Refresh|Principal|CreateAdmin' -v`
Expected: FAIL to compile, "undefined: NewService".

- [ ] **Step 9: Implement the service and create-admin**

`gin/internal/auth/service.go`:

```go
package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/jackc/pgx/v5"
)

type StaffProfile struct {
	ID            int64   `json:"id"`
	Username      string  `json:"username"`
	Email         string  `json:"email"`
	FirstName     string  `json:"first_name"`
	LastName      string  `json:"last_name"`
	IsAdmin       bool    `json:"is_admin"`
	DepartmentIDs []int64 `json:"department_ids"`
}

type Session struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	ExpiresIn    int          `json:"expires_in"`
	Staff        StaffProfile `json:"staff"`
}

type Service struct {
	db         db.Beginner
	tokens     *Tokens
	refreshTTL time.Duration
	now        func() time.Time
}

func NewService(b db.Beginner, tokens *Tokens, refreshTTL time.Duration) *Service {
	return &Service{db: b, tokens: tokens, refreshTTL: refreshTTL, now: time.Now}
}

func (s *Service) Login(ctx context.Context, username, password string) (*Session, error) {
	q := db.New(s.db)
	st, err := q.GetStaffByUsername(ctx, username)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	if !st.IsActive || !CheckPassword(st.PasswordHash, password) {
		return nil, apperr.ErrUnauthorized
	}
	return s.issue(ctx, q, st)
}

func (s *Service) Refresh(ctx context.Context, raw string) (*Session, error) {
	var sess *Session
	err := db.WithTx(ctx, s.db, func(q *db.Queries) error {
		rt, err := q.ConsumeRefreshToken(ctx, HashRefreshToken(raw))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrUnauthorized
		}
		if err != nil {
			return err
		}
		if !rt.ExpiresAt.After(s.now()) {
			return apperr.ErrUnauthorized
		}
		st, err := q.GetStaff(ctx, rt.StaffID)
		if err != nil {
			return err
		}
		if !st.IsActive {
			return apperr.ErrUnauthorized
		}
		sess, err = s.issue(ctx, q, st)
		return err
	})
	return sess, err
}

func (s *Service) Logout(ctx context.Context, raw string) error {
	return db.New(s.db).RevokeRefreshToken(ctx, HashRefreshToken(raw))
}

func (s *Service) Me(ctx context.Context, staffID int64) (*StaffProfile, error) {
	q := db.New(s.db)
	st, err := q.GetStaff(ctx, staffID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p, err := s.profile(ctx, q, st)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Service) LoadPrincipal(ctx context.Context, staffID int64) (Principal, error) {
	q := db.New(s.db)
	st, err := q.GetStaff(ctx, staffID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, apperr.ErrUnauthorized
	}
	if err != nil {
		return Principal{}, err
	}
	if !st.IsActive {
		return Principal{}, apperr.ErrUnauthorized
	}
	ids, err := deptIDs(ctx, q, st)
	if err != nil {
		return Principal{}, err
	}
	return Principal{StaffID: st.ID, IsAdmin: st.IsAdmin, DeptIDs: ids}, nil
}

func (s *Service) issue(ctx context.Context, q *db.Queries, st db.Staff) (*Session, error) {
	access, _, err := s.tokens.IssueAccess(st.ID, st.IsAdmin)
	if err != nil {
		return nil, err
	}
	raw, hash, err := NewRefreshToken()
	if err != nil {
		return nil, err
	}
	if err := q.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
		TokenHash: hash, StaffID: st.ID, ExpiresAt: s.now().Add(s.refreshTTL),
	}); err != nil {
		return nil, err
	}
	profile, err := s.profile(ctx, q, st)
	if err != nil {
		return nil, err
	}
	return &Session{
		AccessToken: access, RefreshToken: raw,
		ExpiresIn: int(s.tokens.AccessTTL().Seconds()), Staff: profile,
	}, nil
}

func (s *Service) profile(ctx context.Context, q *db.Queries, st db.Staff) (StaffProfile, error) {
	ids, err := deptIDs(ctx, q, st)
	if err != nil {
		return StaffProfile{}, err
	}
	return StaffProfile{
		ID: st.ID, Username: st.Username, Email: st.Email, FirstName: st.FirstName,
		LastName: st.LastName, IsAdmin: st.IsAdmin, DepartmentIDs: ids,
	}, nil
}

// deptIDs returns the primary department followed by extra memberships, deduplicated.
func deptIDs(ctx context.Context, q *db.Queries, st db.Staff) ([]int64, error) {
	extra, err := q.ListStaffDepartmentIDs(ctx, st.ID)
	if err != nil {
		return nil, fmt.Errorf("staff departments: %w", err)
	}
	ids := []int64{st.PrimaryDeptID}
	for _, id := range extra {
		if id != st.PrimaryDeptID {
			ids = append(ids, id)
		}
	}
	return ids, nil
}
```

`gin/internal/auth/admin.go`:

```go
package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/jackc/pgx/v5"
)

// CreateAdmin creates an active admin in the first department. Used by the CLI.
func CreateAdmin(ctx context.Context, b db.Beginner, username, email, password string) (int64, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return 0, err
	}
	q := db.New(b)
	dept, err := q.FirstDepartment(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, errors.New("no department exists; run migrations first")
	}
	if err != nil {
		return 0, err
	}
	st, err := q.CreateStaff(ctx, db.CreateStaffParams{
		Username: username, Email: email, PasswordHash: hash,
		IsAdmin: true, IsActive: true, PrimaryDeptID: dept.ID,
	})
	if db.IsUniqueViolation(err) {
		return 0, fmt.Errorf("%w: username or email already exists", apperr.ErrConflict)
	}
	if err != nil {
		return 0, err
	}
	return st.ID, nil
}
```

- [ ] **Step 10: Run service tests**

Run: `go test ./internal/auth/ -v`
Expected: PASS (9 tests).

- [ ] **Step 11: Write the failing handler tests**

`gin/internal/auth/handler_test.go`:

```go
package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/apperr"
)

type fakeSvc struct {
	login   func(u, p string) (*Session, error)
	refresh func(raw string) (*Session, error)
	logout  func(raw string) error
	me      func(id int64) (*StaffProfile, error)
	load    func(id int64) (Principal, error)
}

func (f *fakeSvc) Login(_ context.Context, u, p string) (*Session, error) { return f.login(u, p) }
func (f *fakeSvc) Refresh(_ context.Context, raw string) (*Session, error) { return f.refresh(raw) }
func (f *fakeSvc) Logout(_ context.Context, raw string) error             { return f.logout(raw) }
func (f *fakeSvc) Me(_ context.Context, id int64) (*StaffProfile, error)  { return f.me(id) }
func (f *fakeSvc) LoadPrincipal(_ context.Context, id int64) (Principal, error) {
	return f.load(id)
}

func router(f *fakeSvc, tk *Tokens) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	public := api.Group("")
	private := api.Group("", RequireAuth(tk, f))
	NewHandler(f).Mount(public, private)
	admin := private.Group("", RequireAdmin())
	admin.GET("/admin-only", func(c *gin.Context) { c.Status(204) })
	return r
}

func call(r *gin.Engine, method, path, body, bearer string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestLoginHandler(t *testing.T) {
	f := &fakeSvc{login: func(u, p string) (*Session, error) {
		if u == "agent" && p == "password1" {
			return &Session{AccessToken: "a", RefreshToken: "r", ExpiresIn: 900}, nil
		}
		return nil, apperr.ErrUnauthorized
	}}
	r := router(f, NewTokens(secret, time.Minute))
	if w := call(r, http.MethodPost, "/api/v1/auth/login", `{"username":"agent"}`, ""); w.Code != 400 {
		t.Fatalf("missing password: %d %s", w.Code, w.Body.String())
	}
	if w := call(r, http.MethodPost, "/api/v1/auth/login", `{"username":"agent","password":"bad"}`, ""); w.Code != 401 {
		t.Fatalf("bad password: %d", w.Code)
	}
	w := call(r, http.MethodPost, "/api/v1/auth/login", `{"username":"agent","password":"password1"}`, "")
	var sess Session
	_ = json.Unmarshal(w.Body.Bytes(), &sess)
	if w.Code != 200 || sess.AccessToken != "a" {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
}

func TestRefreshAndLogoutHandlers(t *testing.T) {
	f := &fakeSvc{
		refresh: func(raw string) (*Session, error) {
			if raw == "good" {
				return &Session{AccessToken: "a2"}, nil
			}
			return nil, apperr.ErrUnauthorized
		},
		logout: func(raw string) error { return nil },
		load:   func(id int64) (Principal, error) { return Principal{StaffID: id}, nil },
	}
	tk := NewTokens(secret, time.Minute)
	r := router(f, tk)
	if w := call(r, http.MethodPost, "/api/v1/auth/refresh", `{"refresh_token":"bad"}`, ""); w.Code != 401 {
		t.Fatalf("bad refresh: %d", w.Code)
	}
	if w := call(r, http.MethodPost, "/api/v1/auth/refresh", `{"refresh_token":"good"}`, ""); w.Code != 200 {
		t.Fatalf("good refresh: %d", w.Code)
	}
	if w := call(r, http.MethodPost, "/api/v1/auth/logout", `{"refresh_token":"good"}`, ""); w.Code != 401 {
		t.Fatalf("logout without auth: %d", w.Code)
	}
	access, _, _ := tk.IssueAccess(5, false)
	if w := call(r, http.MethodPost, "/api/v1/auth/logout", `{"refresh_token":"good"}`, access); w.Code != 204 {
		t.Fatalf("logout: %d", w.Code)
	}
}

func TestAuthMiddlewareAndMe(t *testing.T) {
	f := &fakeSvc{
		me: func(id int64) (*StaffProfile, error) { return &StaffProfile{ID: id, Username: "u"}, nil },
		load: func(id int64) (Principal, error) {
			if id == 9 {
				return Principal{}, apperr.ErrUnauthorized
			}
			return Principal{StaffID: id, IsAdmin: id == 1}, nil
		},
	}
	tk := NewTokens(secret, time.Minute)
	r := router(f, tk)
	if w := call(r, http.MethodGet, "/api/v1/me", "", ""); w.Code != 401 {
		t.Fatalf("no token: %d", w.Code)
	}
	if w := call(r, http.MethodGet, "/api/v1/me", "", "garbage"); w.Code != 401 {
		t.Fatalf("bad token: %d", w.Code)
	}
	deactivated, _, _ := tk.IssueAccess(9, false)
	if w := call(r, http.MethodGet, "/api/v1/me", "", deactivated); w.Code != 401 {
		t.Fatalf("deactivated staff: %d", w.Code)
	}
	agent, _, _ := tk.IssueAccess(2, false)
	w := call(r, http.MethodGet, "/api/v1/me", "", agent)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"username":"u"`) {
		t.Fatalf("me: %d %s", w.Code, w.Body.String())
	}
	if w := call(r, http.MethodGet, "/api/v1/admin-only", "", agent); w.Code != 403 {
		t.Fatalf("agent on admin route: %d", w.Code)
	}
	admin, _, _ := tk.IssueAccess(1, true)
	if w := call(r, http.MethodGet, "/api/v1/admin-only", "", admin); w.Code != 204 {
		t.Fatalf("admin on admin route: %d", w.Code)
	}
}
```

- [ ] **Step 12: Run to verify it fails**

Run: `go test ./internal/auth/ -run Handler -v`
Expected: FAIL to compile, "undefined: NewHandler".

- [ ] **Step 13: Implement the handler**

`gin/internal/auth/handler.go`:

```go
package auth

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/httpx"
)

type SessionService interface {
	Login(ctx context.Context, username, password string) (*Session, error)
	Refresh(ctx context.Context, raw string) (*Session, error)
	Logout(ctx context.Context, raw string) error
	Me(ctx context.Context, staffID int64) (*StaffProfile, error)
}

type Handler struct{ svc SessionService }

func NewHandler(svc SessionService) *Handler { return &Handler{svc: svc} }

func (h *Handler) Mount(public, private *gin.RouterGroup) {
	public.POST("/auth/login", h.login)
	public.POST("/auth/refresh", h.refresh)
	private.POST("/auth/logout", h.logout)
	private.GET("/me", h.me)
}

type loginRequest struct {
	Username string `json:"username" binding:"required,max=64"`
	Password string `json:"password" binding:"required,max=256"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

func (h *Handler) login(c *gin.Context) {
	var in loginRequest
	if !httpx.BindJSON(c, &in) {
		return
	}
	sess, err := h.svc.Login(c.Request.Context(), in.Username, in.Password)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, sess)
}

func (h *Handler) refresh(c *gin.Context) {
	var in refreshRequest
	if !httpx.BindJSON(c, &in) {
		return
	}
	sess, err := h.svc.Refresh(c.Request.Context(), in.RefreshToken)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, sess)
}

func (h *Handler) logout(c *gin.Context) {
	var in refreshRequest
	if !httpx.BindJSON(c, &in) {
		return
	}
	if err := h.svc.Logout(c.Request.Context(), in.RefreshToken); err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) me(c *gin.Context) {
	p, ok := FromContext(c)
	if !ok {
		httpx.Fail(c, apperr.ErrUnauthorized)
		return
	}
	profile, err := h.svc.Me(c.Request.Context(), p.StaffID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, profile)
}
```

- [ ] **Step 14: Run all auth tests**

Run: `go test ./internal/auth/ -v`
Expected: PASS (12 tests).

- [ ] **Step 15: Commit**

```bash
git add gin
git commit -m "feat(gin): auth with JWT access tokens, rotating refresh tokens and middleware" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 6: Departments

**Files:**
- Modify: `gin/db/queries/dept.sql` (add queries)
- Create: `gin/internal/dept/service.go`, `gin/internal/dept/handler.go`
- Create: `gin/internal/dept/service_test.go`, `gin/internal/dept/handler_test.go`

**Interfaces:**
- Consumes: `db.*`, `auth.RequireAdmin`, `auth.WithPrincipal`, `httpx.*`, `apperr.*`.
- Produces: `dept.Department{ID int64; Name string; IsPublic bool; ManagerID *int64; CreatedAt, UpdatedAt time.Time}`; `dept.CreateInput{Name string; IsPublic *bool; ManagerID *int64}`; `dept.UpdateInput{Name *string; IsPublic *bool; ManagerID *int64}`; `dept.NewService(b db.Beginner) *Service` with `List(ctx) ([]Department, error)`, `Get(ctx, id) (*Department, error)`, `Create(ctx, in) (*Department, error)`, `Update(ctx, id, in) (*Department, error)`, `Delete(ctx, id) error`; `dept.Service` interface (same methods) consumed by `dept.NewHandler(svc) *Handler` with `Mount(private *gin.RouterGroup)`. Queries: `ListDepartments`, `CreateDepartment`, `UpdateDepartment`, `DeleteDepartment`, `CountDepartmentReferences`.

- [ ] **Step 1: Add queries and regenerate**

Append to `gin/db/queries/dept.sql`:

```sql
-- name: ListDepartments :many
SELECT * FROM department ORDER BY name;

-- name: CreateDepartment :one
INSERT INTO department (name, is_public, manager_id) VALUES ($1, $2, $3) RETURNING *;

-- name: UpdateDepartment :one
UPDATE department
SET name = COALESCE(sqlc.narg('name'), name),
    is_public = COALESCE(sqlc.narg('is_public'), is_public),
    manager_id = COALESCE(sqlc.narg('manager_id'), manager_id),
    updated_at = now()
WHERE id = @id
RETURNING *;

-- name: DeleteDepartment :execrows
DELETE FROM department WHERE id = $1;

-- name: CountDepartmentReferences :one
SELECT (
  (SELECT count(*) FROM ticket WHERE dept_id = $1) +
  (SELECT count(*) FROM staff WHERE primary_dept_id = $1) +
  (SELECT count(*) FROM staff_department WHERE dept_id = $1) +
  (SELECT count(*) FROM help_topic WHERE dept_id = $1)
)::bigint AS refs;
```

Run: `make sqlc && go build ./...`
Expected: `UpdateDepartmentParams{Name *string; IsPublic *bool; ManagerID *int64; ID int64}`, `CountDepartmentReferences(ctx, id) (int64, error)`, `DeleteDepartment(ctx, id) (int64, error)`.

- [ ] **Step 2: Write the failing service tests**

`gin/internal/dept/service_test.go`:

```go
package dept

import (
	"context"
	"errors"
	"testing"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/db/testutil"
)

func TestDepartmentCRUD(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	svc := NewService(tx)

	list, err := svc.List(ctx)
	if err != nil || len(list) != 1 || list[0].Name != "Support" {
		t.Fatalf("seeded list: %+v %v", list, err)
	}
	created, err := svc.Create(ctx, CreateInput{Name: "Billing"})
	if err != nil || created.Name != "Billing" || !created.IsPublic || created.ManagerID != nil {
		t.Fatalf("create: %+v %v", created, err)
	}
	if _, err := svc.Create(ctx, CreateInput{Name: "Billing"}); !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("duplicate name: %v", err)
	}
	bad := int64(999999)
	var ve *apperr.ValidationError
	if _, err := svc.Create(ctx, CreateInput{Name: "X", ManagerID: &bad}); !errors.As(err, &ve) || ve.Fields["manager_id"] == "" {
		t.Fatalf("unknown manager: %v", err)
	}
	st, err := db.New(tx).CreateStaff(ctx, db.CreateStaffParams{Username: "m", Email: "m@x.test", PasswordHash: "h", IsActive: true, PrimaryDeptID: created.ID})
	if err != nil {
		t.Fatal(err)
	}
	private := false
	name := "Billing & Accounts"
	updated, err := svc.Update(ctx, created.ID, UpdateInput{Name: &name, IsPublic: &private, ManagerID: &st.ID})
	if err != nil || updated.Name != name || updated.IsPublic || updated.ManagerID == nil || *updated.ManagerID != st.ID {
		t.Fatalf("update: %+v %v", updated, err)
	}
	if _, err := svc.Update(ctx, 999999, UpdateInput{Name: &name}); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("update missing: %v", err)
	}
	got, err := svc.Get(ctx, created.ID)
	if err != nil || got.Name != name {
		t.Fatalf("get: %+v %v", got, err)
	}
	if _, err := svc.Get(ctx, 999999); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("get missing: %v", err)
	}
	if err := svc.Delete(ctx, created.ID); !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("delete referenced: %v", err)
	}
	empty, _ := svc.Create(ctx, CreateInput{Name: "Empty"})
	if err := svc.Delete(ctx, empty.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := svc.Delete(ctx, empty.ID); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./internal/dept/ -v`
Expected: FAIL to compile, "undefined: NewService".

- [ ] **Step 4: Implement the service**

`gin/internal/dept/service.go`:

```go
// Package dept manages departments.
package dept

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/jackc/pgx/v5"
)

type Department struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	IsPublic  bool      `json:"is_public"`
	ManagerID *int64    `json:"manager_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateInput struct {
	Name      string `json:"name" binding:"required,max=128"`
	IsPublic  *bool  `json:"is_public"`
	ManagerID *int64 `json:"manager_id"`
}

type UpdateInput struct {
	Name      *string `json:"name" binding:"omitempty,min=1,max=128"`
	IsPublic  *bool   `json:"is_public"`
	ManagerID *int64  `json:"manager_id"`
}

type Service interface {
	List(ctx context.Context) ([]Department, error)
	Get(ctx context.Context, id int64) (*Department, error)
	Create(ctx context.Context, in CreateInput) (*Department, error)
	Update(ctx context.Context, id int64, in UpdateInput) (*Department, error)
	Delete(ctx context.Context, id int64) error
}

type service struct{ db db.Beginner }

func NewService(b db.Beginner) Service { return &service{db: b} }

func fromRow(d db.Department) Department {
	return Department{ID: d.ID, Name: d.Name, IsPublic: d.IsPublic, ManagerID: d.ManagerID,
		CreatedAt: d.CreatedAt.UTC(), UpdatedAt: d.UpdatedAt.UTC()}
}

func (s *service) List(ctx context.Context) ([]Department, error) {
	rows, err := db.New(s.db).ListDepartments(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Department, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromRow(r))
	}
	return out, nil
}

func (s *service) Get(ctx context.Context, id int64) (*Department, error) {
	d, err := db.New(s.db).GetDepartment(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("department %d: %w", id, apperr.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	out := fromRow(d)
	return &out, nil
}

func (s *service) Create(ctx context.Context, in CreateInput) (*Department, error) {
	q := db.New(s.db)
	if err := checkManager(ctx, q, in.ManagerID); err != nil {
		return nil, err
	}
	isPublic := true
	if in.IsPublic != nil {
		isPublic = *in.IsPublic
	}
	d, err := q.CreateDepartment(ctx, db.CreateDepartmentParams{Name: in.Name, IsPublic: isPublic, ManagerID: in.ManagerID})
	if db.IsUniqueViolation(err) {
		return nil, fmt.Errorf("%w: department name already exists", apperr.ErrConflict)
	}
	if err != nil {
		return nil, err
	}
	out := fromRow(d)
	return &out, nil
}

func (s *service) Update(ctx context.Context, id int64, in UpdateInput) (*Department, error) {
	q := db.New(s.db)
	if err := checkManager(ctx, q, in.ManagerID); err != nil {
		return nil, err
	}
	d, err := q.UpdateDepartment(ctx, db.UpdateDepartmentParams{ID: id, Name: in.Name, IsPublic: in.IsPublic, ManagerID: in.ManagerID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("department %d: %w", id, apperr.ErrNotFound)
	}
	if db.IsUniqueViolation(err) {
		return nil, fmt.Errorf("%w: department name already exists", apperr.ErrConflict)
	}
	if err != nil {
		return nil, err
	}
	out := fromRow(d)
	return &out, nil
}

func (s *service) Delete(ctx context.Context, id int64) error {
	return db.WithTx(ctx, s.db, func(q *db.Queries) error {
		refs, err := q.CountDepartmentReferences(ctx, id)
		if err != nil {
			return err
		}
		if refs > 0 {
			return fmt.Errorf("%w: department is referenced by tickets, staff or topics", apperr.ErrConflict)
		}
		n, err := q.DeleteDepartment(ctx, id)
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("department %d: %w", id, apperr.ErrNotFound)
		}
		return nil
	})
}

func checkManager(ctx context.Context, q *db.Queries, id *int64) error {
	if id == nil {
		return nil
	}
	_, err := q.GetStaff(ctx, *id)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.Validation("manager_id", "unknown staff")
	}
	return err
}
```

- [ ] **Step 5: Run service tests**

Run: `go test ./internal/dept/ -run CRUD -v`
Expected: PASS.

- [ ] **Step 6: Write the failing handler tests**

`gin/internal/dept/handler_test.go`:

```go
package dept

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
)

type fake struct {
	list   func() ([]Department, error)
	get    func(id int64) (*Department, error)
	create func(in CreateInput) (*Department, error)
	update func(id int64, in UpdateInput) (*Department, error)
	del    func(id int64) error
}

func (f *fake) List(context.Context) ([]Department, error)               { return f.list() }
func (f *fake) Get(_ context.Context, id int64) (*Department, error)     { return f.get(id) }
func (f *fake) Create(_ context.Context, in CreateInput) (*Department, error) { return f.create(in) }
func (f *fake) Update(_ context.Context, id int64, in UpdateInput) (*Department, error) {
	return f.update(id, in)
}
func (f *fake) Delete(_ context.Context, id int64) error { return f.del(id) }

func router(f *fake, admin bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	private := r.Group("/api/v1", func(c *gin.Context) {
		auth.WithPrincipal(c, auth.Principal{StaffID: 1, IsAdmin: admin})
	})
	NewHandler(f).Mount(private)
	return r
}

func call(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestDepartmentRoutes(t *testing.T) {
	f := &fake{
		list:   func() ([]Department, error) { return []Department{{ID: 1, Name: "Support"}}, nil },
		get:    func(id int64) (*Department, error) { return nil, apperr.ErrNotFound },
		create: func(in CreateInput) (*Department, error) { return &Department{ID: 2, Name: in.Name}, nil },
		update: func(id int64, in UpdateInput) (*Department, error) { return &Department{ID: id, Name: *in.Name}, nil },
		del:    func(id int64) error { return apperr.ErrConflict },
	}
	admin := router(f, true)
	if w := call(admin, http.MethodGet, "/api/v1/departments", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "Support") {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	if w := call(admin, http.MethodGet, "/api/v1/departments/7", ""); w.Code != 404 {
		t.Fatalf("get missing: %d", w.Code)
	}
	if w := call(admin, http.MethodGet, "/api/v1/departments/x", ""); w.Code != 400 {
		t.Fatalf("bad id: %d", w.Code)
	}
	if w := call(admin, http.MethodPost, "/api/v1/departments", `{}`); w.Code != 400 {
		t.Fatalf("create missing name: %d", w.Code)
	}
	if w := call(admin, http.MethodPost, "/api/v1/departments", `{"name":"Billing"}`); w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if w := call(admin, http.MethodPatch, "/api/v1/departments/2", `{"name":"B2"}`); w.Code != 200 {
		t.Fatalf("update: %d", w.Code)
	}
	if w := call(admin, http.MethodDelete, "/api/v1/departments/2", ""); w.Code != 409 {
		t.Fatalf("delete conflict: %d", w.Code)
	}
	agent := router(f, false)
	if w := call(agent, http.MethodGet, "/api/v1/departments", ""); w.Code != 200 {
		t.Fatalf("agent list: %d", w.Code)
	}
	if w := call(agent, http.MethodPost, "/api/v1/departments", `{"name":"Nope"}`); w.Code != 403 {
		t.Fatalf("agent create: %d", w.Code)
	}
	if w := call(agent, http.MethodDelete, "/api/v1/departments/2", ""); w.Code != 403 {
		t.Fatalf("agent delete: %d", w.Code)
	}
}
```

- [ ] **Step 7: Implement the handler**

`gin/internal/dept/handler.go`:

```go
package dept

import (
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
	var in UpdateInput
	if !httpx.BindJSON(c, &in) {
		return
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
```

- [ ] **Step 8: Run all dept tests**

Run: `go test ./internal/dept/ -v`
Expected: PASS (2 tests).

- [ ] **Step 9: Commit**

```bash
git add gin
git commit -m "feat(gin): department CRUD with admin-only writes" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 7: Help topics

**Files:**
- Create: `gin/db/queries/topic.sql`
- Create: `gin/internal/topic/service.go`, `gin/internal/topic/handler.go`
- Create: `gin/internal/topic/service_test.go`, `gin/internal/topic/handler_test.go`

**Interfaces:**
- Consumes: `db.GetDepartment`, `db.GetPriority`, `auth.RequireAdmin`, `httpx.*`, `apperr.*`.
- Produces: `topic.Topic{ID int64; Name string; DeptID *int64; PriorityID *int64; IsActive bool; SortOrder int32; CreatedAt, UpdatedAt time.Time}`; `topic.CreateInput{Name string; DeptID *int64; PriorityID *int64; IsActive *bool; SortOrder *int32}`; `topic.UpdateInput` (all pointer fields); `topic.Service` interface with `List`, `Get`, `Create`, `Update`, `Delete` (same shapes as dept); `topic.NewService(b)`, `topic.NewHandler(svc)`, `Mount(private)`. Queries: `ListTopics`, `GetTopic`, `CreateTopic`, `UpdateTopic`, `DeleteTopic`, `CountTopicReferences`.

- [ ] **Step 1: Add queries and regenerate**

`gin/db/queries/topic.sql`:

```sql
-- name: ListTopics :many
SELECT * FROM help_topic ORDER BY sort_order, name;

-- name: GetTopic :one
SELECT * FROM help_topic WHERE id = $1;

-- name: CreateTopic :one
INSERT INTO help_topic (name, dept_id, priority_id, is_active, sort_order)
VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: UpdateTopic :one
UPDATE help_topic
SET name = COALESCE(sqlc.narg('name'), name),
    dept_id = COALESCE(sqlc.narg('dept_id'), dept_id),
    priority_id = COALESCE(sqlc.narg('priority_id'), priority_id),
    is_active = COALESCE(sqlc.narg('is_active'), is_active),
    sort_order = COALESCE(sqlc.narg('sort_order'), sort_order),
    updated_at = now()
WHERE id = @id
RETURNING *;

-- name: DeleteTopic :execrows
DELETE FROM help_topic WHERE id = $1;

-- name: CountTopicReferences :one
SELECT count(*)::bigint FROM ticket WHERE topic_id = $1;
```

Run: `make sqlc && go build ./...`

- [ ] **Step 2: Write the failing service test**

`gin/internal/topic/service_test.go`:

```go
package topic

import (
	"context"
	"errors"
	"testing"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/db/testutil"
)

func TestTopicCRUD(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	svc := NewService(tx)
	q := db.New(tx)

	list, err := svc.List(ctx)
	if err != nil || len(list) != 1 || list[0].Name != "General Inquiry" || list[0].DeptID == nil {
		t.Fatalf("seeded: %+v %v", list, err)
	}
	dept, _ := q.FirstDepartment(ctx)
	prio, _ := q.DefaultPriority(ctx)
	bad := int64(999999)
	var ve *apperr.ValidationError
	if _, err := svc.Create(ctx, CreateInput{Name: "X", DeptID: &bad}); !errors.As(err, &ve) || ve.Fields["dept_id"] == "" {
		t.Fatalf("unknown dept: %v", err)
	}
	if _, err := svc.Create(ctx, CreateInput{Name: "X", PriorityID: &bad}); !errors.As(err, &ve) || ve.Fields["priority_id"] == "" {
		t.Fatalf("unknown priority: %v", err)
	}
	created, err := svc.Create(ctx, CreateInput{Name: "Refunds", DeptID: &dept.ID, PriorityID: &prio.ID})
	if err != nil || !created.IsActive || created.SortOrder != 0 {
		t.Fatalf("create: %+v %v", created, err)
	}
	inactive := false
	order := int32(5)
	updated, err := svc.Update(ctx, created.ID, UpdateInput{IsActive: &inactive, SortOrder: &order})
	if err != nil || updated.IsActive || updated.SortOrder != 5 || updated.Name != "Refunds" {
		t.Fatalf("update: %+v %v", updated, err)
	}
	if _, err := svc.Get(ctx, bad); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("get missing: %v", err)
	}
	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := svc.Delete(ctx, created.ID); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
	// referenced topic cannot be deleted
	status, _ := q.DefaultStatus(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO ticket (number, subject, status_id, dept_id, topic_id, priority_id, requester_email)
		VALUES ('000001', 's', $1, $2, $3, $4, 'r@x.test')`, status.ID, dept.ID, list[0].ID, prio.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, list[0].ID); !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("delete referenced: %v", err)
	}
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./internal/topic/ -v`
Expected: FAIL to compile.

- [ ] **Step 4: Implement the service**

`gin/internal/topic/service.go`:

```go
// Package topic manages help topics.
package topic

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/jackc/pgx/v5"
)

type Topic struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	DeptID     *int64    `json:"dept_id"`
	PriorityID *int64    `json:"priority_id"`
	IsActive   bool      `json:"is_active"`
	SortOrder  int32     `json:"sort_order"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type CreateInput struct {
	Name       string `json:"name" binding:"required,max=128"`
	DeptID     *int64 `json:"dept_id"`
	PriorityID *int64 `json:"priority_id"`
	IsActive   *bool  `json:"is_active"`
	SortOrder  *int32 `json:"sort_order"`
}

type UpdateInput struct {
	Name       *string `json:"name" binding:"omitempty,min=1,max=128"`
	DeptID     *int64  `json:"dept_id"`
	PriorityID *int64  `json:"priority_id"`
	IsActive   *bool   `json:"is_active"`
	SortOrder  *int32  `json:"sort_order"`
}

type Service interface {
	List(ctx context.Context) ([]Topic, error)
	Get(ctx context.Context, id int64) (*Topic, error)
	Create(ctx context.Context, in CreateInput) (*Topic, error)
	Update(ctx context.Context, id int64, in UpdateInput) (*Topic, error)
	Delete(ctx context.Context, id int64) error
}

type service struct{ db db.Beginner }

func NewService(b db.Beginner) Service { return &service{db: b} }

func fromRow(r db.HelpTopic) Topic {
	return Topic{ID: r.ID, Name: r.Name, DeptID: r.DeptID, PriorityID: r.PriorityID, IsActive: r.IsActive,
		SortOrder: r.SortOrder, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC()}
}

func (s *service) List(ctx context.Context) ([]Topic, error) {
	rows, err := db.New(s.db).ListTopics(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Topic, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromRow(r))
	}
	return out, nil
}

func (s *service) Get(ctx context.Context, id int64) (*Topic, error) {
	r, err := db.New(s.db).GetTopic(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("topic %d: %w", id, apperr.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	out := fromRow(r)
	return &out, nil
}

func (s *service) Create(ctx context.Context, in CreateInput) (*Topic, error) {
	q := db.New(s.db)
	if err := checkRefs(ctx, q, in.DeptID, in.PriorityID); err != nil {
		return nil, err
	}
	isActive, sortOrder := true, int32(0)
	if in.IsActive != nil {
		isActive = *in.IsActive
	}
	if in.SortOrder != nil {
		sortOrder = *in.SortOrder
	}
	r, err := q.CreateTopic(ctx, db.CreateTopicParams{Name: in.Name, DeptID: in.DeptID, PriorityID: in.PriorityID, IsActive: isActive, SortOrder: sortOrder})
	if err != nil {
		return nil, err
	}
	out := fromRow(r)
	return &out, nil
}

func (s *service) Update(ctx context.Context, id int64, in UpdateInput) (*Topic, error) {
	q := db.New(s.db)
	if err := checkRefs(ctx, q, in.DeptID, in.PriorityID); err != nil {
		return nil, err
	}
	r, err := q.UpdateTopic(ctx, db.UpdateTopicParams{ID: id, Name: in.Name, DeptID: in.DeptID, PriorityID: in.PriorityID, IsActive: in.IsActive, SortOrder: in.SortOrder})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("topic %d: %w", id, apperr.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	out := fromRow(r)
	return &out, nil
}

func (s *service) Delete(ctx context.Context, id int64) error {
	return db.WithTx(ctx, s.db, func(q *db.Queries) error {
		refs, err := q.CountTopicReferences(ctx, id)
		if err != nil {
			return err
		}
		if refs > 0 {
			return fmt.Errorf("%w: topic is referenced by tickets", apperr.ErrConflict)
		}
		n, err := q.DeleteTopic(ctx, id)
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("topic %d: %w", id, apperr.ErrNotFound)
		}
		return nil
	})
}

func checkRefs(ctx context.Context, q *db.Queries, deptID, priorityID *int64) error {
	fields := map[string]string{}
	if deptID != nil {
		if _, err := q.GetDepartment(ctx, *deptID); errors.Is(err, pgx.ErrNoRows) {
			fields["dept_id"] = "unknown department"
		} else if err != nil {
			return err
		}
	}
	if priorityID != nil {
		if _, err := q.GetPriority(ctx, *priorityID); errors.Is(err, pgx.ErrNoRows) {
			fields["priority_id"] = "unknown priority"
		} else if err != nil {
			return err
		}
	}
	if len(fields) > 0 {
		return &apperr.ValidationError{Fields: fields}
	}
	return nil
}
```

- [ ] **Step 5: Handler test and handler**

`gin/internal/topic/handler_test.go`:

```go
package topic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
)

type fake struct{ topics []Topic }

func (f *fake) List(context.Context) ([]Topic, error)           { return f.topics, nil }
func (f *fake) Get(_ context.Context, id int64) (*Topic, error) { return nil, apperr.ErrNotFound }
func (f *fake) Create(_ context.Context, in CreateInput) (*Topic, error) {
	return &Topic{ID: 9, Name: in.Name}, nil
}
func (f *fake) Update(_ context.Context, id int64, in UpdateInput) (*Topic, error) {
	return &Topic{ID: id, Name: "u"}, nil
}
func (f *fake) Delete(_ context.Context, id int64) error { return nil }

func router(admin bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	private := r.Group("/api/v1", func(c *gin.Context) {
		auth.WithPrincipal(c, auth.Principal{StaffID: 1, IsAdmin: admin})
	})
	NewHandler(&fake{topics: []Topic{{ID: 1, Name: "General"}}}).Mount(private)
	return r
}

func call(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestTopicRoutes(t *testing.T) {
	admin, agent := router(true), router(false)
	if w := call(agent, http.MethodGet, "/api/v1/topics", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "General") {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	if w := call(agent, http.MethodGet, "/api/v1/topics/3", ""); w.Code != 404 {
		t.Fatalf("get: %d", w.Code)
	}
	if w := call(agent, http.MethodPost, "/api/v1/topics", `{"name":"X"}`); w.Code != 403 {
		t.Fatalf("agent create: %d", w.Code)
	}
	if w := call(admin, http.MethodPost, "/api/v1/topics", `{"name":""}`); w.Code != 400 {
		t.Fatalf("empty name: %d", w.Code)
	}
	if w := call(admin, http.MethodPost, "/api/v1/topics", `{"name":"X"}`); w.Code != 201 {
		t.Fatalf("create: %d", w.Code)
	}
	if w := call(admin, http.MethodPatch, "/api/v1/topics/1", `{"sort_order":2}`); w.Code != 200 {
		t.Fatalf("update: %d", w.Code)
	}
	if w := call(admin, http.MethodDelete, "/api/v1/topics/1", ""); w.Code != 204 {
		t.Fatalf("delete: %d", w.Code)
	}
}
```

`gin/internal/topic/handler.go`: identical structure to `dept/handler.go` with `/topics` paths and `Topic` types. Write it out in full:

```go
package topic

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/httpx"
)

type Handler struct{ svc Service }

func NewHandler(svc Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Mount(private *gin.RouterGroup) {
	private.GET("/topics", h.list)
	private.GET("/topics/:id", h.get)
	admin := private.Group("", auth.RequireAdmin())
	admin.POST("/topics", h.create)
	admin.PATCH("/topics/:id", h.update)
	admin.DELETE("/topics/:id", h.delete)
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
	var in UpdateInput
	if !httpx.BindJSON(c, &in) {
		return
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
```

- [ ] **Step 6: Run and commit**

Run: `go test ./internal/topic/ -v`
Expected: PASS (2 tests).

```bash
git add gin
git commit -m "feat(gin): help topic CRUD" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 8: Staff management

**Files:**
- Modify: `gin/db/queries/staff.sql` (add queries)
- Create: `gin/internal/staff/service.go`, `gin/internal/staff/handler.go`
- Create: `gin/internal/staff/service_test.go`, `gin/internal/staff/handler_test.go`

**Interfaces:**
- Consumes: `auth.HashPassword`, `db.RevokeStaffRefreshTokens`, `db.GetDepartment`, `auth.RequireAdmin`.
- Produces: `staff.Staff{ID int64; Username, Email, FirstName, LastName string; IsAdmin, IsActive bool; PrimaryDeptID int64; DepartmentIDs []int64; CreatedAt, UpdatedAt time.Time}`; `staff.CreateInput{Username, Email, Password, FirstName, LastName string; IsAdmin bool; PrimaryDeptID int64; DepartmentIDs []int64}`; `staff.UpdateInput{Email, FirstName, LastName *string; IsAdmin, IsActive *bool; PrimaryDeptID *int64; DepartmentIDs *[]int64}`; `staff.Service` interface `List(ctx) ([]Staff, error)`, `Get(ctx, id) (*Staff, error)`, `Create(ctx, in) (*Staff, error)`, `Update(ctx, id, in) (*Staff, error)`, `SetPassword(ctx, id, password string) error`; `staff.NewService(b)`, `staff.NewHandler(svc)`, `Mount(private)`. Queries: `ListStaff`, `UpdateStaff`, `SetStaffPassword`, `DeleteStaffDepartments`, `StaffCanSeeDept`.

- [ ] **Step 1: Add queries and regenerate**

Append to `gin/db/queries/staff.sql`:

```sql
-- name: ListStaff :many
SELECT * FROM staff ORDER BY username;

-- name: UpdateStaff :one
UPDATE staff
SET email = COALESCE(sqlc.narg('email'), email),
    first_name = COALESCE(sqlc.narg('first_name'), first_name),
    last_name = COALESCE(sqlc.narg('last_name'), last_name),
    is_admin = COALESCE(sqlc.narg('is_admin'), is_admin),
    is_active = COALESCE(sqlc.narg('is_active'), is_active),
    primary_dept_id = COALESCE(sqlc.narg('primary_dept_id'), primary_dept_id),
    updated_at = now()
WHERE id = @id
RETURNING *;

-- name: SetStaffPassword :execrows
UPDATE staff SET password_hash = $2, updated_at = now() WHERE id = $1;

-- name: DeleteStaffDepartments :exec
DELETE FROM staff_department WHERE staff_id = $1;

-- name: StaffCanSeeDept :one
SELECT EXISTS (
  SELECT 1 FROM staff s
  LEFT JOIN staff_department sd ON sd.staff_id = s.id AND sd.dept_id = @dept_id
  WHERE s.id = @staff_id AND (s.is_admin OR s.primary_dept_id = @dept_id OR sd.dept_id IS NOT NULL)
)::boolean AS can_see;
```

Run: `make sqlc && go build ./...`
Expected: `StaffCanSeeDept(ctx, StaffCanSeeDeptParams{DeptID, StaffID}) (bool, error)`.

- [ ] **Step 2: Write the failing service tests**

`gin/internal/staff/service_test.go`:

```go
package staff

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/db/testutil"
)

func TestStaffLifecycle(t *testing.T) {
	ctx := context.Background()
	tx := testutil.Tx(t)
	q := db.New(tx)
	svc := NewService(tx)
	dept, _ := q.FirstDepartment(ctx)
	var billingID int64
	if err := tx.QueryRow(ctx, `INSERT INTO department (name) VALUES ('Billing') RETURNING id`).Scan(&billingID); err != nil {
		t.Fatal(err)
	}

	var ve *apperr.ValidationError
	if _, err := svc.Create(ctx, CreateInput{Username: "a", Email: "a@x.test", Password: "short", PrimaryDeptID: dept.ID}); !errors.As(err, &ve) || ve.Fields["password"] == "" {
		t.Fatalf("short password: %v", err)
	}
	if _, err := svc.Create(ctx, CreateInput{Username: "a", Email: "a@x.test", Password: "password1", PrimaryDeptID: 999999}); !errors.As(err, &ve) || ve.Fields["primary_dept_id"] == "" {
		t.Fatalf("bad dept: %v", err)
	}
	created, err := svc.Create(ctx, CreateInput{
		Username: "ann", Email: "ann@x.test", Password: "password1", FirstName: "Ann",
		PrimaryDeptID: dept.ID, DepartmentIDs: []int64{billingID, dept.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !created.IsActive || created.IsAdmin || len(created.DepartmentIDs) != 2 || created.DepartmentIDs[0] != dept.ID {
		t.Fatalf("created: %+v", created)
	}
	if _, err := svc.Create(ctx, CreateInput{Username: "ann", Email: "other@x.test", Password: "password1", PrimaryDeptID: dept.ID}); !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("duplicate username: %v", err)
	}

	list, err := svc.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %+v %v", list, err)
	}

	// login to get a refresh token, then deactivate: tokens must be revoked
	authSvc := auth.NewService(tx, auth.NewTokens("0123456789abcdef0123456789abcdef", time.Minute), time.Hour)
	sess, err := authSvc.Login(ctx, "ann", "password1")
	if err != nil {
		t.Fatal(err)
	}
	inactive := false
	only := []int64{billingID}
	updated, err := svc.Update(ctx, created.ID, UpdateInput{IsActive: &inactive, DepartmentIDs: &only})
	if err != nil || updated.IsActive || len(updated.DepartmentIDs) != 2 {
		t.Fatalf("update: %+v %v", updated, err)
	}
	if _, err := authSvc.Refresh(ctx, sess.RefreshToken); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatalf("refresh after deactivation must fail: %v", err)
	}
	if _, err := svc.Update(ctx, 999999, UpdateInput{IsActive: &inactive}); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("update missing: %v", err)
	}

	if err := svc.SetPassword(ctx, created.ID, "newpassword1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetPassword(ctx, 999999, "newpassword1"); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("set password missing: %v", err)
	}
	active := true
	if _, err := svc.Update(ctx, created.ID, UpdateInput{IsActive: &active}); err != nil {
		t.Fatal(err)
	}
	if _, err := authSvc.Login(ctx, "ann", "newpassword1"); err != nil {
		t.Fatalf("login with new password: %v", err)
	}
	got, err := svc.Get(ctx, created.ID)
	if err != nil || got.Username != "ann" {
		t.Fatalf("get: %+v %v", got, err)
	}
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./internal/staff/ -v`
Expected: FAIL to compile.

- [ ] **Step 4: Implement the service**

`gin/internal/staff/service.go`:

```go
// Package staff manages agent accounts.
package staff

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/jackc/pgx/v5"
)

type Staff struct {
	ID            int64     `json:"id"`
	Username      string    `json:"username"`
	Email         string    `json:"email"`
	FirstName     string    `json:"first_name"`
	LastName      string    `json:"last_name"`
	IsAdmin       bool      `json:"is_admin"`
	IsActive      bool      `json:"is_active"`
	PrimaryDeptID int64     `json:"primary_dept_id"`
	DepartmentIDs []int64   `json:"department_ids"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type CreateInput struct {
	Username      string  `json:"username" binding:"required,max=64"`
	Email         string  `json:"email" binding:"required,email,max=255"`
	Password      string  `json:"password" binding:"required"`
	FirstName     string  `json:"first_name" binding:"max=64"`
	LastName      string  `json:"last_name" binding:"max=64"`
	IsAdmin       bool    `json:"is_admin"`
	PrimaryDeptID int64   `json:"primary_dept_id" binding:"required"`
	DepartmentIDs []int64 `json:"department_ids"`
}

type UpdateInput struct {
	Email         *string  `json:"email" binding:"omitempty,email,max=255"`
	FirstName     *string  `json:"first_name" binding:"omitempty,max=64"`
	LastName      *string  `json:"last_name" binding:"omitempty,max=64"`
	IsAdmin       *bool    `json:"is_admin"`
	IsActive      *bool    `json:"is_active"`
	PrimaryDeptID *int64   `json:"primary_dept_id"`
	DepartmentIDs *[]int64 `json:"department_ids"`
}

type Service interface {
	List(ctx context.Context) ([]Staff, error)
	Get(ctx context.Context, id int64) (*Staff, error)
	Create(ctx context.Context, in CreateInput) (*Staff, error)
	Update(ctx context.Context, id int64, in UpdateInput) (*Staff, error)
	SetPassword(ctx context.Context, id int64, password string) error
}

type service struct{ db db.Beginner }

func NewService(b db.Beginner) Service { return &service{db: b} }

func (s *service) List(ctx context.Context) ([]Staff, error) {
	q := db.New(s.db)
	rows, err := q.ListStaff(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Staff, 0, len(rows))
	for _, r := range rows {
		st, err := build(ctx, q, r)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, nil
}

func (s *service) Get(ctx context.Context, id int64) (*Staff, error) {
	q := db.New(s.db)
	r, err := q.GetStaff(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("staff %d: %w", id, apperr.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	st, err := build(ctx, q, r)
	if err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *service) Create(ctx context.Context, in CreateInput) (*Staff, error) {
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	var out Staff
	err = db.WithTx(ctx, s.db, func(q *db.Queries) error {
		if err := checkDepts(ctx, q, &in.PrimaryDeptID, in.DepartmentIDs); err != nil {
			return err
		}
		r, err := q.CreateStaff(ctx, db.CreateStaffParams{
			Username: in.Username, Email: in.Email, PasswordHash: hash, FirstName: in.FirstName,
			LastName: in.LastName, IsAdmin: in.IsAdmin, IsActive: true, PrimaryDeptID: in.PrimaryDeptID,
		})
		if db.IsUniqueViolation(err) {
			return fmt.Errorf("%w: username or email already exists", apperr.ErrConflict)
		}
		if err != nil {
			return err
		}
		if err := replaceDepts(ctx, q, r.ID, in.DepartmentIDs); err != nil {
			return err
		}
		out, err = build(ctx, q, r)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *service) Update(ctx context.Context, id int64, in UpdateInput) (*Staff, error) {
	var out Staff
	err := db.WithTx(ctx, s.db, func(q *db.Queries) error {
		var extra []int64
		if in.DepartmentIDs != nil {
			extra = *in.DepartmentIDs
		}
		if err := checkDepts(ctx, q, in.PrimaryDeptID, extra); err != nil {
			return err
		}
		r, err := q.UpdateStaff(ctx, db.UpdateStaffParams{
			ID: id, Email: in.Email, FirstName: in.FirstName, LastName: in.LastName,
			IsAdmin: in.IsAdmin, IsActive: in.IsActive, PrimaryDeptID: in.PrimaryDeptID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("staff %d: %w", id, apperr.ErrNotFound)
		}
		if db.IsUniqueViolation(err) {
			return fmt.Errorf("%w: email already exists", apperr.ErrConflict)
		}
		if err != nil {
			return err
		}
		if in.DepartmentIDs != nil {
			if err := replaceDepts(ctx, q, id, *in.DepartmentIDs); err != nil {
				return err
			}
		}
		if in.IsActive != nil && !*in.IsActive {
			if err := q.RevokeStaffRefreshTokens(ctx, id); err != nil {
				return err
			}
		}
		out, err = build(ctx, q, r)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *service) SetPassword(ctx context.Context, id int64, password string) error {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	n, err := db.New(s.db).SetStaffPassword(ctx, db.SetStaffPasswordParams{ID: id, PasswordHash: hash})
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("staff %d: %w", id, apperr.ErrNotFound)
	}
	return nil
}

func build(ctx context.Context, q *db.Queries, r db.Staff) (Staff, error) {
	extra, err := q.ListStaffDepartmentIDs(ctx, r.ID)
	if err != nil {
		return Staff{}, err
	}
	ids := []int64{r.PrimaryDeptID}
	for _, id := range extra {
		if id != r.PrimaryDeptID {
			ids = append(ids, id)
		}
	}
	return Staff{
		ID: r.ID, Username: r.Username, Email: r.Email, FirstName: r.FirstName, LastName: r.LastName,
		IsAdmin: r.IsAdmin, IsActive: r.IsActive, PrimaryDeptID: r.PrimaryDeptID, DepartmentIDs: ids,
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}, nil
}

func checkDepts(ctx context.Context, q *db.Queries, primary *int64, extra []int64) error {
	if primary != nil {
		if _, err := q.GetDepartment(ctx, *primary); errors.Is(err, pgx.ErrNoRows) {
			return apperr.Validation("primary_dept_id", "unknown department")
		} else if err != nil {
			return err
		}
	}
	for _, id := range extra {
		if _, err := q.GetDepartment(ctx, id); errors.Is(err, pgx.ErrNoRows) {
			return apperr.Validation("department_ids", fmt.Sprintf("unknown department %d", id))
		} else if err != nil {
			return err
		}
	}
	return nil
}

func replaceDepts(ctx context.Context, q *db.Queries, staffID int64, ids []int64) error {
	if err := q.DeleteStaffDepartments(ctx, staffID); err != nil {
		return err
	}
	for _, id := range ids {
		if err := q.AddStaffDepartment(ctx, db.AddStaffDepartmentParams{StaffID: staffID, DeptID: id}); err != nil {
			return err
		}
	}
	return nil
}
```

Note `checkDepts` returns the first problem rather than collecting all; that matches the single-field `apperr.Validation` helper and keeps the loop simple.

- [ ] **Step 5: Run service tests**

Run: `go test ./internal/staff/ -run Lifecycle -v`
Expected: PASS.

- [ ] **Step 6: Handler test and handler**

`gin/internal/staff/handler_test.go`:

```go
package staff

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
)

type fake struct{}

func (fake) List(context.Context) ([]Staff, error) { return []Staff{{ID: 1, Username: "ann"}}, nil }
func (fake) Get(_ context.Context, id int64) (*Staff, error) {
	if id == 1 {
		return &Staff{ID: 1, Username: "ann"}, nil
	}
	return nil, apperr.ErrNotFound
}
func (fake) Create(_ context.Context, in CreateInput) (*Staff, error) {
	return &Staff{ID: 2, Username: in.Username}, nil
}
func (fake) Update(_ context.Context, id int64, in UpdateInput) (*Staff, error) {
	return &Staff{ID: id}, nil
}
func (fake) SetPassword(_ context.Context, id int64, pw string) error {
	if len(pw) < 8 {
		return apperr.Validation("password", "too short")
	}
	return nil
}

func router(admin bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	private := r.Group("/api/v1", func(c *gin.Context) {
		auth.WithPrincipal(c, auth.Principal{StaffID: 1, IsAdmin: admin})
	})
	NewHandler(fake{}).Mount(private)
	return r
}

func call(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestStaffRoutes(t *testing.T) {
	admin, agent := router(true), router(false)
	if w := call(agent, http.MethodGet, "/api/v1/staff", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "ann") {
		t.Fatalf("agent list: %d %s", w.Code, w.Body.String())
	}
	if w := call(agent, http.MethodGet, "/api/v1/staff/1", ""); w.Code != 200 {
		t.Fatalf("agent get: %d", w.Code)
	}
	if w := call(agent, http.MethodPost, "/api/v1/staff", `{}`); w.Code != 403 {
		t.Fatalf("agent create: %d", w.Code)
	}
	if w := call(admin, http.MethodPost, "/api/v1/staff", `{"username":"bob","email":"not-an-email","password":"password1","primary_dept_id":1}`); w.Code != 400 {
		t.Fatalf("bad email: %d %s", w.Code, w.Body.String())
	}
	if w := call(admin, http.MethodPost, "/api/v1/staff", `{"username":"bob","email":"bob@x.test","password":"password1","primary_dept_id":1}`); w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if w := call(admin, http.MethodPatch, "/api/v1/staff/2", `{"is_active":false}`); w.Code != 200 {
		t.Fatalf("update: %d", w.Code)
	}
	if w := call(admin, http.MethodPost, "/api/v1/staff/2/password", `{"password":"short"}`); w.Code != 400 {
		t.Fatalf("short password: %d", w.Code)
	}
	if w := call(admin, http.MethodPost, "/api/v1/staff/2/password", `{"password":"longenough"}`); w.Code != 204 {
		t.Fatalf("set password: %d", w.Code)
	}
	if w := call(admin, http.MethodGet, "/api/v1/staff/9", ""); w.Code != 404 {
		t.Fatalf("get missing: %d", w.Code)
	}
}
```

`gin/internal/staff/handler.go`:

```go
package staff

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/httpx"
)

type Handler struct{ svc Service }

func NewHandler(svc Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Mount(private *gin.RouterGroup) {
	private.GET("/staff", h.list)
	private.GET("/staff/:id", h.get)
	admin := private.Group("", auth.RequireAdmin())
	admin.POST("/staff", h.create)
	admin.PATCH("/staff/:id", h.update)
	admin.POST("/staff/:id/password", h.setPassword)
}

type passwordRequest struct {
	Password string `json:"password" binding:"required"`
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
	var in UpdateInput
	if !httpx.BindJSON(c, &in) {
		return
	}
	out, err := h.svc.Update(c.Request.Context(), id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) setPassword(c *gin.Context) {
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in passwordRequest
	if !httpx.BindJSON(c, &in) {
		return
	}
	if err := h.svc.SetPassword(c.Request.Context(), id, in.Password); err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
```

- [ ] **Step 7: Run and commit**

Run: `go test ./internal/staff/ -v`
Expected: PASS (2 tests).

```bash
git add gin
git commit -m "feat(gin): staff management with department membership and token revocation" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 9: Ticket core (create, get, list, update, priorities, statuses)

**Files:**
- Create: `gin/db/queries/ticket.sql`
- Create: `gin/internal/ticket/types.go`, `gin/internal/ticket/service.go`, `gin/internal/ticket/handler.go`
- Create: `gin/internal/ticket/fixture_test.go`, `gin/internal/ticket/service_test.go`, `gin/internal/ticket/handler_test.go`

**Interfaces:**
- Consumes: `auth.Principal`, `auth.FromContext`, `httpx.*`, `apperr.*`, queries `GetTopic`, `GetDepartment`, `GetPriority`, `DefaultPriority`, `DefaultStatus`, `ListPriorities`, `ListStatuses`, `GetStaff`.
- Produces (types in `types.go`): `ticket.Ref{ID int64; Name string}`; `ticket.Ticket` (fields listed in Step 3); `ticket.Priority{ID int64; Name string; Urgency int32; Color string}`; `ticket.Status{ID int64; Name string; State string; SortOrder int32}`; `ticket.CreateInput`, `ticket.UpdateInput`, `ticket.ListFilter{StatusID *int64; State string; DeptID *int64; AssignedTo string; Q string; Sort string; Page httpx.Page}`; interface `ticket.Service` with `Create(ctx, p auth.Principal, in CreateInput) (*Ticket, error)`, `Get(ctx, p, id) (*Ticket, error)`, `List(ctx, p, f ListFilter) (*httpx.List[Ticket], error)`, `Update(ctx, p, id, in UpdateInput) (*Ticket, error)`, `ListPriorities(ctx) ([]Priority, error)`, `ListStatuses(ctx) ([]Status, error)`; `ticket.NewService(b db.Beginner) Service`; `ticket.NewHandler(svc) *Handler`, `Mount(private)`. Unexported helpers reused by Task 10: `get(ctx, q *db.Queries, p auth.Principal, id int64) (*Ticket, error)`, `loadVisible(ctx, q, p, id) (db.GetTicketRow, error)`, `event(ctx, q, ticketID int64, staffID *int64, kind db.TicketEventKind, data map[string]any) error`, `fromRow(db.ListTicketsRow) Ticket`, `principal(c *gin.Context) (auth.Principal, bool)`, test helper `newFixture(t) *fx`.
- Queries: `NextTicketNumber`, `CreateTicket`, `GetTicket`, `ListTickets`, `CountTickets`, `UpdateTicket`, `CreateThreadEntry`, `CreateTicketEvent`.

- [ ] **Step 1: Add queries and regenerate**

`gin/db/queries/ticket.sql`:

```sql
-- name: NextTicketNumber :one
SELECT nextval('ticket_number_seq')::bigint;

-- name: CreateTicket :one
INSERT INTO ticket (number, subject, status_id, dept_id, topic_id, priority_id,
                    requester_name, requester_email, source, due_at, extra, last_message_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now())
RETURNING id;

-- name: GetTicket :one
SELECT t.id, t.number, t.subject,
       t.status_id, s.name AS status_name, s.state AS status_state,
       t.dept_id, d.name AS dept_name,
       t.topic_id, ht.name AS topic_name,
       t.priority_id, p.name AS priority_name,
       t.assigned_staff_id, st.first_name AS assignee_first_name, st.last_name AS assignee_last_name,
       t.requester_name, t.requester_email, t.source, t.is_answered,
       t.due_at, t.closed_at, t.last_message_at, t.last_response_at, t.extra,
       t.created_at, t.updated_at
FROM ticket t
JOIN ticket_status s ON s.id = t.status_id
JOIN department d ON d.id = t.dept_id
JOIN ticket_priority p ON p.id = t.priority_id
LEFT JOIN help_topic ht ON ht.id = t.topic_id
LEFT JOIN staff st ON st.id = t.assigned_staff_id
WHERE t.id = $1;

-- name: ListTickets :many
SELECT t.id, t.number, t.subject,
       t.status_id, s.name AS status_name, s.state AS status_state,
       t.dept_id, d.name AS dept_name,
       t.topic_id, ht.name AS topic_name,
       t.priority_id, p.name AS priority_name,
       t.assigned_staff_id, st.first_name AS assignee_first_name, st.last_name AS assignee_last_name,
       t.requester_name, t.requester_email, t.source, t.is_answered,
       t.due_at, t.closed_at, t.last_message_at, t.last_response_at, t.extra,
       t.created_at, t.updated_at
FROM ticket t
JOIN ticket_status s ON s.id = t.status_id
JOIN department d ON d.id = t.dept_id
JOIN ticket_priority p ON p.id = t.priority_id
LEFT JOIN help_topic ht ON ht.id = t.topic_id
LEFT JOIN staff st ON st.id = t.assigned_staff_id
WHERE (@all_depts::boolean OR t.dept_id = ANY(@dept_ids::bigint[]))
  AND (sqlc.narg('status_id')::bigint IS NULL OR t.status_id = sqlc.narg('status_id'))
  AND (sqlc.narg('state')::text IS NULL OR s.state::text = sqlc.narg('state'))
  AND (sqlc.narg('filter_dept_id')::bigint IS NULL OR t.dept_id = sqlc.narg('filter_dept_id'))
  AND (sqlc.narg('assigned_staff_id')::bigint IS NULL OR t.assigned_staff_id = sqlc.narg('assigned_staff_id'))
  AND (NOT @unassigned::boolean OR t.assigned_staff_id IS NULL)
  AND (@q::text = '' OR to_tsvector('english', t.subject) @@ plainto_tsquery('english', @q))
ORDER BY
  CASE WHEN @sort::text = 'created_at' THEN t.created_at END ASC,
  CASE WHEN @sort::text = '-created_at' THEN t.created_at END DESC,
  CASE WHEN @sort::text = 'last_message_at' THEN t.last_message_at END ASC,
  CASE WHEN @sort::text = '-last_message_at' THEN t.last_message_at END DESC,
  CASE WHEN @sort::text = 'priority' THEN p.urgency END ASC,
  CASE WHEN @sort::text = '-priority' THEN p.urgency END DESC,
  t.id DESC
LIMIT @page_size::int OFFSET @page_offset::int;

-- name: CountTickets :one
SELECT count(*)::bigint
FROM ticket t
JOIN ticket_status s ON s.id = t.status_id
WHERE (@all_depts::boolean OR t.dept_id = ANY(@dept_ids::bigint[]))
  AND (sqlc.narg('status_id')::bigint IS NULL OR t.status_id = sqlc.narg('status_id'))
  AND (sqlc.narg('state')::text IS NULL OR s.state::text = sqlc.narg('state'))
  AND (sqlc.narg('filter_dept_id')::bigint IS NULL OR t.dept_id = sqlc.narg('filter_dept_id'))
  AND (sqlc.narg('assigned_staff_id')::bigint IS NULL OR t.assigned_staff_id = sqlc.narg('assigned_staff_id'))
  AND (NOT @unassigned::boolean OR t.assigned_staff_id IS NULL)
  AND (@q::text = '' OR to_tsvector('english', t.subject) @@ plainto_tsquery('english', @q));

-- name: UpdateTicket :exec
UPDATE ticket
SET subject = COALESCE(sqlc.narg('subject'), subject),
    priority_id = COALESCE(sqlc.narg('priority_id'), priority_id),
    topic_id = COALESCE(sqlc.narg('topic_id'), topic_id),
    due_at = COALESCE(sqlc.narg('due_at'), due_at),
    extra = COALESCE(sqlc.narg('extra'), extra),
    requester_name = COALESCE(sqlc.narg('requester_name'), requester_name),
    requester_email = COALESCE(sqlc.narg('requester_email'), requester_email),
    updated_at = now()
WHERE id = @id;

-- name: CreateThreadEntry :one
INSERT INTO thread_entry (ticket_id, type, staff_id, poster, title, body, format, parent_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: CreateTicketEvent :exec
INSERT INTO ticket_event (ticket_id, staff_id, kind, data) VALUES ($1, $2, $3, $4);
```

Run: `make sqlc && go build ./...`
Expected generated shapes: `ListTicketsParams{AllDepts bool; DeptIds []int64; StatusID *int64; State *string; FilterDeptID *int64; AssignedStaffID *int64; Unassigned bool; Q string; Sort string; PageSize int32; PageOffset int32}`; `CountTicketsParams` with the same minus `Sort`, `PageSize`, `PageOffset`; `GetTicketRow` and `ListTicketsRow` with identical fields (`TopicName *string`, `AssigneeFirstName *string`, `StatusState TicketState`, `Source TicketSource`, `Extra []byte`, `DueAt *time.Time`); `CreateThreadEntryParams{TicketID int64; Type ThreadEntryType; StaffID *int64; Poster string; Title *string; Body string; Format BodyFormat; ParentID *int64}`; `CreateTicketEventParams{TicketID int64; StaffID *int64; Kind TicketEventKind; Data []byte}`. If sqlc names a field differently (for example `DeptIDs`), adjust the Go code below to the generated name; the SQL is the source of truth.

- [ ] **Step 2: Test fixture**

`gin/internal/ticket/fixture_test.go`:

```go
package ticket

import (
	"context"
	"testing"

	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/db/testutil"
	"github.com/jackc/pgx/v5"
)

type fx struct {
	ctx      context.Context
	tx       pgx.Tx
	q        *db.Queries
	svc      Service
	support  db.Department
	billing  db.Department
	staff    map[string]db.Staff // "admin", "agent", "other"
	admin    auth.Principal
	agent    auth.Principal
	other    auth.Principal
	open     db.TicketStatus
	resolved db.TicketStatus
	closed   db.TicketStatus
	topic    db.HelpTopic
}

func newFixture(t *testing.T) *fx {
	t.Helper()
	ctx := context.Background()
	tx := testutil.Tx(t)
	q := db.New(tx)
	f := &fx{ctx: ctx, tx: tx, q: q, svc: NewService(tx), staff: map[string]db.Staff{}}
	var err error
	if f.support, err = q.FirstDepartment(ctx); err != nil {
		t.Fatal(err)
	}
	if f.billing, err = q.CreateDepartment(ctx, db.CreateDepartmentParams{Name: "Billing", IsPublic: true}); err != nil {
		t.Fatal(err)
	}
	mk := func(name string, admin bool, dept int64) db.Staff {
		st, err := q.CreateStaff(ctx, db.CreateStaffParams{
			Username: name, Email: name + "@x.test", PasswordHash: "h", FirstName: name, LastName: "Person",
			IsAdmin: admin, IsActive: true, PrimaryDeptID: dept,
		})
		if err != nil {
			t.Fatal(err)
		}
		f.staff[name] = st
		return st
	}
	adm := mk("admin", true, f.support.ID)
	ag := mk("agent", false, f.support.ID)
	ot := mk("other", false, f.billing.ID)
	f.admin = auth.Principal{StaffID: adm.ID, IsAdmin: true}
	f.agent = auth.Principal{StaffID: ag.ID, DeptIDs: []int64{f.support.ID}}
	f.other = auth.Principal{StaffID: ot.ID, DeptIDs: []int64{f.billing.ID}}
	statuses, err := q.ListStatuses(ctx)
	if err != nil || len(statuses) != 3 {
		t.Fatalf("statuses: %v %v", statuses, err)
	}
	f.open, f.resolved, f.closed = statuses[0], statuses[1], statuses[2]
	topics, err := q.ListTopics(ctx)
	if err != nil || len(topics) == 0 {
		t.Fatalf("topics: %v %v", topics, err)
	}
	f.topic = topics[0]
	return f
}

// create makes a ticket in dept as principal p; fails the test on error.
func (f *fx) create(t *testing.T, p auth.Principal, subject string, dept int64) *Ticket {
	t.Helper()
	tk, err := f.svc.Create(f.ctx, p, CreateInput{
		Subject: subject, Message: "<p>hello</p>", RequesterName: "Req", RequesterEmail: "req@x.test", DeptID: &dept,
	})
	if err != nil {
		t.Fatalf("create %q: %v", subject, err)
	}
	return tk
}

func (f *fx) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := f.tx.QueryRow(f.ctx, sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
```

- [ ] **Step 3: Write the failing service tests**

`gin/internal/ticket/service_test.go`:

```go
package ticket

import (
	"encoding/json"
	"errors"
	"regexp"
	"testing"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/httpx"
)

func TestCreateWithTopicDefaults(t *testing.T) {
	f := newFixture(t)
	tk, err := f.svc.Create(f.ctx, f.agent, CreateInput{
		Subject: "Printer on fire", Message: "help", RequesterEmail: "r@x.test", TopicID: &f.topic.ID,
		Source: "phone", Extra: json.RawMessage(`{"phone":"123"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^\d{6}$`).MatchString(tk.Number) {
		t.Fatalf("number %q", tk.Number)
	}
	if tk.Department.ID != f.support.ID || tk.Priority.Name != "normal" || tk.Status.ID != f.open.ID || tk.State != "open" {
		t.Fatalf("defaults: %+v", tk)
	}
	if tk.Topic == nil || tk.Topic.ID != f.topic.ID || tk.Source != "phone" || string(tk.Extra) != `{"phone": "123"}` {
		t.Fatalf("fields: %+v extra=%s", tk, tk.Extra)
	}
	if tk.Assignee != nil || tk.IsAnswered || tk.ClosedAt != nil {
		t.Fatalf("initial state: %+v", tk)
	}
	if n := f.count(t, `SELECT count(*) FROM thread_entry WHERE ticket_id = $1 AND type = 'message'`, tk.ID); n != 1 {
		t.Fatalf("message entries: %d", n)
	}
	if n := f.count(t, `SELECT count(*) FROM ticket_event WHERE ticket_id = $1 AND kind = 'created' AND staff_id = $2`, tk.ID, f.agent.StaffID); n != 1 {
		t.Fatalf("created event: %d", n)
	}
	second := f.create(t, f.agent, "Second", f.support.ID)
	if second.Number == tk.Number {
		t.Fatal("numbers must be unique")
	}
}

func TestCreateValidationAndVisibility(t *testing.T) {
	f := newFixture(t)
	var ve *apperr.ValidationError
	_, err := f.svc.Create(f.ctx, f.agent, CreateInput{Subject: "s", Message: "m", RequesterEmail: "r@x.test"})
	if !errors.As(err, &ve) || ve.Fields["dept_id"] == "" {
		t.Fatalf("no dept: %v", err)
	}
	bad := int64(999999)
	_, err = f.svc.Create(f.ctx, f.agent, CreateInput{Subject: "s", Message: "m", RequesterEmail: "r@x.test", TopicID: &bad, PriorityID: &bad})
	if !errors.As(err, &ve) || ve.Fields["topic_id"] == "" || ve.Fields["priority_id"] == "" {
		t.Fatalf("unknown refs: %v", err)
	}
	_, err = f.svc.Create(f.ctx, f.agent, CreateInput{Subject: "s", Message: "m", RequesterEmail: "r@x.test", DeptID: &f.support.ID, Extra: json.RawMessage(`[1]`)})
	if !errors.As(err, &ve) || ve.Fields["extra"] == "" {
		t.Fatalf("extra not object: %v", err)
	}
	_, err = f.svc.Create(f.ctx, f.agent, CreateInput{Subject: "s", Message: "m", RequesterEmail: "r@x.test", DeptID: &f.billing.ID})
	if !errors.Is(err, apperr.ErrForbidden) {
		t.Fatalf("agent into invisible dept: %v", err)
	}
	if _, err := f.svc.Create(f.ctx, f.admin, CreateInput{Subject: "s", Message: "m", RequesterEmail: "r@x.test", DeptID: &f.billing.ID}); err != nil {
		t.Fatalf("admin into any dept: %v", err)
	}
	if n := f.count(t, `SELECT count(*) FROM ticket`); n != 1 {
		t.Fatalf("failed creates must not leave rows: %d", n)
	}
}

func TestGetVisibility(t *testing.T) {
	f := newFixture(t)
	tk := f.create(t, f.agent, "Mine", f.support.ID)
	if _, err := f.svc.Get(f.ctx, f.agent, tk.ID); err != nil {
		t.Fatalf("owner dept: %v", err)
	}
	if _, err := f.svc.Get(f.ctx, f.other, tk.ID); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("other dept must be 404: %v", err)
	}
	if _, err := f.svc.Get(f.ctx, f.admin, tk.ID); err != nil {
		t.Fatalf("admin: %v", err)
	}
	if _, err := f.svc.Get(f.ctx, f.admin, 999999); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestListFilters(t *testing.T) {
	f := newFixture(t)
	a := f.create(t, f.admin, "Printer jam", f.support.ID)
	f.create(t, f.admin, "Invoice wrong", f.billing.ID)
	f.create(t, f.admin, "Printer toner", f.support.ID)
	page := httpx.Page{Page: 1, PageSize: 25}

	res, err := f.svc.List(f.ctx, f.admin, ListFilter{Page: page})
	if err != nil || res.Total != 3 || len(res.Items) != 3 {
		t.Fatalf("admin all: %+v %v", res, err)
	}
	res, _ = f.svc.List(f.ctx, f.agent, ListFilter{Page: page})
	if res.Total != 2 {
		t.Fatalf("agent sees own depts only: %d", res.Total)
	}
	res, _ = f.svc.List(f.ctx, f.agent, ListFilter{Page: page, DeptID: &f.billing.ID})
	if res.Total != 0 {
		t.Fatalf("agent filtering invisible dept: %d", res.Total)
	}
	res, _ = f.svc.List(f.ctx, f.admin, ListFilter{Page: page, Q: "printer"})
	if res.Total != 2 {
		t.Fatalf("search: %d", res.Total)
	}
	res, _ = f.svc.List(f.ctx, f.admin, ListFilter{Page: page, AssignedTo: "none"})
	if res.Total != 3 {
		t.Fatalf("unassigned: %d", res.Total)
	}
	res, _ = f.svc.List(f.ctx, f.admin, ListFilter{Page: page, AssignedTo: "me"})
	if res.Total != 0 {
		t.Fatalf("assigned to me: %d", res.Total)
	}
	res, _ = f.svc.List(f.ctx, f.admin, ListFilter{Page: page, State: "closed"})
	if res.Total != 0 {
		t.Fatalf("closed: %d", res.Total)
	}
	res, _ = f.svc.List(f.ctx, f.admin, ListFilter{Page: page, StatusID: &f.open.ID, Sort: "created_at"})
	if res.Total != 3 || res.Items[0].ID != a.ID {
		t.Fatalf("status + sort asc: %+v", res)
	}
	res, _ = f.svc.List(f.ctx, f.admin, ListFilter{Page: httpx.Page{Page: 2, PageSize: 2}})
	if res.Total != 3 || len(res.Items) != 1 || res.Page != 2 {
		t.Fatalf("pagination: %+v", res)
	}
	var ve *apperr.ValidationError
	if _, err := f.svc.List(f.ctx, f.admin, ListFilter{Page: page, Sort: "nope"}); !errors.As(err, &ve) || ve.Fields["sort"] == "" {
		t.Fatalf("bad sort: %v", err)
	}
	if _, err := f.svc.List(f.ctx, f.admin, ListFilter{Page: page, State: "weird"}); !errors.As(err, &ve) || ve.Fields["state"] == "" {
		t.Fatalf("bad state: %v", err)
	}
	if _, err := f.svc.List(f.ctx, f.admin, ListFilter{Page: page, AssignedTo: "bob"}); !errors.As(err, &ve) || ve.Fields["assigned_to"] == "" {
		t.Fatalf("bad assigned_to: %v", err)
	}
}

func TestUpdate(t *testing.T) {
	f := newFixture(t)
	tk := f.create(t, f.agent, "Old", f.support.ID)
	prios, _ := f.svc.ListPriorities(f.ctx)
	high := prios[2].ID
	subject := "New subject"
	email := "new@x.test"
	out, err := f.svc.Update(f.ctx, f.agent, tk.ID, UpdateInput{Subject: &subject, PriorityID: &high, RequesterEmail: &email, Extra: json.RawMessage(`{"a":1}`)})
	if err != nil {
		t.Fatal(err)
	}
	if out.Subject != subject || out.Priority.ID != high || out.RequesterEmail != email || string(out.Extra) != `{"a": 1}` {
		t.Fatalf("updated: %+v", out)
	}
	if n := f.count(t, `SELECT count(*) FROM ticket_event WHERE ticket_id = $1 AND kind = 'edited' AND data->'fields' ? 'subject'`, tk.ID); n != 1 {
		t.Fatalf("edited event: %d", n)
	}
	bad := int64(999999)
	var ve *apperr.ValidationError
	if _, err := f.svc.Update(f.ctx, f.agent, tk.ID, UpdateInput{PriorityID: &bad}); !errors.As(err, &ve) || ve.Fields["priority_id"] == "" {
		t.Fatalf("bad priority: %v", err)
	}
	if _, err := f.svc.Update(f.ctx, f.other, tk.ID, UpdateInput{Subject: &subject}); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("invisible: %v", err)
	}
}

func TestReferenceLists(t *testing.T) {
	f := newFixture(t)
	prios, err := f.svc.ListPriorities(f.ctx)
	if err != nil || len(prios) != 4 || prios[3].Name != "emergency" {
		t.Fatalf("priorities: %+v %v", prios, err)
	}
	statuses, err := f.svc.ListStatuses(f.ctx)
	if err != nil || len(statuses) != 3 || statuses[2].State != "closed" {
		t.Fatalf("statuses: %+v %v", statuses, err)
	}
}
```

- [ ] **Step 4: Run to verify it fails**

Run: `go test ./internal/ticket/ -v`
Expected: FAIL to compile, "undefined: NewService".

- [ ] **Step 5: Implement types**

`gin/internal/ticket/types.go`:

```go
// Package ticket implements tickets, threads, and the actions agents take on them.
package ticket

import (
	"encoding/json"
	"time"

	"github.com/grandpine/ticket-api/internal/httpx"
)

type Ref struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type Ticket struct {
	ID             int64           `json:"id"`
	Number         string          `json:"number"`
	Subject        string          `json:"subject"`
	Status         Ref             `json:"status"`
	State          string          `json:"state"`
	Department     Ref             `json:"department"`
	Topic          *Ref            `json:"topic"`
	Priority       Ref             `json:"priority"`
	Assignee       *Ref            `json:"assignee"`
	RequesterName  string          `json:"requester_name"`
	RequesterEmail string          `json:"requester_email"`
	Source         string          `json:"source"`
	IsAnswered     bool            `json:"is_answered"`
	DueAt          *time.Time      `json:"due_at"`
	ClosedAt       *time.Time      `json:"closed_at"`
	LastMessageAt  time.Time       `json:"last_message_at"`
	LastResponseAt *time.Time      `json:"last_response_at"`
	Extra          json.RawMessage `json:"extra"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type Priority struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Urgency int32  `json:"urgency"`
	Color   string `json:"color"`
}

type Status struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	State     string `json:"state"`
	SortOrder int32  `json:"sort_order"`
}

type CreateInput struct {
	Subject        string          `json:"subject" binding:"required,max=255"`
	Message        string          `json:"message" binding:"required"`
	MessageFormat  string          `json:"message_format" binding:"omitempty,oneof=html text"`
	RequesterName  string          `json:"requester_name" binding:"max=128"`
	RequesterEmail string          `json:"requester_email" binding:"required,email,max=255"`
	DeptID         *int64          `json:"dept_id"`
	TopicID        *int64          `json:"topic_id"`
	PriorityID     *int64          `json:"priority_id"`
	Source         string          `json:"source" binding:"omitempty,oneof=web api phone other"`
	DueAt          *time.Time      `json:"due_at"`
	Extra          json.RawMessage `json:"extra"`
	FileIDs        []int64         `json:"file_ids"`
}

type UpdateInput struct {
	Subject        *string         `json:"subject" binding:"omitempty,min=1,max=255"`
	PriorityID     *int64          `json:"priority_id"`
	TopicID        *int64          `json:"topic_id"`
	DueAt          *time.Time      `json:"due_at"`
	Extra          json.RawMessage `json:"extra"`
	RequesterName  *string         `json:"requester_name" binding:"omitempty,max=128"`
	RequesterEmail *string         `json:"requester_email" binding:"omitempty,email,max=255"`
}

type ListFilter struct {
	StatusID   *int64
	State      string
	DeptID     *int64
	AssignedTo string
	Q          string
	Sort       string
	Page       httpx.Page
}
```

- [ ] **Step 6: Implement the service**

`gin/internal/ticket/service.go`:

```go
package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/httpx"
	"github.com/jackc/pgx/v5"
)

type Service interface {
	Create(ctx context.Context, p auth.Principal, in CreateInput) (*Ticket, error)
	Get(ctx context.Context, p auth.Principal, id int64) (*Ticket, error)
	List(ctx context.Context, p auth.Principal, f ListFilter) (*httpx.List[Ticket], error)
	Update(ctx context.Context, p auth.Principal, id int64, in UpdateInput) (*Ticket, error)
	ListPriorities(ctx context.Context) ([]Priority, error)
	ListStatuses(ctx context.Context) ([]Status, error)
}

type service struct{ db db.Beginner }

func NewService(b db.Beginner) Service { return &service{db: b} }

func notFound(id int64) error { return fmt.Errorf("ticket %d: %w", id, apperr.ErrNotFound) }

func (s *service) Create(ctx context.Context, p auth.Principal, in CreateInput) (*Ticket, error) {
	if err := validateExtra(in.Extra); err != nil {
		return nil, err
	}
	var out *Ticket
	err := db.WithTx(ctx, s.db, func(q *db.Queries) error {
		fields := map[string]string{}
		deptID, priorityID := in.DeptID, in.PriorityID
		if in.TopicID != nil {
			topic, err := q.GetTopic(ctx, *in.TopicID)
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				fields["topic_id"] = "unknown topic"
			case err != nil:
				return err
			default:
				if deptID == nil {
					deptID = topic.DeptID
				}
				if priorityID == nil {
					priorityID = topic.PriorityID
				}
			}
		}
		if deptID == nil {
			fields["dept_id"] = "required when no topic supplies a department"
		} else if _, err := q.GetDepartment(ctx, *deptID); errors.Is(err, pgx.ErrNoRows) {
			fields["dept_id"] = "unknown department"
		} else if err != nil {
			return err
		}
		if priorityID == nil {
			def, err := q.DefaultPriority(ctx)
			if err != nil {
				return err
			}
			priorityID = &def.ID
		} else if _, err := q.GetPriority(ctx, *priorityID); errors.Is(err, pgx.ErrNoRows) {
			fields["priority_id"] = "unknown priority"
		} else if err != nil {
			return err
		}
		if len(fields) > 0 {
			return &apperr.ValidationError{Fields: fields}
		}
		if !p.CanSeeDept(*deptID) {
			return fmt.Errorf("%w: cannot create tickets in that department", apperr.ErrForbidden)
		}
		status, err := q.DefaultStatus(ctx)
		if err != nil {
			return err
		}
		n, err := q.NextTicketNumber(ctx)
		if err != nil {
			return err
		}
		number := fmt.Sprintf("%06d", n)
		source := db.TicketSourceWeb
		if in.Source != "" {
			source = db.TicketSource(in.Source)
		}
		extra := []byte("{}")
		if len(in.Extra) > 0 {
			extra = in.Extra
		}
		id, err := q.CreateTicket(ctx, db.CreateTicketParams{
			Number: number, Subject: in.Subject, StatusID: status.ID, DeptID: *deptID, TopicID: in.TopicID,
			PriorityID: *priorityID, RequesterName: in.RequesterName, RequesterEmail: in.RequesterEmail,
			Source: source, DueAt: in.DueAt, Extra: extra,
		})
		if err != nil {
			return err
		}
		poster := in.RequesterName
		if poster == "" {
			poster = in.RequesterEmail
		}
		entry, err := q.CreateThreadEntry(ctx, db.CreateThreadEntryParams{
			TicketID: id, Type: db.ThreadEntryTypeMessage, Poster: poster, Body: in.Message, Format: bodyFormat(in.MessageFormat),
		})
		if err != nil {
			return err
		}
		if err := attachFiles(ctx, q, entry.ID, in.FileIDs); err != nil {
			return err
		}
		if err := event(ctx, q, id, &p.StaffID, db.TicketEventKindCreated, map[string]any{"number": number}); err != nil {
			return err
		}
		out, err = get(ctx, q, p, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *service) Get(ctx context.Context, p auth.Principal, id int64) (*Ticket, error) {
	return get(ctx, db.New(s.db), p, id)
}

var sortKeys = map[string]bool{
	"created_at": true, "-created_at": true, "last_message_at": true, "-last_message_at": true,
	"priority": true, "-priority": true,
}

func (s *service) List(ctx context.Context, p auth.Principal, f ListFilter) (*httpx.List[Ticket], error) {
	fields := map[string]string{}
	sort := f.Sort
	if sort == "" {
		sort = "-last_message_at"
	}
	if !sortKeys[sort] {
		fields["sort"] = "unknown sort key"
	}
	var state *string
	if f.State != "" {
		if !db.TicketState(f.State).Valid() {
			fields["state"] = "must be open, resolved or closed"
		}
		st := f.State
		state = &st
	}
	var assigned *int64
	unassigned := false
	switch f.AssignedTo {
	case "":
	case "me":
		id := p.StaffID
		assigned = &id
	case "none":
		unassigned = true
	default:
		n, err := strconv.ParseInt(f.AssignedTo, 10, 64)
		if err != nil || n <= 0 {
			fields["assigned_to"] = "must be a staff id, me or none"
		} else {
			assigned = &n
		}
	}
	if len(fields) > 0 {
		return nil, &apperr.ValidationError{Fields: fields}
	}
	deptIDs := p.DeptIDs
	if deptIDs == nil {
		deptIDs = []int64{}
	}
	q := db.New(s.db)
	rows, err := q.ListTickets(ctx, db.ListTicketsParams{
		AllDepts: p.IsAdmin, DeptIds: deptIDs, StatusID: f.StatusID, State: state, FilterDeptID: f.DeptID,
		AssignedStaffID: assigned, Unassigned: unassigned, Q: f.Q, Sort: sort,
		PageSize: f.Page.Limit(), PageOffset: f.Page.Offset(),
	})
	if err != nil {
		return nil, err
	}
	total, err := q.CountTickets(ctx, db.CountTicketsParams{
		AllDepts: p.IsAdmin, DeptIds: deptIDs, StatusID: f.StatusID, State: state, FilterDeptID: f.DeptID,
		AssignedStaffID: assigned, Unassigned: unassigned, Q: f.Q,
	})
	if err != nil {
		return nil, err
	}
	items := make([]Ticket, 0, len(rows))
	for _, r := range rows {
		items = append(items, fromRow(r))
	}
	return &httpx.List[Ticket]{Items: items, Page: f.Page.Page, PageSize: f.Page.PageSize, Total: total}, nil
}

func (s *service) Update(ctx context.Context, p auth.Principal, id int64, in UpdateInput) (*Ticket, error) {
	if err := validateExtra(in.Extra); err != nil {
		return nil, err
	}
	var out *Ticket
	err := db.WithTx(ctx, s.db, func(q *db.Queries) error {
		if _, err := loadVisible(ctx, q, p, id); err != nil {
			return err
		}
		fields := map[string]string{}
		if in.PriorityID != nil {
			if _, err := q.GetPriority(ctx, *in.PriorityID); errors.Is(err, pgx.ErrNoRows) {
				fields["priority_id"] = "unknown priority"
			} else if err != nil {
				return err
			}
		}
		if in.TopicID != nil {
			if _, err := q.GetTopic(ctx, *in.TopicID); errors.Is(err, pgx.ErrNoRows) {
				fields["topic_id"] = "unknown topic"
			} else if err != nil {
				return err
			}
		}
		if len(fields) > 0 {
			return &apperr.ValidationError{Fields: fields}
		}
		var extra []byte
		if len(in.Extra) > 0 {
			extra = in.Extra
		}
		if err := q.UpdateTicket(ctx, db.UpdateTicketParams{
			ID: id, Subject: in.Subject, PriorityID: in.PriorityID, TopicID: in.TopicID, DueAt: in.DueAt,
			Extra: extra, RequesterName: in.RequesterName, RequesterEmail: in.RequesterEmail,
		}); err != nil {
			return err
		}
		if err := event(ctx, q, id, &p.StaffID, db.TicketEventKindEdited, map[string]any{"fields": changedFields(in)}); err != nil {
			return err
		}
		var err error
		out, err = get(ctx, q, p, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *service) ListPriorities(ctx context.Context) ([]Priority, error) {
	rows, err := db.New(s.db).ListPriorities(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Priority, 0, len(rows))
	for _, r := range rows {
		out = append(out, Priority{ID: r.ID, Name: r.Name, Urgency: r.Urgency, Color: r.Color})
	}
	return out, nil
}

func (s *service) ListStatuses(ctx context.Context) ([]Status, error) {
	rows, err := db.New(s.db).ListStatuses(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Status, 0, len(rows))
	for _, r := range rows {
		out = append(out, Status{ID: r.ID, Name: r.Name, State: string(r.State), SortOrder: r.SortOrder})
	}
	return out, nil
}

// loadVisible returns the raw ticket row or ErrNotFound when missing or invisible.
func loadVisible(ctx context.Context, q *db.Queries, p auth.Principal, id int64) (db.GetTicketRow, error) {
	row, err := q.GetTicket(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.GetTicketRow{}, notFound(id)
	}
	if err != nil {
		return db.GetTicketRow{}, err
	}
	if !p.CanSeeDept(row.DeptID) {
		return db.GetTicketRow{}, notFound(id)
	}
	return row, nil
}

func get(ctx context.Context, q *db.Queries, p auth.Principal, id int64) (*Ticket, error) {
	row, err := loadVisible(ctx, q, p, id)
	if err != nil {
		return nil, err
	}
	t := fromRow(db.ListTicketsRow(row))
	return &t, nil
}

func fromRow(r db.ListTicketsRow) Ticket {
	t := Ticket{
		ID: r.ID, Number: r.Number, Subject: r.Subject,
		Status: Ref{ID: r.StatusID, Name: r.StatusName}, State: string(r.StatusState),
		Department: Ref{ID: r.DeptID, Name: r.DeptName},
		Priority:   Ref{ID: r.PriorityID, Name: r.PriorityName},
		RequesterName: r.RequesterName, RequesterEmail: r.RequesterEmail, Source: string(r.Source),
		IsAnswered: r.IsAnswered, DueAt: utcPtr(r.DueAt), ClosedAt: utcPtr(r.ClosedAt),
		LastMessageAt: r.LastMessageAt.UTC(), LastResponseAt: utcPtr(r.LastResponseAt),
		Extra: json.RawMessage(r.Extra), CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
	if r.TopicID != nil && r.TopicName != nil {
		t.Topic = &Ref{ID: *r.TopicID, Name: *r.TopicName}
	}
	if r.AssignedStaffID != nil {
		t.Assignee = &Ref{ID: *r.AssignedStaffID, Name: fullName(r.AssigneeFirstName, r.AssigneeLastName)}
	}
	return t
}

func fullName(first, last *string) string {
	var parts []string
	for _, s := range []*string{first, last} {
		if s != nil && *s != "" {
			parts = append(parts, *s)
		}
	}
	return strings.Join(parts, " ")
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func bodyFormat(s string) db.BodyFormat {
	if s == "text" {
		return db.BodyFormatText
	}
	return db.BodyFormatHtml
}

func validateExtra(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if !json.Valid(raw) || !strings.HasPrefix(trimmed, "{") {
		return apperr.Validation("extra", "must be a JSON object")
	}
	return nil
}

func changedFields(in UpdateInput) []string {
	var out []string
	if in.Subject != nil {
		out = append(out, "subject")
	}
	if in.PriorityID != nil {
		out = append(out, "priority_id")
	}
	if in.TopicID != nil {
		out = append(out, "topic_id")
	}
	if in.DueAt != nil {
		out = append(out, "due_at")
	}
	if len(in.Extra) > 0 {
		out = append(out, "extra")
	}
	if in.RequesterName != nil {
		out = append(out, "requester_name")
	}
	if in.RequesterEmail != nil {
		out = append(out, "requester_email")
	}
	if out == nil {
		out = []string{}
	}
	return out
}

func event(ctx context.Context, q *db.Queries, ticketID int64, staffID *int64, kind db.TicketEventKind, data map[string]any) error {
	if data == nil {
		data = map[string]any{}
	}
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return q.CreateTicketEvent(ctx, db.CreateTicketEventParams{TicketID: ticketID, StaffID: staffID, Kind: kind, Data: b})
}

// attachFiles is implemented in thread.go (Task 10). Until then it is a no-op
// that rejects any file ids so the create path stays honest.
```

Also create a temporary `gin/internal/ticket/thread.go` containing only:

```go
package ticket

import (
	"context"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/db"
)

func attachFiles(_ context.Context, _ *db.Queries, _ int64, fileIDs []int64) error {
	if len(fileIDs) > 0 {
		return apperr.Validation("file_ids", "attachments are not supported yet")
	}
	return nil
}
```

Task 10 replaces this file.

- [ ] **Step 7: Run service tests**

Run: `go test ./internal/ticket/ -v`
Expected: PASS (6 tests). If `db.TicketState(...).Valid()` does not exist in the generated code, replace with a `switch` over the three string values.

- [ ] **Step 8: Write the failing handler tests**

`gin/internal/ticket/handler_test.go`:

```go
package ticket

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/httpx"
)

type fakeSvc struct {
	lastFilter ListFilter
	lastCreate CreateInput
}

func (f *fakeSvc) Create(_ context.Context, p auth.Principal, in CreateInput) (*Ticket, error) {
	f.lastCreate = in
	return &Ticket{ID: 1, Subject: in.Subject}, nil
}
func (f *fakeSvc) Get(_ context.Context, p auth.Principal, id int64) (*Ticket, error) {
	if id == 1 {
		return &Ticket{ID: 1, Subject: "one"}, nil
	}
	return nil, apperr.ErrNotFound
}
func (f *fakeSvc) List(_ context.Context, p auth.Principal, fl ListFilter) (*httpx.List[Ticket], error) {
	f.lastFilter = fl
	return &httpx.List[Ticket]{Items: []Ticket{}, Page: fl.Page.Page, PageSize: fl.Page.PageSize}, nil
}
func (f *fakeSvc) Update(_ context.Context, p auth.Principal, id int64, in UpdateInput) (*Ticket, error) {
	return &Ticket{ID: id}, nil
}
func (f *fakeSvc) ListPriorities(context.Context) ([]Priority, error) {
	return []Priority{{ID: 1, Name: "low"}}, nil
}
func (f *fakeSvc) ListStatuses(context.Context) ([]Status, error) {
	return []Status{{ID: 1, Name: "Open", State: "open"}}, nil
}

func newRouter(f *fakeSvc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	private := r.Group("/api/v1", func(c *gin.Context) {
		auth.WithPrincipal(c, auth.Principal{StaffID: 7, DeptIDs: []int64{1}})
	})
	NewHandler(f).Mount(private)
	return r
}

func do(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestTicketCoreRoutes(t *testing.T) {
	f := &fakeSvc{}
	r := newRouter(f)
	if w := do(r, http.MethodGet, "/api/v1/priorities", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "low") {
		t.Fatalf("priorities: %d %s", w.Code, w.Body.String())
	}
	if w := do(r, http.MethodGet, "/api/v1/statuses", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "Open") {
		t.Fatalf("statuses: %d", w.Code)
	}
	w := do(r, http.MethodGet, "/api/v1/tickets?state=open&assigned_to=me&q=printer&sort=-priority&status=3&dept_id=1&page=2&page_size=10", "")
	if w.Code != 200 {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	fl := f.lastFilter
	if fl.State != "open" || fl.AssignedTo != "me" || fl.Q != "printer" || fl.Sort != "-priority" ||
		fl.StatusID == nil || *fl.StatusID != 3 || fl.DeptID == nil || *fl.DeptID != 1 || fl.Page.Page != 2 || fl.Page.PageSize != 10 {
		t.Fatalf("filter parsing: %+v", fl)
	}
	if w := do(r, http.MethodGet, "/api/v1/tickets?status=abc", ""); w.Code != 400 {
		t.Fatalf("bad status param: %d", w.Code)
	}
	if w := do(r, http.MethodGet, "/api/v1/tickets?page_size=0", ""); w.Code != 400 {
		t.Fatalf("bad page_size: %d", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/tickets", `{"subject":"s","message":"m","requester_email":"bad"}`); w.Code != 400 {
		t.Fatalf("bad email: %d", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/tickets", `{"subject":"`+strings.Repeat("x", 300)+`","message":"m","requester_email":"a@b.test"}`); w.Code != 400 {
		t.Fatalf("overlong subject: %d", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/tickets", `{"subject":"s","message":"m","requester_email":"a@b.test","dept_id":1,"source":"phone"}`); w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if f.lastCreate.Source != "phone" || f.lastCreate.DeptID == nil {
		t.Fatalf("create binding: %+v", f.lastCreate)
	}
	if w := do(r, http.MethodGet, "/api/v1/tickets/1", ""); w.Code != 200 {
		t.Fatalf("get: %d", w.Code)
	}
	if w := do(r, http.MethodGet, "/api/v1/tickets/2", ""); w.Code != 404 {
		t.Fatalf("get missing: %d", w.Code)
	}
	if w := do(r, http.MethodPatch, "/api/v1/tickets/1", `{"subject":""}`); w.Code != 400 {
		t.Fatalf("empty subject: %d", w.Code)
	}
	if w := do(r, http.MethodPatch, "/api/v1/tickets/1", `{"subject":"new"}`); w.Code != 200 {
		t.Fatalf("update: %d", w.Code)
	}
}
```

- [ ] **Step 9: Implement the handler**

`gin/internal/ticket/handler.go`:

```go
package ticket

import (
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
	var in UpdateInput
	if !httpx.BindJSON(c, &in) {
		return
	}
	out, err := h.svc.Update(c.Request.Context(), p, id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}
```

Add to the temporary `thread.go` so the package compiles:

```go
func (h *Handler) mountThread(private *gin.RouterGroup) {}
```

with `"github.com/gin-gonic/gin"` imported.

- [ ] **Step 10: Run all ticket tests and commit**

Run: `go test ./internal/ticket/ -v`
Expected: PASS (7 tests).

```bash
git add gin
git commit -m "feat(gin): ticket create, get, list, update with department visibility" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 10: Ticket thread and actions (reply, note, thread, status, assign, transfer, events, file linking)

**Files:**
- Create: `gin/db/queries/thread.sql`, `gin/db/queries/file.sql` (initial; Task 11 appends)
- Replace: `gin/internal/ticket/thread.go` (drop the Task 9 stub entirely)
- Modify: `gin/internal/ticket/service.go` (extend the `Service` interface)
- Modify: `gin/internal/ticket/handler_test.go` (extend `fakeSvc`)
- Create: `gin/internal/ticket/thread_test.go`, `gin/internal/ticket/thread_handler_test.go`

**Interfaces:**
- Consumes: `loadVisible`, `get`, `event`, `bodyFormat`, `fullName`, `principal` from Task 9; `StaffCanSeeDept`, `GetStaff`, `GetDepartment`, `GetStatus`.
- Produces: `ticket.ReplyInput{Body, Format string; StatusID *int64; FileIDs []int64}`, `ticket.NoteInput{Title, Body, Format string; FileIDs []int64}`, `ticket.StatusInput{StatusID int64}`, `ticket.AssignInput{StaffID *int64}`, `ticket.TransferInput{DeptID int64}`, `ticket.AttachmentRef{FileID int64; Name, Mime string; Size int64}`, `ticket.Entry`, `ticket.Thread{Items []Entry; NextAfter *int64}`, `ticket.Event{ID, TicketID int64; Staff *Ref; Kind string; Data json.RawMessage; CreatedAt time.Time}`. `Service` gains `Reply(ctx, p, id, in ReplyInput) (*Entry, error)`, `Note(ctx, p, id, in NoteInput) (*Entry, error)`, `Thread(ctx, p, id, after int64, limit int) (*Thread, error)`, `SetStatus(ctx, p, id, statusID int64) (*Ticket, error)`, `Assign(ctx, p, id, staffID *int64) (*Ticket, error)`, `Transfer(ctx, p, id, deptID int64) (*Ticket, error)`, `Events(ctx, p, id) ([]Event, error)`. `attachFiles(ctx, q, entryID int64, fileIDs []int64) error` becomes real.
- Queries: `ListThreadEntries`, `ListAttachmentsForEntries`, `ListTicketEvents`, `SetTicketStatus`, `SetTicketAssignee`, `SetTicketDept`, `MarkTicketAnswered`, `CreateFile`, `GetFile`, `IsFileAttached`, `CreateAttachment`.

- [ ] **Step 1: Add queries and regenerate**

`gin/db/queries/thread.sql`:

```sql
-- name: ListThreadEntries :many
SELECT * FROM thread_entry
WHERE ticket_id = @ticket_id AND id > @after
ORDER BY id
LIMIT @page_limit::int;

-- name: ListAttachmentsForEntries :many
SELECT a.thread_entry_id, a.file_id, a.inline, f.name, f.mime, f.size
FROM attachment a
JOIN file f ON f.id = a.file_id
WHERE a.thread_entry_id = ANY(@entry_ids::bigint[])
ORDER BY a.thread_entry_id, a.file_id;

-- name: ListTicketEvents :many
SELECT e.id, e.ticket_id, e.staff_id, s.first_name AS staff_first_name, s.last_name AS staff_last_name,
       e.kind, e.data, e.created_at
FROM ticket_event e
LEFT JOIN staff s ON s.id = e.staff_id
WHERE e.ticket_id = $1
ORDER BY e.id;

-- name: SetTicketStatus :exec
UPDATE ticket SET status_id = $2, closed_at = $3, updated_at = now() WHERE id = $1;

-- name: SetTicketAssignee :exec
UPDATE ticket SET assigned_staff_id = $2, updated_at = now() WHERE id = $1;

-- name: SetTicketDept :exec
UPDATE ticket SET dept_id = $2, updated_at = now() WHERE id = $1;

-- name: MarkTicketAnswered :exec
UPDATE ticket SET is_answered = true, last_response_at = now(), updated_at = now() WHERE id = $1;
```

`gin/db/queries/file.sql`:

```sql
-- name: CreateFile :one
INSERT INTO file (key, name, mime, size, sha256, backend, uploaded_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetFile :one
SELECT * FROM file WHERE id = $1;

-- name: IsFileAttached :one
SELECT EXISTS (SELECT 1 FROM attachment WHERE file_id = $1)::boolean;

-- name: CreateAttachment :exec
INSERT INTO attachment (thread_entry_id, file_id, inline) VALUES ($1, $2, false);
```

Run: `make sqlc && go build ./...`
Expected: `ListThreadEntriesParams{TicketID int64; After int64; PageLimit int32}`, `ListAttachmentsForEntriesRow{ThreadEntryID, FileID int64; Inline bool; Name, Mime string; Size int64}`, `ListTicketEventsRow{... StaffFirstName *string; Kind TicketEventKind; Data []byte ...}`, `SetTicketStatusParams{ID, StatusID int64; ClosedAt *time.Time}`, `SetTicketAssigneeParams{ID int64; AssignedStaffID *int64}`, `CreateFileParams{Key, Name, Mime string; Size int64; Sha256, Backend string; UploadedBy *int64}`.

- [ ] **Step 2: Write the failing service tests**

`gin/internal/ticket/thread_test.go`:

```go
package ticket

import (
	"errors"
	"testing"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/db"
)

func (f *fx) file(t *testing.T, name string) db.File {
	t.Helper()
	fl, err := f.q.CreateFile(f.ctx, db.CreateFileParams{
		Key: "key-" + name, Name: name, Mime: "text/plain", Size: 3, Sha256: "abc", Backend: "local",
	})
	if err != nil {
		t.Fatal(err)
	}
	return fl
}

func TestReplyMarksAnsweredAndCanClose(t *testing.T) {
	f := newFixture(t)
	tk := f.create(t, f.agent, "Q", f.support.ID)
	fl := f.file(t, "a.txt")
	entry, err := f.svc.Reply(f.ctx, f.agent, tk.ID, ReplyInput{Body: "Answer", FileIDs: []int64{fl.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if entry.Type != "response" || entry.StaffID == nil || entry.Poster != "agent Person" || len(entry.Attachments) != 1 || entry.Attachments[0].Name != "a.txt" {
		t.Fatalf("entry: %+v", entry)
	}
	got, _ := f.svc.Get(f.ctx, f.agent, tk.ID)
	if !got.IsAnswered || got.LastResponseAt == nil || got.State != "open" {
		t.Fatalf("after reply: %+v", got)
	}
	if _, err := f.svc.Reply(f.ctx, f.agent, tk.ID, ReplyInput{Body: "Closing", StatusID: &f.closed.ID}); err != nil {
		t.Fatal(err)
	}
	got, _ = f.svc.Get(f.ctx, f.agent, tk.ID)
	if got.State != "closed" || got.ClosedAt == nil {
		t.Fatalf("reply with status: %+v", got)
	}
	if _, err := f.svc.Reply(f.ctx, f.other, tk.ID, ReplyInput{Body: "x"}); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("invisible reply: %v", err)
	}
}

func TestReplyOnClosedTicketKeepsStatus(t *testing.T) {
	f := newFixture(t)
	tk := f.create(t, f.agent, "Q", f.support.ID)
	if _, err := f.svc.SetStatus(f.ctx, f.agent, tk.ID, f.closed.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Reply(f.ctx, f.agent, tk.ID, ReplyInput{Body: "late reply"}); err != nil {
		t.Fatal(err)
	}
	got, _ := f.svc.Get(f.ctx, f.agent, tk.ID)
	if got.State != "closed" || got.ClosedAt == nil {
		t.Fatalf("reply must not reopen: %+v", got)
	}
}

func TestNoteDoesNotMarkAnswered(t *testing.T) {
	f := newFixture(t)
	tk := f.create(t, f.agent, "Q", f.support.ID)
	entry, err := f.svc.Note(f.ctx, f.agent, tk.ID, NoteInput{Title: "internal", Body: "note body", Format: "text"})
	if err != nil {
		t.Fatal(err)
	}
	if entry.Type != "note" || entry.Title == nil || *entry.Title != "internal" || entry.Format != "text" {
		t.Fatalf("note: %+v", entry)
	}
	got, _ := f.svc.Get(f.ctx, f.agent, tk.ID)
	if got.IsAnswered || got.LastResponseAt != nil {
		t.Fatalf("note must not answer: %+v", got)
	}
}

func TestThreadCursor(t *testing.T) {
	f := newFixture(t)
	tk := f.create(t, f.agent, "Q", f.support.ID)
	for i := 0; i < 4; i++ {
		if _, err := f.svc.Note(f.ctx, f.agent, tk.ID, NoteInput{Body: "n"}); err != nil {
			t.Fatal(err)
		}
	}
	page, err := f.svc.Thread(f.ctx, f.agent, tk.ID, 0, 2)
	if err != nil || len(page.Items) != 2 || page.NextAfter == nil || page.Items[0].Type != "message" {
		t.Fatalf("page 1: %+v %v", page, err)
	}
	page2, _ := f.svc.Thread(f.ctx, f.agent, tk.ID, *page.NextAfter, 2)
	if len(page2.Items) != 2 || page2.NextAfter == nil || page2.Items[0].ID <= page.Items[1].ID {
		t.Fatalf("page 2: %+v", page2)
	}
	page3, _ := f.svc.Thread(f.ctx, f.agent, tk.ID, *page2.NextAfter, 2)
	if len(page3.Items) != 1 || page3.NextAfter != nil {
		t.Fatalf("page 3: %+v", page3)
	}
	if _, err := f.svc.Thread(f.ctx, f.other, tk.ID, 0, 10); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("invisible thread: %v", err)
	}
}

func TestStatusTransitions(t *testing.T) {
	f := newFixture(t)
	tk := f.create(t, f.agent, "Q", f.support.ID)
	same, err := f.svc.SetStatus(f.ctx, f.agent, tk.ID, f.open.ID)
	if err != nil || same.State != "open" {
		t.Fatalf("same status: %+v %v", same, err)
	}
	if n := f.count(t, `SELECT count(*) FROM ticket_event WHERE ticket_id = $1 AND kind <> 'created'`, tk.ID); n != 0 {
		t.Fatalf("no-op must not write events: %d", n)
	}
	closed, err := f.svc.SetStatus(f.ctx, f.agent, tk.ID, f.closed.ID)
	if err != nil || closed.State != "closed" || closed.ClosedAt == nil {
		t.Fatalf("close: %+v %v", closed, err)
	}
	if n := f.count(t, `SELECT count(*) FROM ticket_event WHERE ticket_id = $1 AND kind = 'closed'`, tk.ID); n != 1 {
		t.Fatalf("closed event: %d", n)
	}
	resolved, err := f.svc.SetStatus(f.ctx, f.agent, tk.ID, f.resolved.ID)
	if err != nil || resolved.State != "resolved" || resolved.ClosedAt == nil {
		t.Fatalf("closed to resolved keeps closed_at: %+v %v", resolved, err)
	}
	if n := f.count(t, `SELECT count(*) FROM ticket_event WHERE ticket_id = $1 AND kind = 'status_changed'`, tk.ID); n != 1 {
		t.Fatalf("status_changed event: %d", n)
	}
	reopened, err := f.svc.SetStatus(f.ctx, f.agent, tk.ID, f.open.ID)
	if err != nil || reopened.State != "open" || reopened.ClosedAt != nil {
		t.Fatalf("reopen: %+v %v", reopened, err)
	}
	if n := f.count(t, `SELECT count(*) FROM ticket_event WHERE ticket_id = $1 AND kind = 'reopened'`, tk.ID); n != 1 {
		t.Fatalf("reopened event: %d", n)
	}
	var ve *apperr.ValidationError
	if _, err := f.svc.SetStatus(f.ctx, f.agent, tk.ID, 999999); !errors.As(err, &ve) || ve.Fields["status_id"] == "" {
		t.Fatalf("unknown status: %v", err)
	}
}

func TestAssign(t *testing.T) {
	f := newFixture(t)
	tk := f.create(t, f.agent, "Q", f.support.ID)
	var ve *apperr.ValidationError
	bad := int64(999999)
	if _, err := f.svc.Assign(f.ctx, f.agent, tk.ID, &bad); !errors.As(err, &ve) || ve.Fields["staff_id"] == "" {
		t.Fatalf("unknown staff: %v", err)
	}
	other := f.staff["other"].ID
	if _, err := f.svc.Assign(f.ctx, f.agent, tk.ID, &other); !errors.As(err, &ve) || ve.Fields["staff_id"] == "" {
		t.Fatalf("staff outside dept: %v", err)
	}
	if _, err := f.tx.Exec(f.ctx, `UPDATE staff SET is_active = false WHERE id = $1`, f.staff["admin"].ID); err != nil {
		t.Fatal(err)
	}
	adminID := f.staff["admin"].ID
	if _, err := f.svc.Assign(f.ctx, f.agent, tk.ID, &adminID); !errors.As(err, &ve) || ve.Fields["staff_id"] == "" {
		t.Fatalf("inactive staff: %v", err)
	}
	me := f.agent.StaffID
	out, err := f.svc.Assign(f.ctx, f.agent, tk.ID, &me)
	if err != nil || out.Assignee == nil || out.Assignee.ID != me || out.Assignee.Name != "agent Person" {
		t.Fatalf("assign: %+v %v", out, err)
	}
	out, err = f.svc.Assign(f.ctx, f.agent, tk.ID, nil)
	if err != nil || out.Assignee != nil {
		t.Fatalf("unassign: %+v %v", out, err)
	}
	if n := f.count(t, `SELECT count(*) FROM ticket_event WHERE ticket_id = $1 AND kind IN ('assigned','unassigned')`, tk.ID); n != 2 {
		t.Fatalf("assignment events: %d", n)
	}
}

func TestTransfer(t *testing.T) {
	f := newFixture(t)
	tk := f.create(t, f.admin, "Q", f.support.ID)
	me := f.agent.StaffID
	if _, err := f.svc.Assign(f.ctx, f.admin, tk.ID, &me); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Transfer(f.ctx, f.agent, tk.ID, f.billing.ID); !errors.Is(err, apperr.ErrForbidden) {
		t.Fatalf("agent transfer to invisible dept: %v", err)
	}
	var ve *apperr.ValidationError
	if _, err := f.svc.Transfer(f.ctx, f.admin, tk.ID, 999999); !errors.As(err, &ve) || ve.Fields["dept_id"] == "" {
		t.Fatalf("unknown dept: %v", err)
	}
	same, err := f.svc.Transfer(f.ctx, f.admin, tk.ID, f.support.ID)
	if err != nil || same.Assignee == nil {
		t.Fatalf("same dept no-op: %+v %v", same, err)
	}
	moved, err := f.svc.Transfer(f.ctx, f.admin, tk.ID, f.billing.ID)
	if err != nil || moved.Department.ID != f.billing.ID || moved.Assignee != nil {
		t.Fatalf("transfer must unassign agent who cannot see target: %+v %v", moved, err)
	}
	if n := f.count(t, `SELECT count(*) FROM ticket_event WHERE ticket_id = $1 AND kind = 'transferred'`, tk.ID); n != 1 {
		t.Fatalf("transferred event: %d", n)
	}
	if _, err := f.svc.Get(f.ctx, f.agent, tk.ID); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("agent lost visibility after transfer: %v", err)
	}
}

func TestAttachFilesRules(t *testing.T) {
	f := newFixture(t)
	tk := f.create(t, f.agent, "Q", f.support.ID)
	fl := f.file(t, "b.txt")
	if _, err := f.svc.Note(f.ctx, f.agent, tk.ID, NoteInput{Body: "n", FileIDs: []int64{fl.ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Note(f.ctx, f.agent, tk.ID, NoteInput{Body: "n", FileIDs: []int64{fl.ID}}); !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("attach twice: %v", err)
	}
	var ve *apperr.ValidationError
	if _, err := f.svc.Note(f.ctx, f.agent, tk.ID, NoteInput{Body: "n", FileIDs: []int64{999999}}); !errors.As(err, &ve) || ve.Fields["file_ids"] == "" {
		t.Fatalf("unknown file: %v", err)
	}
	if n := f.count(t, `SELECT count(*) FROM thread_entry WHERE ticket_id = $1`, tk.ID); n != 2 {
		t.Fatalf("failed notes must roll back entries: %d", n)
	}
	fl2 := f.file(t, "c.txt")
	created, err := f.svc.Create(f.ctx, f.agent, CreateInput{Subject: "with file", Message: "m", RequesterEmail: "r@x.test", DeptID: &f.support.ID, FileIDs: []int64{fl2.ID}})
	if err != nil {
		t.Fatal(err)
	}
	th, _ := f.svc.Thread(f.ctx, f.agent, created.ID, 0, 10)
	if len(th.Items) != 1 || len(th.Items[0].Attachments) != 1 || th.Items[0].Attachments[0].FileID != fl2.ID {
		t.Fatalf("create with file: %+v", th)
	}
}

func TestEvents(t *testing.T) {
	f := newFixture(t)
	tk := f.create(t, f.agent, "Q", f.support.ID)
	if _, err := f.svc.SetStatus(f.ctx, f.agent, tk.ID, f.closed.ID); err != nil {
		t.Fatal(err)
	}
	events, err := f.svc.Events(f.ctx, f.agent, tk.ID)
	if err != nil || len(events) != 2 || events[0].Kind != "created" || events[1].Kind != "closed" {
		t.Fatalf("events: %+v %v", events, err)
	}
	if events[1].Staff == nil || events[1].Staff.Name != "agent Person" || string(events[1].Data) == "" {
		t.Fatalf("event staff/data: %+v", events[1])
	}
	if _, err := f.svc.Events(f.ctx, f.other, tk.ID); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("invisible events: %v", err)
	}
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./internal/ticket/ -run 'Reply|Note|Thread|Status|Assign|Transfer|Attach|Events' -v`
Expected: FAIL to compile, "f.svc.Reply undefined".

- [ ] **Step 4: Extend the Service interface**

In `gin/internal/ticket/service.go`, replace the `Service` interface with:

```go
type Service interface {
	Create(ctx context.Context, p auth.Principal, in CreateInput) (*Ticket, error)
	Get(ctx context.Context, p auth.Principal, id int64) (*Ticket, error)
	List(ctx context.Context, p auth.Principal, f ListFilter) (*httpx.List[Ticket], error)
	Update(ctx context.Context, p auth.Principal, id int64, in UpdateInput) (*Ticket, error)
	ListPriorities(ctx context.Context) ([]Priority, error)
	ListStatuses(ctx context.Context) ([]Status, error)

	Reply(ctx context.Context, p auth.Principal, id int64, in ReplyInput) (*Entry, error)
	Note(ctx context.Context, p auth.Principal, id int64, in NoteInput) (*Entry, error)
	Thread(ctx context.Context, p auth.Principal, id int64, after int64, limit int) (*Thread, error)
	SetStatus(ctx context.Context, p auth.Principal, id int64, statusID int64) (*Ticket, error)
	Assign(ctx context.Context, p auth.Principal, id int64, staffID *int64) (*Ticket, error)
	Transfer(ctx context.Context, p auth.Principal, id int64, deptID int64) (*Ticket, error)
	Events(ctx context.Context, p auth.Principal, id int64) ([]Event, error)
}
```

- [ ] **Step 5: Implement thread.go (replaces the stub)**

`gin/internal/ticket/thread.go`:

```go
package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/httpx"
	"github.com/jackc/pgx/v5"
)

type ReplyInput struct {
	Body     string  `json:"body" binding:"required"`
	Format   string  `json:"format" binding:"omitempty,oneof=html text"`
	StatusID *int64  `json:"status_id"`
	FileIDs  []int64 `json:"file_ids"`
}

type NoteInput struct {
	Title   string  `json:"title" binding:"max=255"`
	Body    string  `json:"body" binding:"required"`
	Format  string  `json:"format" binding:"omitempty,oneof=html text"`
	FileIDs []int64 `json:"file_ids"`
}

type StatusInput struct {
	StatusID int64 `json:"status_id" binding:"required"`
}

type AssignInput struct {
	StaffID *int64 `json:"staff_id"`
}

type TransferInput struct {
	DeptID int64 `json:"dept_id" binding:"required"`
}

type AttachmentRef struct {
	FileID int64  `json:"file_id"`
	Name   string `json:"name"`
	Mime   string `json:"mime"`
	Size   int64  `json:"size"`
}

type Entry struct {
	ID          int64           `json:"id"`
	TicketID    int64           `json:"ticket_id"`
	Type        string          `json:"type"`
	StaffID     *int64          `json:"staff_id"`
	Poster      string          `json:"poster"`
	Title       *string         `json:"title"`
	Body        string          `json:"body"`
	Format      string          `json:"format"`
	ParentID    *int64          `json:"parent_id"`
	Attachments []AttachmentRef `json:"attachments"`
	CreatedAt   time.Time       `json:"created_at"`
}

type Thread struct {
	Items     []Entry `json:"items"`
	NextAfter *int64  `json:"next_after"`
}

type Event struct {
	ID        int64           `json:"id"`
	TicketID  int64           `json:"ticket_id"`
	Staff     *Ref            `json:"staff"`
	Kind      string          `json:"kind"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
}

func (s *service) Reply(ctx context.Context, p auth.Principal, id int64, in ReplyInput) (*Entry, error) {
	var out *Entry
	err := db.WithTx(ctx, s.db, func(q *db.Queries) error {
		row, err := loadVisible(ctx, q, p, id)
		if err != nil {
			return err
		}
		st, err := q.GetStaff(ctx, p.StaffID)
		if err != nil {
			return err
		}
		entry, err := q.CreateThreadEntry(ctx, db.CreateThreadEntryParams{
			TicketID: id, Type: db.ThreadEntryTypeResponse, StaffID: &p.StaffID,
			Poster: fullName(&st.FirstName, &st.LastName), Body: in.Body, Format: bodyFormat(in.Format),
		})
		if err != nil {
			return err
		}
		if err := attachFiles(ctx, q, entry.ID, in.FileIDs); err != nil {
			return err
		}
		if err := q.MarkTicketAnswered(ctx, id); err != nil {
			return err
		}
		if in.StatusID != nil {
			if err := applyStatus(ctx, q, p, row, *in.StatusID); err != nil {
				return err
			}
		}
		out, err = entryWithAttachments(ctx, q, entry)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *service) Note(ctx context.Context, p auth.Principal, id int64, in NoteInput) (*Entry, error) {
	var out *Entry
	err := db.WithTx(ctx, s.db, func(q *db.Queries) error {
		if _, err := loadVisible(ctx, q, p, id); err != nil {
			return err
		}
		st, err := q.GetStaff(ctx, p.StaffID)
		if err != nil {
			return err
		}
		var title *string
		if in.Title != "" {
			title = &in.Title
		}
		entry, err := q.CreateThreadEntry(ctx, db.CreateThreadEntryParams{
			TicketID: id, Type: db.ThreadEntryTypeNote, StaffID: &p.StaffID, Title: title,
			Poster: fullName(&st.FirstName, &st.LastName), Body: in.Body, Format: bodyFormat(in.Format),
		})
		if err != nil {
			return err
		}
		if err := attachFiles(ctx, q, entry.ID, in.FileIDs); err != nil {
			return err
		}
		out, err = entryWithAttachments(ctx, q, entry)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *service) Thread(ctx context.Context, p auth.Principal, id int64, after int64, limit int) (*Thread, error) {
	q := db.New(s.db)
	if _, err := loadVisible(ctx, q, p, id); err != nil {
		return nil, err
	}
	rows, err := q.ListThreadEntries(ctx, db.ListThreadEntriesParams{TicketID: id, After: after, PageLimit: int32(limit + 1)})
	if err != nil {
		return nil, err
	}
	var next *int64
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1].ID
		next = &last
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	atts, err := q.ListAttachmentsForEntries(ctx, ids)
	if err != nil {
		return nil, err
	}
	byEntry := map[int64][]AttachmentRef{}
	for _, a := range atts {
		byEntry[a.ThreadEntryID] = append(byEntry[a.ThreadEntryID], AttachmentRef{FileID: a.FileID, Name: a.Name, Mime: a.Mime, Size: a.Size})
	}
	items := make([]Entry, 0, len(rows))
	for _, r := range rows {
		e := toEntry(r)
		if list := byEntry[r.ID]; list != nil {
			e.Attachments = list
		}
		items = append(items, e)
	}
	return &Thread{Items: items, NextAfter: next}, nil
}

func (s *service) SetStatus(ctx context.Context, p auth.Principal, id int64, statusID int64) (*Ticket, error) {
	return s.mutate(ctx, p, id, func(q *db.Queries, row db.GetTicketRow) error {
		return applyStatus(ctx, q, p, row, statusID)
	})
}

func (s *service) Assign(ctx context.Context, p auth.Principal, id int64, staffID *int64) (*Ticket, error) {
	return s.mutate(ctx, p, id, func(q *db.Queries, row db.GetTicketRow) error {
		if staffID == nil {
			if err := q.SetTicketAssignee(ctx, db.SetTicketAssigneeParams{ID: id, AssignedStaffID: nil}); err != nil {
				return err
			}
			return event(ctx, q, id, &p.StaffID, db.TicketEventKindUnassigned, nil)
		}
		st, err := q.GetStaff(ctx, *staffID)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Validation("staff_id", "unknown staff")
		}
		if err != nil {
			return err
		}
		if !st.IsActive {
			return apperr.Validation("staff_id", "staff is inactive")
		}
		can, err := q.StaffCanSeeDept(ctx, db.StaffCanSeeDeptParams{StaffID: *staffID, DeptID: row.DeptID})
		if err != nil {
			return err
		}
		if !can {
			return apperr.Validation("staff_id", "staff cannot see the ticket's department")
		}
		if err := q.SetTicketAssignee(ctx, db.SetTicketAssigneeParams{ID: id, AssignedStaffID: staffID}); err != nil {
			return err
		}
		return event(ctx, q, id, &p.StaffID, db.TicketEventKindAssigned, map[string]any{"staff_id": *staffID})
	})
}

func (s *service) Transfer(ctx context.Context, p auth.Principal, id int64, deptID int64) (*Ticket, error) {
	return s.mutate(ctx, p, id, func(q *db.Queries, row db.GetTicketRow) error {
		if _, err := q.GetDepartment(ctx, deptID); errors.Is(err, pgx.ErrNoRows) {
			return apperr.Validation("dept_id", "unknown department")
		} else if err != nil {
			return err
		}
		if !p.CanSeeDept(deptID) {
			return fmt.Errorf("%w: cannot transfer to that department", apperr.ErrForbidden)
		}
		if deptID == row.DeptID {
			return nil
		}
		if err := q.SetTicketDept(ctx, db.SetTicketDeptParams{ID: id, DeptID: deptID}); err != nil {
			return err
		}
		if row.AssignedStaffID != nil {
			can, err := q.StaffCanSeeDept(ctx, db.StaffCanSeeDeptParams{StaffID: *row.AssignedStaffID, DeptID: deptID})
			if err != nil {
				return err
			}
			if !can {
				if err := q.SetTicketAssignee(ctx, db.SetTicketAssigneeParams{ID: id, AssignedStaffID: nil}); err != nil {
					return err
				}
				if err := event(ctx, q, id, &p.StaffID, db.TicketEventKindUnassigned, map[string]any{"reason": "transfer"}); err != nil {
					return err
				}
			}
		}
		return event(ctx, q, id, &p.StaffID, db.TicketEventKindTransferred, map[string]any{"from": row.DeptID, "to": deptID})
	})
}

func (s *service) Events(ctx context.Context, p auth.Principal, id int64) ([]Event, error) {
	q := db.New(s.db)
	if _, err := loadVisible(ctx, q, p, id); err != nil {
		return nil, err
	}
	rows, err := q.ListTicketEvents(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(rows))
	for _, r := range rows {
		e := Event{ID: r.ID, TicketID: r.TicketID, Kind: string(r.Kind), Data: json.RawMessage(r.Data), CreatedAt: r.CreatedAt.UTC()}
		if r.StaffID != nil {
			e.Staff = &Ref{ID: *r.StaffID, Name: fullName(r.StaffFirstName, r.StaffLastName)}
		}
		out = append(out, e)
	}
	return out, nil
}

// mutate loads the visible ticket, runs fn in a transaction, and returns the fresh ticket.
func (s *service) mutate(ctx context.Context, p auth.Principal, id int64, fn func(q *db.Queries, row db.GetTicketRow) error) (*Ticket, error) {
	var out *Ticket
	err := db.WithTx(ctx, s.db, func(q *db.Queries) error {
		row, err := loadVisible(ctx, q, p, id)
		if err != nil {
			return err
		}
		if err := fn(q, row); err != nil {
			return err
		}
		out, err = get(ctx, q, p, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func applyStatus(ctx context.Context, q *db.Queries, p auth.Principal, row db.GetTicketRow, statusID int64) error {
	if statusID == row.StatusID {
		return nil
	}
	ns, err := q.GetStatus(ctx, statusID)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.Validation("status_id", "unknown status")
	}
	if err != nil {
		return err
	}
	closedAt := row.ClosedAt
	kind := db.TicketEventKindStatusChanged
	switch {
	case row.StatusState == db.TicketStateOpen && ns.State != db.TicketStateOpen:
		now := time.Now().UTC()
		closedAt = &now
		kind = db.TicketEventKindClosed
	case row.StatusState != db.TicketStateOpen && ns.State == db.TicketStateOpen:
		closedAt = nil
		kind = db.TicketEventKindReopened
	}
	if err := q.SetTicketStatus(ctx, db.SetTicketStatusParams{ID: row.ID, StatusID: statusID, ClosedAt: closedAt}); err != nil {
		return err
	}
	return event(ctx, q, row.ID, &p.StaffID, kind, map[string]any{"from": row.StatusID, "to": statusID})
}

func attachFiles(ctx context.Context, q *db.Queries, entryID int64, fileIDs []int64) error {
	for _, fid := range fileIDs {
		if _, err := q.GetFile(ctx, fid); errors.Is(err, pgx.ErrNoRows) {
			return apperr.Validation("file_ids", "unknown file "+strconv.FormatInt(fid, 10))
		} else if err != nil {
			return err
		}
		attached, err := q.IsFileAttached(ctx, fid)
		if err != nil {
			return err
		}
		if attached {
			return fmt.Errorf("%w: file %d is already attached", apperr.ErrConflict, fid)
		}
		if err := q.CreateAttachment(ctx, db.CreateAttachmentParams{ThreadEntryID: entryID, FileID: fid}); err != nil {
			return err
		}
	}
	return nil
}

func entryWithAttachments(ctx context.Context, q *db.Queries, r db.ThreadEntry) (*Entry, error) {
	e := toEntry(r)
	atts, err := q.ListAttachmentsForEntries(ctx, []int64{r.ID})
	if err != nil {
		return nil, err
	}
	for _, a := range atts {
		e.Attachments = append(e.Attachments, AttachmentRef{FileID: a.FileID, Name: a.Name, Mime: a.Mime, Size: a.Size})
	}
	return &e, nil
}

func toEntry(r db.ThreadEntry) Entry {
	return Entry{
		ID: r.ID, TicketID: r.TicketID, Type: string(r.Type), StaffID: r.StaffID, Poster: r.Poster,
		Title: r.Title, Body: r.Body, Format: string(r.Format), ParentID: r.ParentID,
		Attachments: []AttachmentRef{}, CreatedAt: r.CreatedAt.UTC(),
	}
}

// Handlers

func (h *Handler) mountThread(private *gin.RouterGroup) {
	private.POST("/tickets/:id/reply", h.reply)
	private.POST("/tickets/:id/notes", h.note)
	private.GET("/tickets/:id/thread", h.thread)
	private.POST("/tickets/:id/status", h.setStatus)
	private.POST("/tickets/:id/assign", h.assign)
	private.POST("/tickets/:id/transfer", h.transfer)
	private.GET("/tickets/:id/events", h.events)
}

func (h *Handler) reply(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in ReplyInput
	if !httpx.BindJSON(c, &in) {
		return
	}
	out, err := h.svc.Reply(c.Request.Context(), p, id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}

func (h *Handler) note(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in NoteInput
	if !httpx.BindJSON(c, &in) {
		return
	}
	out, err := h.svc.Note(c.Request.Context(), p, id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}

func (h *Handler) thread(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	after, limit := int64(0), 50
	if v := c.Query("after"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			httpx.Fail(c, apperr.Validation("after", "must be a non-negative integer"))
			return
		}
		after = n
	}
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			httpx.Fail(c, apperr.Validation("limit", "must be between 1 and 200"))
			return
		}
		limit = n
	}
	out, err := h.svc.Thread(c.Request.Context(), p, id, after, limit)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) setStatus(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in StatusInput
	if !httpx.BindJSON(c, &in) {
		return
	}
	out, err := h.svc.SetStatus(c.Request.Context(), p, id, in.StatusID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) assign(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in AssignInput
	if !httpx.BindJSON(c, &in) {
		return
	}
	out, err := h.svc.Assign(c.Request.Context(), p, id, in.StaffID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) transfer(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in TransferInput
	if !httpx.BindJSON(c, &in) {
		return
	}
	out, err := h.svc.Transfer(c.Request.Context(), p, id, in.DeptID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) events(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Events(c.Request.Context(), p, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}
```

Remove the trailing comment about `attachFiles` from the end of `service.go`.

- [ ] **Step 6: Run service tests**

Run: `go test ./internal/ticket/ -v`
Expected: the new tests PASS; `handler_test.go` fails to compile because `fakeSvc` no longer satisfies `Service`. Fix in the next step.

- [ ] **Step 7: Extend the fake and add handler tests**

Append to `fakeSvc` in `gin/internal/ticket/handler_test.go`:

```go
func (f *fakeSvc) Reply(_ context.Context, p auth.Principal, id int64, in ReplyInput) (*Entry, error) {
	return &Entry{ID: 10, TicketID: id, Type: "response", Body: in.Body}, nil
}
func (f *fakeSvc) Note(_ context.Context, p auth.Principal, id int64, in NoteInput) (*Entry, error) {
	return &Entry{ID: 11, TicketID: id, Type: "note", Body: in.Body}, nil
}
func (f *fakeSvc) Thread(_ context.Context, p auth.Principal, id int64, after int64, limit int) (*Thread, error) {
	f.lastAfter, f.lastLimit = after, limit
	return &Thread{Items: []Entry{}}, nil
}
func (f *fakeSvc) SetStatus(_ context.Context, p auth.Principal, id int64, statusID int64) (*Ticket, error) {
	if statusID == 99 {
		return nil, apperr.Validation("status_id", "unknown status")
	}
	return &Ticket{ID: id}, nil
}
func (f *fakeSvc) Assign(_ context.Context, p auth.Principal, id int64, staffID *int64) (*Ticket, error) {
	f.lastAssign = staffID
	return &Ticket{ID: id}, nil
}
func (f *fakeSvc) Transfer(_ context.Context, p auth.Principal, id int64, deptID int64) (*Ticket, error) {
	return &Ticket{ID: id}, nil
}
func (f *fakeSvc) Events(_ context.Context, p auth.Principal, id int64) ([]Event, error) {
	return []Event{{ID: 1, Kind: "created"}}, nil
}
```

and add fields `lastAfter int64`, `lastLimit int`, `lastAssign *int64` to the `fakeSvc` struct.

`gin/internal/ticket/thread_handler_test.go`:

```go
package ticket

import (
	"net/http"
	"strings"
	"testing"
)

func TestThreadRoutes(t *testing.T) {
	f := &fakeSvc{}
	r := newRouter(f)
	if w := do(r, http.MethodPost, "/api/v1/tickets/1/reply", `{}`); w.Code != 400 {
		t.Fatalf("reply without body: %d", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/tickets/1/reply", `{"body":"hi","format":"markdown"}`); w.Code != 400 {
		t.Fatalf("reply bad format: %d", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/tickets/1/reply", `{"body":"hi","file_ids":[1,2]}`); w.Code != 201 || !strings.Contains(w.Body.String(), `"response"`) {
		t.Fatalf("reply: %d %s", w.Code, w.Body.String())
	}
	if w := do(r, http.MethodPost, "/api/v1/tickets/1/notes", `{"body":"n"}`); w.Code != 201 {
		t.Fatalf("note: %d", w.Code)
	}
	if w := do(r, http.MethodGet, "/api/v1/tickets/1/thread?after=5&limit=20", ""); w.Code != 200 || f.lastAfter != 5 || f.lastLimit != 20 {
		t.Fatalf("thread params: %d after=%d limit=%d", w.Code, f.lastAfter, f.lastLimit)
	}
	if w := do(r, http.MethodGet, "/api/v1/tickets/1/thread", ""); w.Code != 200 || f.lastLimit != 50 {
		t.Fatalf("thread defaults: %d limit=%d", w.Code, f.lastLimit)
	}
	if w := do(r, http.MethodGet, "/api/v1/tickets/1/thread?limit=500", ""); w.Code != 400 {
		t.Fatalf("thread limit too big: %d", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/tickets/1/status", `{}`); w.Code != 400 {
		t.Fatalf("status missing: %d", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/tickets/1/status", `{"status_id":99}`); w.Code != 400 {
		t.Fatalf("status unknown: %d", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/tickets/1/status", `{"status_id":2}`); w.Code != 200 {
		t.Fatalf("status: %d", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/tickets/1/assign", `{"staff_id":null}`); w.Code != 200 || f.lastAssign != nil {
		t.Fatalf("unassign: %d %v", w.Code, f.lastAssign)
	}
	if w := do(r, http.MethodPost, "/api/v1/tickets/1/assign", `{"staff_id":4}`); w.Code != 200 || f.lastAssign == nil || *f.lastAssign != 4 {
		t.Fatalf("assign: %d %v", w.Code, f.lastAssign)
	}
	if w := do(r, http.MethodPost, "/api/v1/tickets/1/transfer", `{}`); w.Code != 400 {
		t.Fatalf("transfer missing dept: %d", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/tickets/1/transfer", `{"dept_id":2}`); w.Code != 200 {
		t.Fatalf("transfer: %d", w.Code)
	}
	if w := do(r, http.MethodGet, "/api/v1/tickets/1/events", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "created") {
		t.Fatalf("events: %d %s", w.Code, w.Body.String())
	}
}
```

- [ ] **Step 8: Run all ticket tests and commit**

Run: `go test ./internal/ticket/ -v`
Expected: PASS (17 tests).

```bash
git add gin
git commit -m "feat(gin): ticket thread, replies, notes, status, assignment, transfer and events" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 11: Attachments (upload, download, storage, garbage collection)

**Files:**
- Modify: `gin/db/queries/file.sql` (append queries)
- Create: `gin/internal/attachment/storage.go`, `service.go`, `handler.go`
- Create: `gin/internal/attachment/storage_test.go`, `service_test.go`, `handler_test.go`

**Interfaces:**
- Consumes: `auth.Principal`, `auth.FromContext`, `httpx.*`, `apperr.*`, `db.CreateFile`, `db.GetFile`.
- Produces: `attachment.Storage` interface `Put(ctx, key string, r io.Reader) (size int64, sha256Hex string, err error)`, `Open(ctx, key string) (io.ReadCloser, error)`, `Delete(ctx, key string) error`; `attachment.NewLocalStorage(dir string) (*LocalStorage, error)`; `attachment.File{ID int64; Name, Mime string; Size int64}`; `attachment.Service` interface `Upload(ctx, p auth.Principal, name, mime string, r io.Reader) (*File, error)`, `Download(ctx, p, id) (*File, io.ReadCloser, error)`, `GC(ctx, olderThan time.Duration) (int, error)`; `attachment.NewService(b db.Beginner, store Storage, maxBytes int64, allowedMIME []string) Service`; `attachment.NewHandler(svc Service, maxBytes int64) *Handler` with `Mount(private)`.
- Queries: `FileTicketDeptID`, `ListUnattachedFilesBefore`, `DeleteFile`.

- [ ] **Step 1: Add queries and regenerate**

Append to `gin/db/queries/file.sql`:

```sql
-- name: FileTicketDeptID :one
SELECT t.dept_id
FROM attachment a
JOIN thread_entry te ON te.id = a.thread_entry_id
JOIN ticket t ON t.id = te.ticket_id
WHERE a.file_id = $1
LIMIT 1;

-- name: ListUnattachedFilesBefore :many
SELECT f.* FROM file f
WHERE f.created_at < $1
  AND NOT EXISTS (SELECT 1 FROM attachment a WHERE a.file_id = f.id)
ORDER BY f.id;

-- name: DeleteFile :exec
DELETE FROM file WHERE id = $1;
```

Run: `make sqlc && go build ./...`

- [ ] **Step 2: Write the failing storage test**

`gin/internal/attachment/storage_test.go`:

```go
package attachment

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
)

func TestLocalStorageRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := NewLocalStorage(dir)
	if err != nil {
		t.Fatal(err)
	}
	size, sum, err := st.Put(ctx, "abc123", strings.NewReader("hello"))
	if err != nil || size != 5 || sum != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
		t.Fatalf("put: %d %s %v", size, sum, err)
	}
	rc, err := st.Open(ctx, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if string(b) != "hello" {
		t.Fatalf("read back %q", b)
	}
	if err := st.Delete(ctx, "abc123"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Open(ctx, "abc123"); err == nil {
		t.Fatal("open after delete must fail")
	}
	if _, _, err := st.Put(ctx, "../escape", strings.NewReader("x")); err == nil {
		t.Fatal("keys with path separators must be rejected")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 3: Implement storage**

`gin/internal/attachment/storage.go`:

```go
// Package attachment stores uploaded files and links them to thread entries.
package attachment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

type Storage interface {
	Put(ctx context.Context, key string, r io.Reader) (size int64, sha256Hex string, err error)
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

var keyRe = regexp.MustCompile(`^[a-f0-9]{16,64}$`)

// LocalStorage keeps files under dir/<first two chars of key>/<key>.
type LocalStorage struct{ dir string }

func NewLocalStorage(dir string) (*LocalStorage, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("storage dir: %w", err)
	}
	return &LocalStorage{dir: dir}, nil
}

func (s *LocalStorage) path(key string) (string, error) {
	if !keyRe.MatchString(key) {
		return "", errors.New("invalid storage key")
	}
	return filepath.Join(s.dir, key[:2], key), nil
}

func (s *LocalStorage) Put(_ context.Context, key string, r io.Reader) (int64, string, error) {
	p, err := s.path(key)
	if err != nil {
		return 0, "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return 0, "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".upload-*")
	if err != nil {
		return 0, "", err
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), r)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return 0, "", err
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

func (s *LocalStorage) Open(_ context.Context, key string) (io.ReadCloser, error) {
	p, err := s.path(key)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}

func (s *LocalStorage) Delete(_ context.Context, key string) error {
	p, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
```

Run: `go test ./internal/attachment/ -run Storage -v`
Expected: PASS.

- [ ] **Step 4: Write the failing service tests**

`gin/internal/attachment/service_test.go`:

```go
package attachment

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/db/testutil"
	"github.com/jackc/pgx/v5"
)

type fx struct {
	ctx     context.Context
	tx      pgx.Tx
	q       *db.Queries
	svc     Service
	store   *LocalStorage
	support db.Department
	billing db.Department
	agent   auth.Principal
	other   auth.Principal
	admin   auth.Principal
}

func newFixture(t *testing.T) *fx {
	t.Helper()
	ctx := context.Background()
	tx := testutil.Tx(t)
	q := db.New(tx)
	store, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := &fx{ctx: ctx, tx: tx, q: q, store: store}
	f.svc = NewService(tx, store, 10, []string{"text/plain", "image/png"})
	f.support, _ = q.FirstDepartment(ctx)
	f.billing, _ = q.CreateDepartment(ctx, db.CreateDepartmentParams{Name: "Billing", IsPublic: true})
	mk := func(name string, admin bool, dept int64) int64 {
		st, err := q.CreateStaff(ctx, db.CreateStaffParams{Username: name, Email: name + "@x.test", PasswordHash: "h", IsAdmin: admin, IsActive: true, PrimaryDeptID: dept})
		if err != nil {
			t.Fatal(err)
		}
		return st.ID
	}
	f.agent = auth.Principal{StaffID: mk("agent", false, f.support.ID), DeptIDs: []int64{f.support.ID}}
	f.other = auth.Principal{StaffID: mk("other", false, f.billing.ID), DeptIDs: []int64{f.billing.ID}}
	f.admin = auth.Principal{StaffID: mk("admin", true, f.support.ID), IsAdmin: true}
	return f
}

// attachToTicket creates a ticket in dept with one message entry and links fileID to it.
func (f *fx) attachToTicket(t *testing.T, fileID, dept int64) {
	t.Helper()
	status, _ := f.q.DefaultStatus(f.ctx)
	prio, _ := f.q.DefaultPriority(f.ctx)
	n, _ := f.q.NextTicketNumber(f.ctx)
	id, err := f.q.CreateTicket(f.ctx, db.CreateTicketParams{Number: fmt.Sprintf("%06d", n), Subject: "s", StatusID: status.ID, DeptID: dept, PriorityID: prio.ID, RequesterEmail: "r@x.test", Source: db.TicketSourceWeb, Extra: []byte("{}")})
	if err != nil {
		t.Fatal(err)
	}
	entry, err := f.q.CreateThreadEntry(f.ctx, db.CreateThreadEntryParams{TicketID: id, Type: db.ThreadEntryTypeMessage, Poster: "r", Body: "b", Format: db.BodyFormatHtml})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.q.CreateAttachment(f.ctx, db.CreateAttachmentParams{ThreadEntryID: entry.ID, FileID: fileID}); err != nil {
		t.Fatal(err)
	}
}

func TestUploadRules(t *testing.T) {
	f := newFixture(t)
	fl, err := f.svc.Upload(f.ctx, f.agent, "notes.txt", "text/plain", strings.NewReader("0123456789"))
	if err != nil || fl.Size != 10 || fl.Name != "notes.txt" || fl.Mime != "text/plain" {
		t.Fatalf("upload: %+v %v", fl, err)
	}
	if _, err := f.svc.Upload(f.ctx, f.agent, "big.txt", "text/plain", strings.NewReader("01234567890")); !errors.Is(err, apperr.ErrPayloadTooLarge) {
		t.Fatalf("too large: %v", err)
	}
	var ve *apperr.ValidationError
	if _, err := f.svc.Upload(f.ctx, f.agent, "x.exe", "application/octet-stream", strings.NewReader("x")); !errors.As(err, &ve) || ve.Fields["file"] == "" {
		t.Fatalf("disallowed mime: %v", err)
	}
	if _, err := f.svc.Upload(f.ctx, f.agent, "", "text/plain", strings.NewReader("x")); !errors.As(err, &ve) || ve.Fields["file"] == "" {
		t.Fatalf("empty name: %v", err)
	}
	var n int
	_ = f.tx.QueryRow(f.ctx, `SELECT count(*) FROM file`).Scan(&n)
	if n != 1 {
		t.Fatalf("rejected uploads must not leave rows: %d", n)
	}
}

func TestDownloadAuthorization(t *testing.T) {
	f := newFixture(t)
	fl, err := f.svc.Upload(f.ctx, f.agent, "a.txt", "text/plain", strings.NewReader("abc"))
	if err != nil {
		t.Fatal(err)
	}
	// unattached: uploader and admin may read, others may not
	meta, rc, err := f.svc.Download(f.ctx, f.agent, fl.ID)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if string(b) != "abc" || meta.Name != "a.txt" {
		t.Fatalf("uploader download: %q %+v", b, meta)
	}
	if _, _, err := f.svc.Download(f.ctx, f.other, fl.ID); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("other on unattached: %v", err)
	}
	if _, rc, err := f.svc.Download(f.ctx, f.admin, fl.ID); err != nil {
		t.Fatalf("admin on unattached: %v", err)
	} else {
		rc.Close()
	}
	// attached to a billing ticket: billing agent may read, support uploader may not
	f.attachToTicket(t, fl.ID, f.billing.ID)
	if _, rc, err := f.svc.Download(f.ctx, f.other, fl.ID); err != nil {
		t.Fatalf("dept member on attached: %v", err)
	} else {
		rc.Close()
	}
	if _, _, err := f.svc.Download(f.ctx, f.agent, fl.ID); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("uploader outside dept on attached: %v", err)
	}
	if _, _, err := f.svc.Download(f.ctx, f.admin, 999999); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("missing file: %v", err)
	}
}

func TestGC(t *testing.T) {
	f := newFixture(t)
	old, _ := f.svc.Upload(f.ctx, f.agent, "old.txt", "text/plain", strings.NewReader("1"))
	kept, _ := f.svc.Upload(f.ctx, f.agent, "kept.txt", "text/plain", strings.NewReader("2"))
	fresh, _ := f.svc.Upload(f.ctx, f.agent, "fresh.txt", "text/plain", strings.NewReader("3"))
	if _, err := f.tx.Exec(f.ctx, `UPDATE file SET created_at = now() - interval '2 days' WHERE id IN ($1, $2)`, old.ID, kept.ID); err != nil {
		t.Fatal(err)
	}
	f.attachToTicket(t, kept.ID, f.support.ID)
	n, err := f.svc.GC(f.ctx, 24*time.Hour)
	if err != nil || n != 1 {
		t.Fatalf("gc: %d %v", n, err)
	}
	if _, err := f.q.GetFile(f.ctx, old.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("old row must be gone: %v", err)
	}
	for _, id := range []int64{kept.ID, fresh.ID} {
		if _, err := f.q.GetFile(f.ctx, id); err != nil {
			t.Fatalf("file %d must survive: %v", id, err)
		}
	}
	row, _ := f.q.GetFile(f.ctx, kept.ID)
	if _, err := f.store.Open(f.ctx, row.Key); err != nil {
		t.Fatalf("kept blob must exist: %v", err)
	}
}
```

- [ ] **Step 5: Run to verify it fails**

Run: `go test ./internal/attachment/ -v`
Expected: FAIL to compile, "undefined: NewService".

- [ ] **Step 6: Implement the service**

`gin/internal/attachment/service.go`:

```go
package attachment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/jackc/pgx/v5"
)

type File struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Mime string `json:"mime"`
	Size int64  `json:"size"`
}

type Service interface {
	Upload(ctx context.Context, p auth.Principal, name, mime string, r io.Reader) (*File, error)
	Download(ctx context.Context, p auth.Principal, id int64) (*File, io.ReadCloser, error)
	GC(ctx context.Context, olderThan time.Duration) (int, error)
}

type service struct {
	db       db.Beginner
	store    Storage
	maxBytes int64
	allowed  map[string]bool
}

func NewService(b db.Beginner, store Storage, maxBytes int64, allowedMIME []string) Service {
	allowed := make(map[string]bool, len(allowedMIME))
	for _, m := range allowedMIME {
		allowed[strings.ToLower(strings.TrimSpace(m))] = true
	}
	return &service{db: b, store: store, maxBytes: maxBytes, allowed: allowed}
}

func (s *service) Upload(ctx context.Context, p auth.Principal, name, mime string, r io.Reader) (*File, error) {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "" || name == "." || name == "/" {
		return nil, apperr.Validation("file", "filename is required")
	}
	if len(name) > 255 {
		name = name[len(name)-255:]
	}
	mime = strings.ToLower(strings.TrimSpace(strings.Split(mime, ";")[0]))
	if !s.allowed[mime] {
		return nil, apperr.Validation("file", "file type "+mime+" is not allowed")
	}
	var kb [16]byte
	if _, err := rand.Read(kb[:]); err != nil {
		return nil, err
	}
	key := hex.EncodeToString(kb[:])
	size, sum, err := s.store.Put(ctx, key, io.LimitReader(r, s.maxBytes+1))
	if err != nil {
		return nil, err
	}
	if size > s.maxBytes {
		_ = s.store.Delete(ctx, key)
		return nil, fmt.Errorf("%w: file exceeds %d bytes", apperr.ErrPayloadTooLarge, s.maxBytes)
	}
	uploader := p.StaffID
	row, err := db.New(s.db).CreateFile(ctx, db.CreateFileParams{
		Key: key, Name: name, Mime: mime, Size: size, Sha256: sum, Backend: "local", UploadedBy: &uploader,
	})
	if err != nil {
		_ = s.store.Delete(ctx, key)
		return nil, err
	}
	return &File{ID: row.ID, Name: row.Name, Mime: row.Mime, Size: row.Size}, nil
}

func (s *service) Download(ctx context.Context, p auth.Principal, id int64) (*File, io.ReadCloser, error) {
	q := db.New(s.db)
	row, err := q.GetFile(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, fmt.Errorf("file %d: %w", id, apperr.ErrNotFound)
	}
	if err != nil {
		return nil, nil, err
	}
	deptID, err := q.FileTicketDeptID(ctx, id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// not attached yet: only the uploader or an admin may read it
		if !p.IsAdmin && (row.UploadedBy == nil || *row.UploadedBy != p.StaffID) {
			return nil, nil, fmt.Errorf("file %d: %w", id, apperr.ErrNotFound)
		}
	case err != nil:
		return nil, nil, err
	default:
		if !p.CanSeeDept(deptID) {
			return nil, nil, fmt.Errorf("file %d: %w", id, apperr.ErrNotFound)
		}
	}
	rc, err := s.store.Open(ctx, row.Key)
	if err != nil {
		return nil, nil, fmt.Errorf("open blob %s: %w", row.Key, err)
	}
	return &File{ID: row.ID, Name: row.Name, Mime: row.Mime, Size: row.Size}, rc, nil
}

func (s *service) GC(ctx context.Context, olderThan time.Duration) (int, error) {
	q := db.New(s.db)
	rows, err := q.ListUnattachedFilesBefore(ctx, time.Now().Add(-olderThan))
	if err != nil {
		return 0, err
	}
	n := 0
	for _, row := range rows {
		if err := s.store.Delete(ctx, row.Key); err != nil {
			return n, fmt.Errorf("delete blob %s: %w", row.Key, err)
		}
		if err := q.DeleteFile(ctx, row.ID); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
```

- [ ] **Step 7: Run service tests**

Run: `go test ./internal/attachment/ -v`
Expected: PASS (4 tests).

- [ ] **Step 8: Write the failing handler tests**

`gin/internal/attachment/handler_test.go`:

```go
package attachment

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
)

type fake struct {
	lastName string
	lastMime string
}

func (f *fake) Upload(_ context.Context, p auth.Principal, name, mime string, r io.Reader) (*File, error) {
	f.lastName, f.lastMime = name, mime
	b, _ := io.ReadAll(r)
	if mime != "text/plain" {
		return nil, apperr.Validation("file", "not allowed")
	}
	return &File{ID: 3, Name: name, Mime: mime, Size: int64(len(b))}, nil
}
func (f *fake) Download(_ context.Context, p auth.Principal, id int64) (*File, io.ReadCloser, error) {
	if id != 3 {
		return nil, nil, apperr.ErrNotFound
	}
	return &File{ID: 3, Name: "../../evil name.txt", Mime: "text/plain", Size: 5}, io.NopCloser(strings.NewReader("hello")), nil
}
func (f *fake) GC(context.Context, time.Duration) (int, error) { return 0, nil }

func newRouter(f *fake) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	private := r.Group("/api/v1", func(c *gin.Context) {
		auth.WithPrincipal(c, auth.Principal{StaffID: 1})
	})
	NewHandler(f, 1<<20).Mount(private)
	return r
}

func multipartBody(t *testing.T, field, filename, mime, content string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{`form-data; name="` + field + `"; filename="` + filename + `"`}
	h["Content-Type"] = []string{mime}
	part, err := w.CreatePart(h)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte(content))
	_ = w.Close()
	return &buf, w.FormDataContentType()
}

func TestUploadHandler(t *testing.T) {
	f := &fake{}
	r := newRouter(f)
	body, ctype := multipartBody(t, "file", "notes.txt", "text/plain", "hello")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/files", body)
	req.Header.Set("Content-Type", ctype)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 201 || !strings.Contains(w.Body.String(), `"id":3`) || f.lastName != "notes.txt" || f.lastMime != "text/plain" {
		t.Fatalf("upload: %d %s name=%q mime=%q", w.Code, w.Body.String(), f.lastName, f.lastMime)
	}
	body, ctype = multipartBody(t, "wrong", "notes.txt", "text/plain", "hello")
	req = httptest.NewRequest(http.MethodPost, "/api/v1/files", body)
	req.Header.Set("Content-Type", ctype)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("missing file field: %d", w.Code)
	}
	body, ctype = multipartBody(t, "file", "x.bin", "application/octet-stream", "hello")
	req = httptest.NewRequest(http.MethodPost, "/api/v1/files", body)
	req.Header.Set("Content-Type", ctype)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("disallowed mime: %d", w.Code)
	}
}

func TestDownloadHandler(t *testing.T) {
	r := newRouter(&fake{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/files/3", nil))
	if w.Code != 200 || w.Body.String() != "hello" {
		t.Fatalf("download: %d %q", w.Code, w.Body.String())
	}
	cd := w.Header().Get("Content-Disposition")
	if !strings.Contains(cd, `filename="evil name.txt"`) || strings.Contains(cd, "..") {
		t.Fatalf("content-disposition must use base name: %q", cd)
	}
	if w.Header().Get("Content-Type") != "text/plain" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("headers: %v", w.Header())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/files/4", nil))
	if w.Code != 404 {
		t.Fatalf("missing: %d", w.Code)
	}
}
```

- [ ] **Step 9: Implement the handler**

`gin/internal/attachment/handler.go`:

```go
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
```

- [ ] **Step 10: Run all attachment tests and commit**

Run: `go test ./internal/attachment/ -v`
Expected: PASS (6 tests).

```bash
git add gin
git commit -m "feat(gin): file upload, authorized download and unattached-file GC" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 12: Wire everything in main.go, subcommands, coverage gate, README

**Files:**
- Modify: `gin/cmd/api/main.go` (full wiring; replaces the Task 2 version)
- Create: `gin/README.md`

**Interfaces:**
- Consumes: every `NewService`, `NewHandler`, `Mount`, `auth.RequireAuth`, `auth.CreateAdmin`, `attachment.NewLocalStorage`, `db.NewPool`, `server.New`.
- Produces: the runnable binary with `serve`, `create-admin`, `gc-files`.

- [ ] **Step 1: Replace main.go**

`gin/cmd/api/main.go`:

```go
// Command api runs the ticket API server and its maintenance subcommands.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/attachment"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/config"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/dept"
	"github.com/grandpine/ticket-api/internal/server"
	"github.com/grandpine/ticket-api/internal/staff"
	"github.com/grandpine/ticket-api/internal/ticket"
	"github.com/grandpine/ticket-api/internal/topic"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	accessTTL  = 15 * time.Minute
	refreshTTL = 14 * 24 * time.Hour
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
		args = args[1:]
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	switch cmd {
	case "serve":
		return serve(ctx, cfg, pool)
	case "create-admin":
		return createAdmin(ctx, pool, args)
	case "gc-files":
		return gcFiles(ctx, cfg, pool, args)
	default:
		return fmt.Errorf("unknown command %q (expected serve, create-admin or gc-files)", cmd)
	}
}

func serve(ctx context.Context, cfg config.Config, pool *pgxpool.Pool) error {
	store, err := attachment.NewLocalStorage(cfg.StorageDir)
	if err != nil {
		return err
	}
	tokens := auth.NewTokens(cfg.JWTSecret, accessTTL)
	authSvc := auth.NewService(pool, tokens, refreshTTL)
	authH := auth.NewHandler(authSvc)
	deptH := dept.NewHandler(dept.NewService(pool))
	topicH := topic.NewHandler(topic.NewService(pool))
	staffH := staff.NewHandler(staff.NewService(pool))
	ticketH := ticket.NewHandler(ticket.NewService(pool))
	fileH := attachment.NewHandler(attachment.NewService(pool, store, cfg.MaxUploadBytes, cfg.AllowedMIME), cfg.MaxUploadBytes)

	engine := server.New(server.Options{
		Pinger:      pool,
		CORSOrigins: cfg.CORSOrigins,
		RequireAuth: auth.RequireAuth(tokens, authSvc),
		Mount: func(public, private *gin.RouterGroup) {
			authH.Mount(public, private)
			deptH.Mount(private)
			topicH.Mount(private)
			staffH.Mount(private)
			ticketH.Mount(private)
			fileH.Mount(private)
		},
	})
	srv := &http.Server{
		Addr: fmt.Sprintf(":%d", cfg.Port), Handler: engine,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	slog.Info("listening", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func createAdmin(ctx context.Context, pool *pgxpool.Pool, args []string) error {
	fs := flag.NewFlagSet("create-admin", flag.ContinueOnError)
	username := fs.String("username", "", "admin username")
	email := fs.String("email", "", "admin email")
	password := fs.String("password", "", "admin password (min 8 chars)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *username == "" || *email == "" || *password == "" {
		return errors.New("create-admin requires --username, --email and --password")
	}
	id, err := auth.CreateAdmin(ctx, pool, *username, *email, *password)
	if err != nil {
		return err
	}
	fmt.Printf("created admin %s with id %d\n", *username, id)
	return nil
}

func gcFiles(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, args []string) error {
	fs := flag.NewFlagSet("gc-files", flag.ContinueOnError)
	olderThan := fs.Duration("older-than", 24*time.Hour, "delete unattached files older than this")
	if err := fs.Parse(args); err != nil {
		return err
	}
	store, err := attachment.NewLocalStorage(cfg.StorageDir)
	if err != nil {
		return err
	}
	svc := attachment.NewService(pool, store, cfg.MaxUploadBytes, cfg.AllowedMIME)
	n, err := svc.GC(ctx, *olderThan)
	if err != nil {
		return err
	}
	fmt.Printf("removed %d unattached files\n", n)
	return nil
}
```

Run: `go build ./... && go vet ./...`
Expected: clean.

- [ ] **Step 2: Smoke test against the local database**

```bash
make migrate
JWT_SECRET=dev-secret-change-me-dev-secret-change-me DATABASE_URL='postgres://ticket:ticket@localhost:5432/ticket?sslmode=disable' \
  go run ./cmd/api create-admin --username admin --email admin@example.test --password adminpass1
make run &
sleep 2
curl -s localhost:8080/health
curl -s -X POST localhost:8080/api/v1/auth/login -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"adminpass1"}' | tee /tmp/login.json
TOKEN=$(sed -E 's/.*"access_token":"([^"]+)".*/\1/' /tmp/login.json)
curl -s -X POST localhost:8080/api/v1/tickets -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"subject":"Smoke","message":"hello","requester_email":"r@example.test","topic_id":1}'
curl -s "localhost:8080/api/v1/tickets?state=open" -H "Authorization: Bearer $TOKEN"
kill %1
```

Expected: health `{"status":"ok"}`, login returns tokens, ticket create returns 201 JSON with `"number":"000001"`, list shows `"total":1`.

- [ ] **Step 3: Run the full suite with the coverage gate**

Run: `make test`
Expected: all packages PASS and the final line reads `coverage NN.N% (minimum 75%)` with NN.N above 75. If it is not, add tests to the packages `go tool cover -func=coverage.filtered.out | sort -k3 -n | head` lists as weakest before continuing; do not lower `COVER_MIN`.

- [ ] **Step 4: Write README**

`gin/README.md`:

```markdown
# Ticket API (Go)

JSON API for the ticket system, consumed by the React frontend. See
`docs/superpowers/specs/2026-09-24-go-ticket-api-design.md` for the design.

## Requirements

Go 1.26+, Docker (Postgres and Flyway run as containers).

## Run locally

    make migrate            # starts postgres, applies db/migrations with Flyway
    export DATABASE_URL='postgres://ticket:ticket@localhost:5432/ticket?sslmode=disable'
    export JWT_SECRET='some-string-of-at-least-32-bytes-long'
    go run ./cmd/api create-admin --username admin --email admin@example.test --password changeme1
    make run                # serves on :8080

## Configuration (environment)

| Variable | Default | Notes |
|---|---|---|
| DATABASE_URL | required | pgx connection string |
| JWT_SECRET | required | at least 32 bytes |
| PORT | 8080 | |
| STORAGE_DIR | ./storage | attachment files |
| CORS_ORIGINS | none | comma-separated allowed origins |
| MAX_UPLOAD_BYTES | 10485760 | per file |
| ALLOWED_MIME | images, pdf, text, csv, zip, office | comma-separated |

## Commands

    make test               # full suite with coverage gate (> 75%)
    make sqlc               # regenerate internal/db from db/queries
    go run ./cmd/api gc-files --older-than 24h

## Layout

`cmd/api` entrypoint; `internal/<feature>` packages each with `handler.go` and
`service.go`; `internal/db` is sqlc output plus pool and transaction helpers;
`db/migrations` is owned by Flyway.
```

- [ ] **Step 5: Commit**

```bash
git add gin
git commit -m "feat(gin): wire services, CLI subcommands, coverage gate and README" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

## Done criteria

- `make test` passes with coverage above 75%.
- `make migrate` on an empty database applies two migrations; running it again reports no migration necessary.
- The smoke test in Task 12 Step 2 works end to end.
- Every spec section maps to a task: layout (1, 2), data model (3, 4), API surface (5 through 11), authorization (5, 9, 10, 11), error handling and observability (1, 2), testing (every task, gate in 12).
