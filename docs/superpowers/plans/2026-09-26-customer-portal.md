# Customer Portal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give end users a portal, modelled on osTicket's client portal, to open tickets without an account, sign in by password or emailed link, use a guest link for one ticket, list and read their own tickets, reply with attachments, close and reopen, and manage a profile.

**Architecture:** A new Go package `internal/client` owns end-user identity (tables `end_user`, `client_token`, `client_refresh_token`), a client-audience JWT with its own middleware, and the `/api/v1/portal` routes, reusing the ticket service's `CreateExternal`/`AppendMessage`, the attachment service and the mail outbox. The React app gains a `/portal` subtree with its own shell, session store and pages built on the existing UI primitives and tokens.

**Tech Stack:** Go 1.26, Gin, pgx/v5, sqlc, Flyway (V5), testcontainers; React 19, TypeScript, Vite, react-router-dom, @tanstack/react-query, CSS modules, Vitest + Testing Library + msw. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-09-26-customer-portal-design.md`

## Global Constraints

- Branch `portal` from `main` (6bd2ceda). Go under `gin/`, React under `app/`. `V1`–`V4` are frozen; this slice adds `V5__portal.sql` only.
- No new npm or Go dependencies.
- Client JWTs carry `aud: "client"`; the staff middleware rejects them and the client middleware rejects tokens without it. Access token TTL 15 minutes; refresh tokens rotate on use and are stored hashed in `client_refresh_token`.
- Token lifetimes: `confirm` and `reset` 24 h; `signin` and `access` 1 h. Tokens are 32 random bytes, sent raw only in the emailed URL, stored as SHA-256 hex. One-time use (`used_at`).
- Never reveal whether an address or ticket exists: `link`, `reset` and `access` requests always return 202; login failures return the generic message.
- Password policy: at least 8 characters; hashed with `auth.HashPassword`.
- Rate limits: login, link, reset and access requests use the existing fixed-window limiter keyed by `lower(email)` and by client IP; anonymous ticket creation and anonymous uploads 10 per hour per IP and per email; 429 with `retry_after` seconds.
- Portal error codes: 410 `token_invalid` for token exchange; 403 `guest_session` for account-only routes; otherwise the shared envelope and mapping.
- Portal session storage key `ticket.portal_refresh_token`; the staff key `ticket.refresh_token` is never read or written by portal code.
- Seeded template keys: `client_confirm`, `client_signin_link`, `client_access_link`, `client_reset`; links are `${APP_BASE_URL}/portal/t/<raw token>`.
- Customers see thread entries of type `message` and `response` only; never `note`.
- `cd gin && go build ./... && go vet ./... && gofmt -l ./cmd ./internal` clean after every Go task; `make test` strictly above 75%. `cd app && npm test && npm run lint && npm run build` clean after every React task.
- Every commit message ends with `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`.

## Review Focus

1. Two tickets opened anonymously with the same address in different letter case must resolve to one end user, and registering that address later must attach to it. (Task 2 tests `TestUpsertByEmailIsCaseInsensitive`, Task 3 `TestRegisterAttachesToAnonymousUser`.)
2. A guest access link must not grant access to any other ticket of the same requester, and a guest refresh must not widen the scope. (Task 3 `TestGuestSessionScopedToTicket`, Task 4 `TestGuestCannotListTickets`.)
3. A staff token presented to the portal, or a client token to the staff API, must be rejected. (Task 3 `TestAudienceSeparation`.)
4. Replying to a closed ticket must reopen it and record the event before the message is appended, so the agent alert goes out on an open ticket. (Task 4 `TestReplyReopensClosedTicket`.)
5. Following the same emailed link twice must fail the second time with 410 and leave the first session valid. (Task 3 `TestTokenSingleUse`.)

## File structure

Go (`gin/`):
- `db/migrations/V5__portal.sql` — tables, columns, backfill, seeded templates.
- `db/queries/client.sql` — end users, client tokens, client refresh tokens, portal ticket queries.
- `internal/client/principal.go` — `Principal`, context helpers, `RequireUser`, `RequireAccount`.
- `internal/client/tokens.go` — client JWT issue/parse (audience), one-time token generation/hash.
- `internal/client/service.go` — identity service (register, login, link, exchange, reset, refresh, logout, profile, password).
- `internal/client/portal.go` — portal ticket service (reference, create, list, get, reply, close, reopen, file access).
- `internal/client/handler.go` — routes under `/api/v1/portal`.
- `internal/client/ratelimit.go` — small wrapper over `auth`'s limiter for per-email + per-IP checks and the 10/hour anonymous limit.
- `internal/auth/middleware.go` — reject `aud: client`.
- `internal/mail/template.go` — no change (Vars already has `Link`); seeded templates only.
- `cmd/api/main.go` — construct and mount.

React (`app/src/`):
- `api/sessionStore.ts` — factory extracted from `api/client.ts` (storage key + refresh path), used by both staff and portal clients.
- `api/portalClient.ts` — portal `request` built on the factory with key `ticket.portal_refresh_token` and base `/api/v1/portal`.
- `api/portal.ts` — typed calls; `api/types.ts` — portal types.
- `portal/PortalAuthContext.tsx`, `portal/RequirePortalUser.tsx`, `portal/RequirePortalAccount.tsx`.
- `portal/PortalShell.tsx` + `.module.css`, `portal/portalNav.ts`.
- `portal/pages/` — `LandingPage`, `OpenTicketPage`, `LoginPage`, `RegisterPage`, `TokenPage`, `ResetPage`, `TicketListPage`, `TicketPage`, `ProfilePage`, `CheckEmailPage`.
- `App.tsx` — mount `/portal/*`.
- `test/portal.ts` — msw handlers and fixtures for the portal API.

## Lanes

| Lane | Tasks | Depends on | Notes |
|---|---|---|---|
| A | 1, 2, 3, 4 | — | Go: migration+queries → identity → auth flows → portal tickets, sequential |
| B | 5 | — | React session factory + portal client + shell + guards (mocks the API) |
| C | 6, 7 | B | React: public pages (landing, open, login, register, token, reset) |
| D | 8, 9 | B | React: ticket list, ticket page, profile |
| — | 10 | all | wiring in `main.go`, staff-side touches, e2e leg, docs |

Merge order into `portal`: A and B (independent), then C and D, then Task 10.

---

### Task 1: Migration and queries

**Files:**
- Create: `gin/db/migrations/V5__portal.sql`
- Create: `gin/db/queries/client.sql`
- Modify: `gin/db/migrate_test.go` (migration count 4 → 5; assert `end_user` exists)
- Test: `gin/internal/db/portal_schema_test.go` (backfill behaviour)

**Interfaces:**
- Consumes: `testutil.Tx(t)`, `db.New`.
- Produces: sqlc queries listed in Step 2 (names are what later tasks call; check the generated param/row struct names in `internal/db/client.sql.go` and report them).

- [ ] **Step 1: Write the migration**

`gin/db/migrations/V5__portal.sql`:

```sql
CREATE TYPE client_token_kind AS ENUM ('confirm', 'reset', 'signin', 'access');

CREATE TABLE end_user (
  id                 bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  email              text NOT NULL,
  name               text NOT NULL DEFAULT '',
  password_hash      text,
  email_verified_at  timestamptz,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX end_user_email_idx ON end_user (lower(email));

CREATE TABLE client_token (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  end_user_id  bigint NOT NULL REFERENCES end_user(id) ON DELETE CASCADE,
  kind         client_token_kind NOT NULL,
  token_hash   text NOT NULL UNIQUE,
  ticket_id    bigint REFERENCES ticket(id) ON DELETE CASCADE,
  expires_at   timestamptz NOT NULL,
  used_at      timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX client_token_user_kind_idx ON client_token (end_user_id, kind);

CREATE TABLE client_refresh_token (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  token_hash   text NOT NULL UNIQUE,
  end_user_id  bigint NOT NULL REFERENCES end_user(id) ON DELETE CASCADE,
  ticket_id    bigint REFERENCES ticket(id) ON DELETE CASCADE,
  expires_at   timestamptz NOT NULL,
  revoked_at   timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE ticket ADD COLUMN user_id bigint REFERENCES end_user(id) ON DELETE SET NULL;
CREATE INDEX ticket_user_idx ON ticket (user_id, last_message_at DESC);
ALTER TABLE thread_entry ADD COLUMN user_id bigint REFERENCES end_user(id) ON DELETE SET NULL;

-- Backfill: one end user per distinct address, named from the most recent ticket.
INSERT INTO end_user (email, name)
SELECT DISTINCT ON (lower(requester_email)) requester_email, requester_name
FROM ticket
ORDER BY lower(requester_email), created_at DESC;

UPDATE ticket t SET user_id = u.id
FROM end_user u WHERE lower(u.email) = lower(t.requester_email);

INSERT INTO email_template (key, subject, body_html, body_text) VALUES
('client_confirm', 'Confirm your {{.SiteName}} account',
 '<p>Hello {{.RequesterName}},</p><p>Confirm your email address to finish creating your {{.SiteName}} account:</p><p><a href="{{.Link}}">{{.Link}}</a></p><p>This link expires in 24 hours. If you did not create an account, ignore this message.</p>',
 'Hello {{.RequesterName}},\n\nConfirm your email address to finish creating your {{.SiteName}} account:\n\n{{.Link}}\n\nThis link expires in 24 hours. If you did not create an account, ignore this message.'),
('client_signin_link', 'Your {{.SiteName}} sign-in link',
 '<p>Hello {{.RequesterName}},</p><p>Use this link to sign in to {{.SiteName}}:</p><p><a href="{{.Link}}">{{.Link}}</a></p><p>It expires in 1 hour and works once. If you did not request it, ignore this message.</p>',
 'Hello {{.RequesterName}},\n\nUse this link to sign in to {{.SiteName}}:\n\n{{.Link}}\n\nIt expires in 1 hour and works once. If you did not request it, ignore this message.'),
('client_access_link', '[#{{.Number}}] Access link for {{.Subject}}',
 '<p>Hello {{.RequesterName}},</p><p>Use this link to view ticket #{{.Number}} ({{.Subject}}):</p><p><a href="{{.Link}}">{{.Link}}</a></p><p>It expires in 1 hour and works once.</p>',
 'Hello {{.RequesterName}},\n\nUse this link to view ticket #{{.Number}} ({{.Subject}}):\n\n{{.Link}}\n\nIt expires in 1 hour and works once.'),
('client_reset', 'Reset your {{.SiteName}} password',
 '<p>Hello {{.RequesterName}},</p><p>Use this link to set a new password for {{.SiteName}}:</p><p><a href="{{.Link}}">{{.Link}}</a></p><p>It expires in 24 hours and works once. If you did not request it, ignore this message.</p>',
 'Hello {{.RequesterName}},\n\nUse this link to set a new password for {{.SiteName}}:\n\n{{.Link}}\n\nIt expires in 24 hours and works once. If you did not request it, ignore this message.');
```

Check the `email_template` column list in `V3__email.sql` (it may have `updated_at` defaults only; if it has NOT NULL columns without defaults, add them). Postgres string literals do not interpret `\n`; use `E'...'` for the text bodies.

- [ ] **Step 2: Write the queries**

`gin/db/queries/client.sql`:

```sql
-- name: GetEndUserByEmail :one
SELECT * FROM end_user WHERE lower(email) = lower($1);

-- name: GetEndUser :one
SELECT * FROM end_user WHERE id = $1;

-- name: CreateEndUser :one
INSERT INTO end_user (email, name) VALUES ($1, $2) RETURNING *;

-- name: UpdateEndUserName :one
UPDATE end_user SET name = $2, updated_at = now() WHERE id = $1 RETURNING *;

-- name: SetEndUserPassword :exec
UPDATE end_user SET password_hash = $2, email_verified_at = COALESCE(email_verified_at, now()), updated_at = now() WHERE id = $1;

-- name: MarkEndUserVerified :exec
UPDATE end_user SET email_verified_at = COALESCE(email_verified_at, now()), updated_at = now() WHERE id = $1;

-- name: CreateClientToken :exec
INSERT INTO client_token (end_user_id, kind, token_hash, ticket_id, expires_at) VALUES ($1, $2, $3, $4, $5);

-- name: ConsumeClientToken :one
UPDATE client_token SET used_at = now()
WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()
RETURNING *;

-- name: CreateClientRefreshToken :exec
INSERT INTO client_refresh_token (token_hash, end_user_id, ticket_id, expires_at) VALUES ($1, $2, $3, $4);

-- name: GetClientRefreshToken :one
SELECT * FROM client_refresh_token WHERE token_hash = $1;

-- name: RevokeClientRefreshToken :exec
UPDATE client_refresh_token SET revoked_at = now(), updated_at = now() WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: RevokeClientRefreshTokensForUser :exec
UPDATE client_refresh_token SET revoked_at = now(), updated_at = now() WHERE end_user_id = $1 AND revoked_at IS NULL;

-- name: SetTicketUser :exec
UPDATE ticket SET user_id = $2 WHERE id = $1;

-- name: ListPortalTickets :many
SELECT t.id, t.number, t.subject, t.status_id, s.name AS status_name, s.state, d.name AS dept_name,
       t.created_at, t.last_message_at, t.closed_at
FROM ticket t JOIN ticket_status s ON s.id = t.status_id JOIN department d ON d.id = t.dept_id
WHERE t.user_id = @user_id
  AND (sqlc.narg('state')::text IS NULL OR s.state::text = sqlc.narg('state'))
ORDER BY t.last_message_at DESC
LIMIT @lim OFFSET @off;

-- name: CountPortalTickets :one
SELECT count(*) FROM ticket t JOIN ticket_status s ON s.id = t.status_id
WHERE t.user_id = @user_id
  AND (sqlc.narg('state')::text IS NULL OR s.state::text = sqlc.narg('state'));

-- name: GetPortalTicket :one
SELECT t.id, t.number, t.subject, t.user_id, t.status_id, s.name AS status_name, s.state, d.name AS dept_name,
       h.name AS topic_name, t.created_at, t.updated_at, t.last_message_at, t.closed_at
FROM ticket t JOIN ticket_status s ON s.id = t.status_id JOIN department d ON d.id = t.dept_id
LEFT JOIN help_topic h ON h.id = t.topic_id
WHERE t.id = $1;

-- name: ListPortalThread :many
SELECT e.id, e.type, e.body, e.format, e.created_at, e.user_id, e.staff_id,
       u.name AS user_name, st.first_name, st.last_name, st.username
FROM thread_entry e
LEFT JOIN end_user u ON u.id = e.user_id
LEFT JOIN staff st ON st.id = e.staff_id
WHERE e.ticket_id = $1 AND e.type IN ('message', 'response')
ORDER BY e.id;

-- name: SetThreadEntryUser :exec
UPDATE thread_entry SET user_id = $2 WHERE id = $1;

-- name: ListPublicDepartments :many
SELECT id, name FROM department WHERE is_public ORDER BY name;

-- name: ListActiveTopics :many
SELECT id, name FROM help_topic WHERE is_active ORDER BY sort_order, name;

-- name: FirstStatusInState :one
SELECT id, name, state FROM ticket_status WHERE state = $1 ORDER BY sort_order, id LIMIT 1;

-- name: GetTicketIDByNumberAndEmail :one
SELECT id FROM ticket WHERE number = $1 AND lower(requester_email) = lower($2);
```

Check the real column names in `V1__init.sql` (`is_public`, `is_active`, `sort_order`, `closed_at`, `last_message_at`, `dept_id`, `topic_id`) and the thread-entry columns before running `cd gin && make sqlc`. Also check `db/queries/attachment.sql` for the query that lists a thread entry's attachments; the portal thread reuses it.

- [ ] **Step 3: Write the failing schema test**

`gin/internal/db/portal_schema_test.go`:

```go
package db_test

import (
	"context"
	"testing"

	"github.com/grandpine/ticket-api/internal/db/testutil"
)

func TestPortalBackfillLinksTicketsByEmail(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	// The migration ran against an empty database, so create tickets and re-run the
	// backfill statements to prove they are idempotent and case-insensitive.
	_, err := tx.Exec(ctx, `INSERT INTO department (name, is_public) VALUES ('Support', true)`)
	if err != nil { t.Fatal(err) }
	_, err = tx.Exec(ctx, `
	  INSERT INTO ticket (number, subject, dept_id, priority_id, status_id, requester_name, requester_email, source, extra)
	  SELECT '000101', 's', d.id, p.id, s.id, 'Pat', 'Pat@Example.test', 'web', '{}'::jsonb
	  FROM department d, (SELECT id FROM ticket_priority ORDER BY id LIMIT 1) p, (SELECT id FROM ticket_status ORDER BY id LIMIT 1) s
	  WHERE d.name = 'Support'`)
	if err != nil { t.Fatal(err) }
	_, err = tx.Exec(ctx, `
	  INSERT INTO ticket (number, subject, dept_id, priority_id, status_id, requester_name, requester_email, source, extra)
	  SELECT '000102', 's', d.id, p.id, s.id, 'pat', 'pat@example.test', 'web', '{}'::jsonb
	  FROM department d, (SELECT id FROM ticket_priority ORDER BY id LIMIT 1) p, (SELECT id FROM ticket_status ORDER BY id LIMIT 1) s
	  WHERE d.name = 'Support'`)
	if err != nil { t.Fatal(err) }
	_, err = tx.Exec(ctx, `
	  INSERT INTO end_user (email, name)
	  SELECT DISTINCT ON (lower(requester_email)) requester_email, requester_name FROM ticket
	  WHERE NOT EXISTS (SELECT 1 FROM end_user u WHERE lower(u.email) = lower(ticket.requester_email))
	  ORDER BY lower(requester_email), created_at DESC`)
	if err != nil { t.Fatal(err) }
	_, err = tx.Exec(ctx, `UPDATE ticket t SET user_id = u.id FROM end_user u WHERE lower(u.email) = lower(t.requester_email)`)
	if err != nil { t.Fatal(err) }
	var users, linked int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM end_user WHERE lower(email) = 'pat@example.test'`).Scan(&users); err != nil { t.Fatal(err) }
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM ticket WHERE user_id IS NOT NULL AND number IN ('000101','000102')`).Scan(&linked); err != nil { t.Fatal(err) }
	if users != 1 || linked != 2 {
		t.Fatalf("users=%d linked=%d, want 1 and 2", users, linked)
	}
}
```

The migration's own backfill runs on the real database at upgrade time; this test re-executes the same statements (with the `NOT EXISTS` guard, which the migration must also carry so a re-run is safe) against seeded rows. Adapt the ticket insert to the real NOT NULL columns of `ticket` (check `V1__init.sql`; add `extra` or others as needed). Also update `gin/db/migrate_test.go`: the expected migration count becomes 5 and the index assertion list gains `end_user_email_idx`.

- [ ] **Step 4: Run to verify it fails**

Run: `cd gin && go test ./internal/db/ ./db/ -run 'Portal|Flyway' -timeout 10m`
Expected: FAIL — `relation "end_user" does not exist` / migration count mismatch.

- [ ] **Step 5: Generate and run**

Run: `cd gin && make sqlc && go build ./... && go test ./internal/db/ ./db/ -timeout 10m`
Expected: PASS. Record the generated names for: `EndUser`, `ClientToken`, `ClientRefreshToken`, `ClientTokenKind` constants, `ListPortalTicketsParams`, `ListPortalTicketsRow`, `GetPortalTicketRow`, `ListPortalThreadRow`, `CreateClientTokenParams`, `CreateClientRefreshTokenParams`.

- [ ] **Step 6: Commit**

```bash
git add gin/db/migrations/V5__portal.sql gin/db/queries/client.sql gin/internal/db gin/db/migrate_test.go
git commit -m "feat(db): end users, client tokens, and portal queries

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---
### Task 2: Client identity core — tokens, principal, middleware, errors, limiter, outbox ripple (Go, lane A)

**Files:**
- Modify: `gin/db/migrations/V5__portal.sql` (append: `ALTER TABLE email_outbox ALTER COLUMN ticket_id DROP NOT NULL;`), `gin/db/queries/mail.sql` (no text change; regenerate so `CreateOutboxParams.TicketID` becomes `*int64`), then every Go caller of `CreateOutbox`/`Notification.TicketID`
- Create: `gin/internal/client/tokens.go`, `tokens_test.go`, `principal.go`, `principal_test.go`, `ratelimit.go`, `ratelimit_test.go`
- Modify: `gin/internal/auth/token.go` (staff `ParseAccess` rejects `aud` containing `client`), `gin/internal/auth/ratelimit.go` (export a reusable limiter), `gin/internal/apperr/apperr.go` (+ `ErrTokenInvalid`, `ErrGuestSession`, `RateLimitedError`), `gin/internal/httpx/httpx.go` (+ 410 `token_invalid`, 403 `guest_session`, 429 with `retry_after` field), `gin/internal/mail/notifier.go` (`Notification.TicketID *int64`; `Link` only set when empty; message id for ticket-less mail), `gin/internal/mail/sender.go`, `handler.go` (outbox JSON `ticket_id` nullable), `gin/internal/ticket/service.go` + `thread.go` (pass `&id`), tests touched accordingly

**Interfaces:**
- Consumes: `auth.NewTokens`-style JWT code (`github.com/golang-jwt/jwt/v5`, HS256), `auth.HashRefreshToken`, `auth.NewRefreshToken`, `httpx.Fail`, `apperr` sentinels.
- Produces:
  - `client.Claims{ TicketID *int64 `json:"tid,omitempty"`; jwt.RegisteredClaims }` with `Audience: ["client"]`.
  - `client.NewTokens(secret string, accessTTL time.Duration) *Tokens`; `(*Tokens).IssueAccess(userID int64, ticketID *int64) (string, time.Time, error)`; `(*Tokens).ParseAccess(raw string) (Claims, error)` (requires audience `client`, HS256, exp).
  - `client.NewOneTimeToken() (raw, hash string, err error)` (32 random bytes hex; hash = sha256 hex) and `client.HashToken(raw string) string`.
  - `client.Principal{ UserID int64; Email string; Verified bool; TicketID *int64 }` with `IsGuest() bool`; `client.WithPrincipal`, `client.FromContext`, `client.RequireUser(tokens *Tokens, loader UserLoader) gin.HandlerFunc`, `client.RequireAccount() gin.HandlerFunc` (403 `guest_session`), `UserLoader interface { LoadClient(ctx, userID int64) (Principal, error) }`.
  - `auth.NewRateLimiter(max int, window time.Duration) *RateLimiter` with `Allow(key string) bool`, `Reset(key string)`, `RetryAfter(key string) time.Duration` (the existing login limiter becomes `NewRateLimiter(10, time.Minute)`); `client.Limiter` wraps two of them: `func NewLimiter(max int, window time.Duration) *Limiter`, `(*Limiter).Check(email, ip string) error` returning `apperr.RateLimited(retryAfter)` when either key is over.
  - `apperr.ErrTokenInvalid` → 410 `token_invalid`; `apperr.ErrGuestSession` → 403 `guest_session`; `apperr.RateLimited(after time.Duration) error` (type `*RateLimitedError{RetryAfter}`) → 429 `rate_limited` with `fields: {"retry_after": "<seconds>"}`; existing `ErrRateLimited` keeps mapping to 429 without fields.
  - `mail.Notification.TicketID *int64` (nil for account mail); `mail.NewMessageID(ticketID *int64, domain)`: `<ticket-<id>-<hex>@domain>` when set, `<client-<hex>@domain>` when nil; `Enqueue` fills `Vars.Link` only when empty and skips the In-Reply-To lookup when `TicketID` is nil.

- [ ] **Step 1: Failing tests for tokens and middleware**

`gin/internal/client/tokens_test.go`:

```go
package client

import (
	"testing"
	"time"

	"github.com/grandpine/ticket-api/internal/auth"
)

const secret = "0123456789abcdef0123456789abcdef"

func TestClientTokenRoundTripAndAudience(t *testing.T) {
	ct := NewTokens(secret, 15*time.Minute)
	raw, exp, err := ct.IssueAccess(42, nil)
	if err != nil { t.Fatal(err) }
	if time.Until(exp) < 14*time.Minute { t.Fatalf("exp too soon: %v", exp) }
	c, err := ct.ParseAccess(raw)
	if err != nil { t.Fatal(err) }
	if c.Subject != "42" || c.TicketID != nil { t.Fatalf("claims %+v", c) }
	tid := int64(7)
	g, _, _ := ct.IssueAccess(42, &tid)
	gc, err := ct.ParseAccess(g)
	if err != nil || gc.TicketID == nil || *gc.TicketID != 7 { t.Fatalf("guest claims %+v %v", gc, err) }

	// A staff token is not a client token and vice versa.
	st := auth.NewTokens(secret, 15*time.Minute)
	staffRaw, _, _ := st.IssueAccess(1, true)
	if _, err := ct.ParseAccess(staffRaw); err == nil { t.Fatal("client parser accepted a staff token") }
	if _, err := st.ParseAccess(raw); err == nil { t.Fatal("staff parser accepted a client token") }
}

func TestOneTimeTokenHash(t *testing.T) {
	raw, hash, err := NewOneTimeToken()
	if err != nil { t.Fatal(err) }
	if len(raw) != 64 || HashToken(raw) != hash || len(hash) != 64 { t.Fatalf("raw %q hash %q", raw, hash) }
	raw2, _, _ := NewOneTimeToken()
	if raw2 == raw { t.Fatal("tokens must differ") }
}
```

`gin/internal/client/principal_test.go`:

```go
package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/auth"
)

type fakeLoader struct{}

func (fakeLoader) LoadClient(_ context.Context, id int64) (Principal, error) {
	return Principal{UserID: id, Email: "u@x.test", Verified: true}, nil
}

func TestRequireUserAndAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ct := NewTokens(secret, time.Minute)
	r := gin.New()
	g := r.Group("/p", RequireUser(ct, fakeLoader{}))
	g.GET("/any", func(c *gin.Context) { p, _ := FromContext(c); c.JSON(200, gin.H{"uid": p.UserID, "guest": p.IsGuest()}) })
	g.GET("/acct", RequireAccount(), func(c *gin.Context) { c.Status(204) })

	call := func(tok, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if tok != "" { req.Header.Set("Authorization", "Bearer "+tok) }
		w := httptest.NewRecorder(); r.ServeHTTP(w, req); return w
	}
	if w := call("", "/p/any"); w.Code != 401 { t.Fatalf("no token: %d", w.Code) }
	user, _, _ := ct.IssueAccess(5, nil)
	if w := call(user, "/p/any"); w.Code != 200 || w.Body.String() != `{"guest":false,"uid":5}` { t.Fatalf("user: %d %s", w.Code, w.Body.String()) }
	if w := call(user, "/p/acct"); w.Code != 204 { t.Fatalf("account: %d", w.Code) }
	tid := int64(9)
	guest, _, _ := ct.IssueAccess(5, &tid)
	if w := call(guest, "/p/acct"); w.Code != 403 || !contains(w.Body.String(), "guest_session") { t.Fatalf("guest on account route: %d %s", w.Code, w.Body.String()) }
	staff, _, _ := auth.NewTokens(secret, time.Minute).IssueAccess(1, false)
	if w := call(staff, "/p/any"); w.Code != 401 { t.Fatalf("staff token on portal: %d", w.Code) }
}

func contains(s, sub string) bool { return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0) }
func indexOf(s, sub string) int { for i := 0; i+len(sub) <= len(s); i++ { if s[i:i+len(sub)] == sub { return i } }; return -1 }
```

(Use `strings.Contains` instead of the helpers; they are shown only to keep the snippet self-contained.)

`gin/internal/client/ratelimit_test.go`:

```go
func TestLimiterChecksEmailAndIP(t *testing.T) {
	l := NewLimiter(2, time.Minute)
	if err := l.Check("a@x.test", "1.1.1.1"); err != nil { t.Fatal(err) }
	if err := l.Check("a@x.test", "2.2.2.2"); err != nil { t.Fatal(err) }
	err := l.Check("A@X.TEST", "3.3.3.3") // same email, case-insensitive, third hit
	var rl *apperr.RateLimitedError
	if !errors.As(err, &rl) || rl.RetryAfter <= 0 { t.Fatalf("want rate limited, got %v", err) }
	if err := l.Check("b@x.test", "1.1.1.1"); err != nil { t.Fatal(err) } // ip has 2 hits now
	if err := l.Check("c@x.test", "1.1.1.1"); err == nil { t.Fatal("ip over limit") }
}
```

Run: `cd gin && go test ./internal/client/ -timeout 10m` — Expected: build failure (package empty).

- [ ] **Step 2: Implement tokens, principal, middleware**

`gin/internal/client/tokens.go`:

```go
package client

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

const Audience = "client"

type Claims struct {
	TicketID *int64 `json:"tid,omitempty"`
	jwt.RegisteredClaims
}

type Tokens struct {
	secret    []byte
	accessTTL time.Duration
	now       func() time.Time
}

func NewTokens(secret string, accessTTL time.Duration) *Tokens {
	return &Tokens{secret: []byte(secret), accessTTL: accessTTL, now: time.Now}
}

func (t *Tokens) AccessTTL() time.Duration { return t.accessTTL }

func (t *Tokens) IssueAccess(userID int64, ticketID *int64) (string, time.Time, error) {
	now := t.now()
	exp := now.Add(t.accessTTL)
	jti := make([]byte, 16)
	if _, err := rand.Read(jti); err != nil {
		return "", time.Time{}, err
	}
	c := Claims{TicketID: ticketID, RegisteredClaims: jwt.RegisteredClaims{
		Subject: strconv.FormatInt(userID, 10), Audience: jwt.ClaimStrings{Audience},
		IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(exp), ID: hex.EncodeToString(jti),
	}}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(t.secret)
	return s, exp, err
}

func (t *Tokens) ParseAccess(raw string) (Claims, error) {
	var c Claims
	_, err := jwt.ParseWithClaims(raw, &c, func(*jwt.Token) (any, error) { return t.secret, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithAudience(Audience), jwt.WithTimeFunc(t.now))
	if err != nil {
		return Claims{}, fmt.Errorf("%w: %v", apperr.ErrUnauthorized, err)
	}
	return c, nil
}

// NewOneTimeToken returns a raw 32-byte token (hex) and its sha256 hex, the only form stored.
func NewOneTimeToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	raw = hex.EncodeToString(b)
	return raw, HashToken(raw), nil
}

func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
```

In `gin/internal/auth/token.go` `ParseAccess`: after parsing, `if slices.Contains(c.Audience, "client") { return Claims{}, apperr.ErrUnauthorized }` (staff tokens have no audience). Add a test in `auth/token_test.go`.

`gin/internal/client/principal.go`:

```go
package client

type Principal struct {
	UserID   int64
	Email    string
	Verified bool
	TicketID *int64
}

func (p Principal) IsGuest() bool { return p.TicketID != nil }

const principalKey = "client.principal"

func WithPrincipal(c *gin.Context, p Principal) { c.Set(principalKey, p) }

func FromContext(c *gin.Context) (Principal, bool) {
	v, ok := c.Get(principalKey)
	if !ok { return Principal{}, false }
	p, ok := v.(Principal)
	return p, ok
}

type UserLoader interface {
	LoadClient(ctx context.Context, userID int64) (Principal, error)
}

func RequireUser(tokens *Tokens, loader UserLoader) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if len(h) < 8 || !strings.EqualFold(h[:7], "Bearer ") {
			httpx.Fail(c, apperr.ErrUnauthorized); c.Abort(); return
		}
		claims, err := tokens.ParseAccess(strings.TrimSpace(h[7:]))
		if err != nil { httpx.Fail(c, err); c.Abort(); return }
		id, err := strconv.ParseInt(claims.Subject, 10, 64)
		if err != nil { httpx.Fail(c, apperr.ErrUnauthorized); c.Abort(); return }
		p, err := loader.LoadClient(c.Request.Context(), id)
		if err != nil { httpx.Fail(c, err); c.Abort(); return }
		p.TicketID = claims.TicketID
		WithPrincipal(c, p)
		c.Next()
	}
}

func RequireAccount() gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := FromContext(c)
		if !ok { httpx.Fail(c, apperr.ErrUnauthorized); c.Abort(); return }
		if p.IsGuest() { httpx.Fail(c, apperr.ErrGuestSession); c.Abort(); return }
		c.Next()
	}
}
```

Check how `httpx.Fail` aborts (it may call `c.AbortWithStatusJSON`); match the staff middleware exactly.

- [ ] **Step 3: Errors, httpx mapping, limiter**

`apperr`: add `ErrTokenInvalid = errors.New("token invalid")`, `ErrGuestSession = errors.New("guest session")`, and

```go
type RateLimitedError struct{ RetryAfter time.Duration }
func (e *RateLimitedError) Error() string { return "rate limited" }
func (e *RateLimitedError) Is(target error) bool { return target == ErrRateLimited }
func RateLimited(after time.Duration) error { return &RateLimitedError{RetryAfter: after} }
```

`httpx.Fail`: before the `ErrRateLimited` case, `var rl *apperr.RateLimitedError; if errors.As(err, &rl) { write(c, 429, "rate_limited", "too many requests", map[string]string{"retry_after": strconv.Itoa(int(math.Ceil(rl.RetryAfter.Seconds())))}); return }`; add `ErrTokenInvalid` → `write(c, http.StatusGone, "token_invalid", "this link has expired or was already used", nil)` and `ErrGuestSession` → `write(c, 403, "guest_session", "sign in to an account to do this", nil)`. Tests in `httpx_test.go` for all three.

`auth/ratelimit.go`: rename the type to `RateLimiter` with exported `NewRateLimiter(max int, window time.Duration) *RateLimiter`, `Allow`, `Reset`, and add `RetryAfter(key string) time.Duration` (time until the key's window ends, 0 if unknown); keep `newLoginRateLimiter()` as `NewRateLimiter(loginRateLimitMax, loginRateLimitWindow)` so the staff handler is unchanged.

`client/ratelimit.go`:

```go
type Limiter struct{ byEmail, byIP *auth.RateLimiter }

func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{byEmail: auth.NewRateLimiter(max, window), byIP: auth.NewRateLimiter(max, window)}
}

// Check counts one attempt for the address and the IP and reports the longer wait when either is over.
func (l *Limiter) Check(email, ip string) error {
	e := strings.ToLower(strings.TrimSpace(email))
	okE, okI := l.byEmail.Allow("e|"+e), l.byIP.Allow("i|"+ip)
	if okE && okI { return nil }
	after := l.byEmail.RetryAfter("e|" + e)
	if a := l.byIP.RetryAfter("i|" + ip); a > after { after = a }
	return apperr.RateLimited(after)
}
```

- [ ] **Step 4: Outbox ripple**

Append `ALTER TABLE email_outbox ALTER COLUMN ticket_id DROP NOT NULL;` to `V5__portal.sql`; `make sqlc`; fix compile errors: `mail.Notification.TicketID *int64`; in `ticket/service.go` and `thread.go` pass `&row.ID` (or the local id) at every `Enqueue`; `mail.NewMessageID(ticketID *int64, domain string)` produces `<client-<12hex>@domain>` when nil (update `TicketIDFromMessageID` to return false for those); `Enqueue`: `if nt.Vars.Link == "" && nt.TicketID != nil { nt.Vars.Link = fmt.Sprintf("%s/tickets/%d", n.baseURL, *nt.TicketID) }`; skip `LastSentMessageID` when nil; `mail/handler.go` outbox JSON `TicketID *int64`; `sender.go` if it reads `TicketID`. Update the affected tests (`notify_test.go`, `notifier_test.go`, `handler_test.go`) and add one: enqueueing a notification with nil ticket and a preset `Link` stores a row with null ticket_id and the given link in the body. `internal/inbound` uses `TicketIDFromMessageID` on References; a `client-` id simply yields false.

- [ ] **Step 5: Gate and commit**

Run: `cd gin && make sqlc && go build ./... && go vet ./... && gofmt -l ./cmd ./internal && go test ./internal/client/ ./internal/auth/ ./internal/apperr/ ./internal/httpx/ ./internal/mail/ ./internal/ticket/ ./internal/inbound/ -timeout 15m`
Expected: PASS.

```bash
git add -A gin
git commit -m "feat(api): client tokens, principal, middleware, and ticket-less outbox mail

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 3: Identity service and auth routes (Go, lane A)

**Files:**
- Create: `gin/internal/client/service.go`, `service_test.go`, `handler.go`, `handler_test.go`

**Interfaces:**
- Consumes: Task 1 queries; Task 2 tokens/principal/limiter/errors; `auth.HashPassword`, `auth.CheckPassword`, `auth.NewRefreshToken`, `auth.HashRefreshToken`, `mail.Notifier.Enqueue`, `mail.Vars`, `db.WithTx`, `config.MailConfig.BaseURL`/`SiteName` (passed in as strings).
- Produces:
  - `client.Profile{ ID int64 `json:"id"`; Email string `json:"email"`; Name string `json:"name"`; Verified bool `json:"verified"`; HasPassword bool `json:"has_password"` }`
  - `client.Session{ AccessToken string `json:"access_token"`; RefreshToken string `json:"refresh_token"`; ExpiresIn int `json:"expires_in"`; User Profile `json:"user"`; TicketID *int64 `json:"ticket_id"`; Kind string `json:"kind,omitempty"` }`
  - `client.NewService(b db.Beginner, tokens *Tokens, refreshTTL time.Duration, notifier mail.Notifier, baseURL, siteName string) *Service`
  - Methods: `UpsertByEmail(ctx, q *db.Queries, email, name string) (db.EndUser, error)` (exported for the portal ticket service); `Register(ctx, in RegisterInput) error`; `Login(ctx, email, password string) (*Session, error)`; `RequestLink(ctx, email string) error`; `RequestReset(ctx, email string) error`; `RequestAccess(ctx, email, number string) error`; `Exchange(ctx, raw string) (*Session, error)`; `Refresh(ctx, raw string) (*Session, error)`; `Logout(ctx, userID int64, raw string) error`; `Me(ctx, userID int64) (*Profile, error)`; `UpdateName(ctx, userID int64, name string) (*Profile, error)`; `SetPassword(ctx, p Principal, in PasswordInput) error`; `LoadClient(ctx, userID int64) (Principal, error)`.
  - `client.NewHandler(svc IdentityService, limiter *Limiter) *Handler`; `(*Handler).MountAuth(public *gin.RouterGroup, user *gin.RouterGroup)` where `user` is a group already wrapped in `RequireUser`.
  - `RegisterInput{ Email string `json:"email" binding:"required,email,max=255"`; Name string `json:"name" binding:"required,max=128"`; Password string `json:"password" binding:"required"` }`; `PasswordInput{ Password string `json:"password" binding:"required"`; CurrentPassword string `json:"current_password"` }`.
  - Reset sessions: `Exchange` of a `reset` token returns a session whose access token carries `TicketID == nil` and whose refresh token is **not** issued (RefreshToken empty); the `Kind` field is `"reset"`; `SetPassword` accepts a missing `current_password` only when the user has no password **or** the request came within 15 minutes of a `reset` exchange — implement the latter by storing `password_reset_until` in memory keyed by user id? No: simpler and stateless — a `reset` exchange returns an access token with a `pwr: true` claim; add `PasswordReset bool `json:"pwr,omitempty"`` to `Claims` and to `Principal`, and `SetPassword` skips the current-password check when `p.PasswordReset`. Add this claim in Task 2's structs now (one field each).

- [ ] **Step 1: Failing service tests**

`gin/internal/client/service_test.go` (testcontainers; a `recordingNotifier` captures `Notification`s so tests can read `Vars.Link` and derive the raw token from the URL):

```go
type recordingNotifier struct{ sent []mail.Notification }
func (r *recordingNotifier) Enqueue(_ context.Context, _ *db.Queries, n mail.Notification) error { r.sent = append(r.sent, n); return nil }

func newSvc(t *testing.T) (*Service, *recordingNotifier, pgx.Tx) {
	tx := testutil.Tx(t)
	n := &recordingNotifier{}
	return NewService(tx, NewTokens(secret, 15*time.Minute), 24*time.Hour, n, "https://desk.test", "Desk"), n, tx
}

func rawFromLink(t *testing.T, link string) string { // https://desk.test/portal/t/<raw>
	i := strings.LastIndex(link, "/t/"); if i < 0 { t.Fatalf("link %q", link) }; return link[i+3:]
}

func TestRegisterConfirmLogin(t *testing.T) {
	svc, n, _ := newSvc(t)
	ctx := context.Background()
	if err := svc.Register(ctx, RegisterInput{Email: "Pat@Example.test", Name: "Pat", Password: "secret123"}); err != nil { t.Fatal(err) }
	if len(n.sent) != 1 || n.sent[0].TemplateKey != "client_confirm" || n.sent[0].TicketID != nil { t.Fatalf("mail %+v", n.sent) }
	if _, err := svc.Login(ctx, "pat@example.test", "secret123"); !errors.Is(err, apperr.ErrUnauthorized) { t.Fatalf("login before confirm: %v", err) }
	s, err := svc.Exchange(ctx, rawFromLink(t, n.sent[0].Vars.Link))
	if err != nil || s.Kind != "confirm" || !s.User.Verified { t.Fatalf("exchange %+v %v", s, err) }
	if _, err := svc.Exchange(ctx, rawFromLink(t, n.sent[0].Vars.Link)); !errors.Is(err, apperr.ErrTokenInvalid) { t.Fatalf("second use: %v", err) }
	s2, err := svc.Login(ctx, "PAT@example.test", "secret123")
	if err != nil || s2.User.ID != s.User.ID || !s2.User.HasPassword { t.Fatalf("login %+v %v", s2, err) }
	if _, err := svc.Login(ctx, "pat@example.test", "wrong"); !errors.Is(err, apperr.ErrUnauthorized) { t.Fatal("wrong password accepted") }
}

func TestUpsertByEmailIsCaseInsensitive(t *testing.T) { /* UpsertByEmail twice with different case → same id; name updated only when previously empty */ }

func TestRegisterAttachesToAnonymousUser(t *testing.T) { /* UpsertByEmail("a@x.test","A") then Register(a@x.test) → same id, password set, confirm mail; Register again → apperr.ErrConflict */ }

func TestSigninLinkFlow(t *testing.T) { /* RequestLink for unknown address sends nothing and returns nil; for known sends client_signin_link; Exchange → Kind "signin", session with refresh token; Refresh rotates (old raw → ErrUnauthorized); Logout revokes */ }

func TestGuestSessionScopedToTicket(t *testing.T) { /* create two tickets for pat via raw SQL (or ticket service); RequestAccess(email, number1) → client_access_link with TicketID=&t1 and Vars.Number; Exchange → Kind "access", TicketID == t1, RefreshToken non-empty; Refresh keeps TicketID; RequestAccess with wrong email sends nothing */ }

func TestTokenExpiry(t *testing.T) { /* set svc.now to +2h → signin exchange fails ErrTokenInvalid; +25h → confirm fails */ }

func TestResetFlow(t *testing.T) { /* RequestReset → client_reset; Exchange → Kind "reset", RefreshToken == "", access claims PasswordReset; SetPassword with PasswordReset principal and no current → ok, verified; Login works; SetPassword without reset and without current → apperr.Validation on current_password */ }

func TestAudienceSeparation(t *testing.T) { /* LoadClient for a user id that does not exist → ErrUnauthorized; staff token parsing covered in Task 2 */ }
```

Write the bodies in full (the comments describe the assertions; each test must assert what its name says).

- [ ] **Step 2: Implement the service**

Key shapes (write the full file):

```go
type Service struct {
	b        db.Beginner
	tokens   *Tokens
	refreshTTL time.Duration
	notifier mail.Notifier
	baseURL, siteName string
	now      func() time.Time
}

func (s *Service) link(raw string) string { return s.baseURL + "/portal/t/" + raw }

func (s *Service) UpsertByEmail(ctx context.Context, q *db.Queries, email, name string) (db.EndUser, error) {
	u, err := q.GetEndUserByEmail(ctx, email)
	if err == nil {
		if u.Name == "" && name != "" { return q.UpdateEndUserName(ctx, db.UpdateEndUserNameParams{ID: u.ID, Name: name}) }
		return u, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) { return db.EndUser{}, err }
	return q.CreateEndUser(ctx, db.CreateEndUserParams{Email: strings.TrimSpace(email), Name: name})
}

func (s *Service) issueToken(ctx context.Context, q *db.Queries, u db.EndUser, kind db.ClientTokenKind, ticketID *int64, ttl time.Duration) (string, error) {
	raw, hash, err := NewOneTimeToken()
	if err != nil { return "", err }
	err = q.CreateClientToken(ctx, db.CreateClientTokenParams{EndUserID: u.ID, Kind: kind, TokenHash: hash, TicketID: ticketID, ExpiresAt: s.now().Add(ttl)})
	return raw, err
}

func (s *Service) sendLink(ctx context.Context, q *db.Queries, u db.EndUser, key string, raw string, ticketID *int64, vars mail.Vars) error {
	vars.Link = s.link(raw); vars.SiteName = s.siteName
	if vars.RequesterName == "" { vars.RequesterName = u.Name }
	return s.notifier.Enqueue(ctx, q, mail.Notification{TemplateKey: key, TicketID: ticketID, To: []mail.Recipient{{Name: u.Name, Address: u.Email}}, Vars: vars})
}
```

`Register`: in `WithTx`: `UpsertByEmail`; if `password_hash != nil` → `apperr.ErrConflict`; hash (`auth.HashPassword` enforces 8..72) and `SetEndUserPassword`… no: setting the password must not verify the email; use a dedicated query `UPDATE end_user SET password_hash=$2 WHERE id=$1` (`SetEndUserPasswordHash`, add to `client.sql` in this task) and keep `SetEndUserPassword` (which verifies) for the reset/profile path; then `issueToken(confirm, 24h)` + `sendLink("client_confirm")`.
`Login`: lookup by email; if missing or no password or not verified → dummy compare + `ErrUnauthorized`; `CheckPassword`; `issue(u, nil, "")`.
`issue(ctx, q, u, ticketID, kind) (*Session, error)`: `IssueAccess(u.ID, ticketID)` (with the `pwr` claim when kind == "reset"); unless kind == "reset": `auth.NewRefreshToken()` + `CreateClientRefreshToken{TokenHash, EndUserID, TicketID, ExpiresAt}`.
`RequestLink`/`RequestReset`: lookup; unknown → return nil; else token + mail (`signin` 1h / `reset` 24h).
`RequestAccess`: `GetTicketIDByNumberAndEmail`; `pgx.ErrNoRows` → nil; else `UpsertByEmail` (requester name from the ticket via `GetPortalTicket`), token `access` 1h with `ticket_id`, mail `client_access_link` with `Vars{Number, Subject}` and `TicketID` set (so the outbox row links to the ticket).
`Exchange`: `ConsumeClientToken(HashToken(raw))`; `ErrNoRows` → `ErrTokenInvalid`; load user; `confirm` → `MarkEndUserVerified`; build session by kind; `Session.Kind = string(tok.Kind)`.
`Refresh`: mirror staff `Refresh` with `client_refresh_token` (consume = revoke + return; add `ConsumeClientRefreshToken` query in this task, `UPDATE … SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL RETURNING *`), check expiry, re-issue with the same `TicketID`.
`Logout`: `RevokeClientRefreshToken` scoped to the user (add `end_user_id` to that query's WHERE).
`Me`/`UpdateName`/`SetPassword` (validation error on `current_password` when required and missing/wrong; `auth.HashPassword`; `SetEndUserPassword` verifies; `RevokeClientRefreshTokensForUser` after a password change except for the session's own? keep simple: revoke all others by revoking all — document).
`LoadClient`: `GetEndUser` → `ErrNoRows` → `ErrUnauthorized`.

- [ ] **Step 3: Handler and tests**

`handler.go` `MountAuth(public, user)`: `public.POST("/auth/login")` (limiter `Check(email, c.ClientIP())` first; generic 401), `POST("/auth/link")`, `POST("/auth/reset")`, `POST("/access")` (all three: limiter, call, always 202 `{}`), `POST("/auth/exchange")`, `POST("/auth/register")` (201), `POST("/auth/refresh")`; `user.POST("/auth/logout")`, `user.GET("/me")`, `user.PATCH("/me")`, `user.POST("/me/password")`. Handler tests with a fake `IdentityService` (interface listing the methods above) assert: 202 for link/reset/access regardless of the service's "unknown" result; 401 body for a bad login has no `fields`; 410 `token_invalid`; 429 with `retry_after` after the limiter trips (limiter with max 1 in the test); 403 `guest_session` on `/me` for a guest principal (mount `user` under `RequireAccount()` for `/me`, `/me/password`; `logout` stays under `RequireUser` only).

- [ ] **Step 4: Gate and commit**

Run: `cd gin && go build ./... && go vet ./... && gofmt -l ./cmd ./internal && go test ./internal/client/ -timeout 15m`

```bash
git add -A gin
git commit -m "feat(api): end-user accounts, sign-in links, guest access, and password reset

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 4: Portal ticket service and routes (Go, lane A)

**Files:**
- Create: `gin/internal/client/portal.go`, `portal_test.go`; extend `handler.go` with `MountPortal(public, user, account *gin.RouterGroup)` and `handler_test.go`
- Modify: `gin/internal/ticket/types.go` + `service.go` (`ExternalCreateInput` gains `Source string` (default `"email"` when empty) and `UserID *int64`; `create` sets `ticket.user_id` via `SetTicketUser` when given; `MessageInput` gains `UserID *int64`; `AppendMessage` sets `thread_entry.user_id` via `SetThreadEntryUser`; the `created` event data `via` becomes `in.Source` when the source is not email — i.e. `"portal"` for web)
- Modify: `gin/internal/attachment/service.go` (`Download` also allows a caller that owns the ticket: add `DownloadForTicket(ctx, ticketID, fileID int64) (*File, io.ReadCloser, error)` that checks the attachment belongs to an entry of that ticket, used by the portal; `Upload` gains an uploader-less variant `UploadAnonymous(ctx, name, mime, r)` storing `uploaded_by = NULL`)

**Interfaces:**
- Consumes: Task 1 queries (`ListPortalTickets`, `CountPortalTickets`, `GetPortalTicket`, `ListPortalThread`, `ListPublicDepartments`, `ListActiveTopics`, `FirstStatusInState`, `SetTicketUser`, `SetThreadEntryUser`), the attachment-list-per-entry query used by `ticket.Thread` (reuse its name), `ticket.Service.CreateExternal/AppendMessage/SetStatus` (with `ticket.SystemPrincipal`), `mail` nothing new.
- Produces:
  - `client.PortalService` with `Reference(ctx) (*Reference, error)`; `OpenTicket(ctx, p *Principal, ip string, in OpenInput) (*Opened, error)`; `ListTickets(ctx, p Principal, state string, page httpx.Page) (*httpx.List[TicketRow], error)`; `GetTicket(ctx, p Principal, id int64) (*TicketView, error)`; `Reply(ctx, p Principal, id int64, in ReplyInput) error`; `Close(ctx, p Principal, id int64) (*TicketView, error)`; `Reopen(ctx, p Principal, id int64) (*TicketView, error)`; `Download(ctx, p Principal, ticketID, fileID int64) (*attachment.File, io.ReadCloser, error)`.
  - JSON: `Reference{ SiteName string `json:"site_name"`; Departments []Ref `json:"departments"`; Topics []Ref `json:"topics"` }`; `Opened{ ID int64 `json:"id"`; Number string `json:"number"` }`; `TicketRow{ ID; Number; Subject; Status Ref `json:"status"`; State string `json:"state"`; Department string `json:"department"`; CreatedAt time.Time `json:"created_at"`; LastMessageAt time.Time `json:"last_message_at"`; ClosedAt *time.Time `json:"closed_at"` }`; `TicketView{ TicketRow; Topic *string `json:"topic"`; UpdatedAt time.Time `json:"updated_at"`; Entries []EntryView `json:"entries"` }`; `EntryView{ ID int64; Type string; Poster string; Body string; Format string; CreatedAt time.Time; Attachments []ticket.AttachmentRef }` with snake_case tags.
  - `OpenInput{ Name string `json:"name" binding:"max=128"`; Email string `json:"email" binding:"omitempty,email,max=255"`; Subject string `json:"subject" binding:"required,max=255"`; Message string `json:"message" binding:"required"`; Format string `json:"format" binding:"omitempty,oneof=html text"`; TopicID *int64 `json:"topic_id"`; DeptID *int64 `json:"dept_id"`; FileIDs []int64 `json:"file_ids"` }` — email required when there is no session (validated in the service: `apperr.Validation("email", "required")`).
  - `ReplyInput{ Body string `json:"body" binding:"required"`; Format string `json:"format" binding:"omitempty,oneof=html text"`; FileIDs []int64 `json:"file_ids"` }`.
  - Routes: `public.GET("/reference")`, `public.POST("/tickets")` (optional session: a small `OptionalUser(tokens, loader)` middleware that sets the principal when a valid bearer token is present and otherwise continues), `public.POST("/files")` (same optional session; anonymous limited 10/hour per IP), `user.GET("/tickets/:id")`, `user.POST("/tickets/:id/reply")`, `user.GET("/tickets/:id/files/:fileId")`, `account.GET("/tickets")`, `account.POST("/tickets/:id/close")`, `account.POST("/tickets/:id/reopen")`.
  - Anonymous open limiter: `NewLimiter(10, time.Hour)` keyed by email and IP.

- [ ] **Step 1: Failing service tests** (`portal_test.go`, testcontainers)

`TestOpenTicketAnonymousCreatesUserAndAutoresponse` (user row created, `ticket.user_id` set, source `web`, `created` event data `via: "portal"`, one `ticket_autoresp` notification with `TicketID` set); `TestOpenTicketSignedInUsesSessionEmail`; `TestListTicketsOwnOnly` (two users, state filter, paging); `TestGuestCannotListTickets` (`ErrGuestSession`) and `TestGuestCanViewAndReplyOwnTicketOnly` (other ticket → `ErrNotFound`); `TestGetTicketHidesNotes` (a `note` entry via the ticket service is absent; message and response present with poster names and attachments); `TestReplyAppendsMessageWithUser` (`thread_entry.user_id` set, `message_alert` enqueued, ticket unanswered); `TestReplyReopensClosedTicket` (close via `SetStatus`, reply → state open, a `status_changed`/`reopened` event with `user_id` in data recorded before the entry id); `TestCloseAndReopen` (409 on repeat via `ErrConflict`; events carry `user_id`); `TestDownloadChecksTicket` (file attached to another ticket → `ErrNotFound`); `TestAnonymousOpenRateLimited` (limiter max 1 in the test → `RateLimitedError`).

- [ ] **Step 2: Implement**

`portal.go` outline:

```go
type PortalService struct {
	b        db.Beginner
	tickets  ticket.Service   // constructed by the caller with the notifier
	files    attachment.Service
	ident    *Service
	openLimit *Limiter
	siteName string
}

func NewPortalService(b db.Beginner, tickets ticket.Service, files attachment.Service, ident *Service, siteName string) *PortalService

func (s *PortalService) OpenTicket(ctx context.Context, p *Principal, ip string, in OpenInput) (*Opened, error) {
	email, name := in.Email, in.Name
	if p != nil { email = p.Email } else if email == "" { return nil, apperr.Validation("email", "required") }
	if p == nil { if err := s.openLimit.Check(email, ip); err != nil { return nil, err } }
	var out *Opened
	err := db.WithTx(ctx, s.b, func(q *db.Queries) error {
		u, err := s.ident.UpsertByEmail(ctx, q, email, name)
		if err != nil { return err }
		deptID := int64(0); if in.DeptID != nil { deptID = *in.DeptID }
		svc := ticket.NewService(q.DB().(db.Beginner), s.ticketOpts...)   // same pattern as internal/inbound: a tx satisfies Beginner
		t, err := svc.CreateExternal(ctx, ticket.ExternalCreateInput{Subject: in.Subject, Body: in.Message, Format: in.Format,
			RequesterName: u.Name, RequesterEmail: u.Email, DeptID: deptID, TopicID: in.TopicID, FileIDs: in.FileIDs, Source: "web", UserID: &u.ID})
		if err != nil { return err }
		out = &Opened{ID: t.ID, Number: t.Number}
		return nil
	})
	return out, err
}
```

Check how `internal/inbound/process.go` constructs the ticket service inside a transaction and copy it exactly (it passes the notifier option through). `ExternalCreateInput` gains `TopicID *int64` too if `CreateInput` supports a topic (it does: `TopicID`); when `DeptID` is 0 and a topic has a department, the ticket service's existing rule applies (the API says "required when no topic supplies a department" — surface that validation as-is).

`GetTicket`: `GetPortalTicket`; `row.UserID == nil || *row.UserID != p.UserID` → `ErrNotFound`; guest: `p.TicketID != nil && *p.TicketID != id` → `ErrNotFound`; entries from `ListPortalThread` + attachments per entry (same query `ticket.Thread` uses; find it in `db/queries/thread.sql`); poster = `user_name` when `user_id` set, else staff full name/username, else the entry's `poster` column (include `e.poster` in the query if not already).

`Reply`: ownership check as above; in one tx: if `row.State == "closed"` → `svc.SetStatus(ctx, ticket.SystemPrincipal, id, firstOpenStatusID)` and record the event with `user_id` (SetStatus records `reopened`; add the user id by a follow-up `UPDATE ticket_event SET data = data || '{"user_id": n}' WHERE id = (SELECT max(id) FROM ticket_event WHERE ticket_id = $1)` — or, cleaner, give `ticket.SetStatus` an actor option; choose the follow-up update and document it); then `svc.AppendMessage(ctx, id, ticket.MessageInput{Poster: name, Body, Format, FileIDs, UserID: &p.UserID})`.

`Close`/`Reopen`: `FirstStatusInState('closed' | 'open')`; `ErrConflict` when already in that state; `SetStatus` + the same event data update; return `GetTicket`.

`Download`: ownership check then `files.DownloadForTicket(ctx, id, fileID)`.

Handler: `MountPortal(public, user, account)`; `OptionalUser` middleware in `principal.go` (`if header present → same as RequireUser else c.Next()`); upload handler copies `attachment.Handler`'s multipart handling (or expose a helper from that package) and calls `UploadAnonymous` when no principal, else `Upload` with… the attachment service's `Upload` takes an `auth.Principal`; for portal users call `UploadAnonymous` too (the file is linked to the ticket on create/reply, which is the ownership that matters) — rate-limit anonymous uploads with the same `openLimit` keyed by IP only.

- [ ] **Step 3: Gate and commit**

Run: `cd gin && go build ./... && go vet ./... && gofmt -l ./cmd ./internal && go test ./internal/client/ ./internal/ticket/ ./internal/attachment/ -timeout 15m`

```bash
git add -A gin
git commit -m "feat(api): portal tickets — open, list, view, reply, close, reopen, files

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---
### Task 5: Session factory, portal client, shell, and guards (React, lane B)

**Files:**
- Create: `app/src/api/sessionStore.ts`, `app/src/api/sessionStore.test.ts`
- Modify: `app/src/api/client.ts` (becomes a thin wrapper over the factory; every existing export keeps its name and behaviour)
- Create: `app/src/api/portalClient.ts`, `app/src/api/portal.ts`, portal types appended to `app/src/api/types.ts`
- Create: `app/src/portal/PortalAuthContext.tsx`, `RequirePortalUser.tsx`, `RequirePortalAccount.tsx`, `PortalShell.tsx`, `PortalShell.module.css`, `portalNav.ts`, tests `PortalAuthContext.test.tsx`, `PortalShell.test.tsx`
- Create: `app/src/test/portal.ts` (msw handlers + fixtures for `/api/v1/portal/*`); register them in `app/src/test/handlers.ts`
- Modify: `app/src/App.tsx` (mount `/portal/*` with placeholder pages that later tasks replace)

**Interfaces:**
- Consumes: the current `api/client.ts` internals (module-scope `accessToken`, `refreshing`, `generation`, `sessionLostHandler`, `tokens`, `refreshSession`, `waitForRefresh`, `fetchWithAuth`, `request`, `toError`, `buildUrl`), `Banner`, `BannerProvider`, `TabBar`, `SubNav`, `useSubNav` pattern from `AppShell`.
- Produces (exact, used by Tasks 6–9):
  - `createSessionStore({ storageKey, base, refreshPath }): { tokens, refreshSession, waitForRefresh, fetchWithAuth, request }` in `sessionStore.ts`; `client.ts` re-exports the staff instance so nothing else changes.
  - `portalClient.ts`: `export const portal = createSessionStore({ storageKey: PORTAL_REFRESH_KEY, base: '/api/v1/portal', refreshPath: '/auth/refresh' })` and `export const PORTAL_REFRESH_KEY = 'ticket.portal_refresh_token'`.
  - `api/portal.ts` functions: `getReference(): Promise<PortalReference>`; `openTicket(input: OpenTicketInput): Promise<{ id: number; number: string }>`; `uploadPortalFile(file: File): Promise<FileInfo>`; `login(email, password): Promise<PortalSession>`; `requestLink(email): Promise<void>`; `exchange(token): Promise<PortalSession>`; `register(input: { email; name; password }): Promise<void>`; `requestReset(email): Promise<void>`; `requestAccess(email, number): Promise<void>`; `logout(): Promise<void>`; `listMyTickets(f: { state?: 'open'|'closed'; page: number; page_size: number }): Promise<ListResponse<PortalTicketRow>>`; `getMyTicket(id): Promise<PortalTicket>`; `replyTicket(id, input: { body; format: 'text'|'html'; file_ids?: number[] }): Promise<void>`; `closeTicket(id)`, `reopenTicket(id)`: `Promise<PortalTicket>`; `getMe(): Promise<PortalProfile>`; `updateMe({ name })`; `setPassword({ password, current_password? })`; `downloadPortalFile(ticketId, fileId, name)`.
  - Types: `PortalProfile { id; email; name; verified: boolean; has_password: boolean }`; `PortalSession { access_token; refresh_token; expires_in; user: PortalProfile; ticket_id: number | null }`; `PortalReference { site_name; departments: Ref[]; topics: Ref[] }`; `OpenTicketInput { name; email; subject; message; format: 'text'|'html'; topic_id?; dept_id?; file_ids? }`; `PortalTicketRow { id; number; subject; status: Ref; state: TicketState; department: string; created_at; last_message_at; closed_at: string|null }`; `PortalEntry { id; type: 'message'|'response'; poster: string; body; format; created_at; attachments: AttachmentRef[] }`; `PortalTicket extends PortalTicketRow { topic: string | null; updated_at; entries: PortalEntry[] }`.
  - `PortalAuthProvider`, `usePortalAuth(): { status: 'loading'|'anonymous'|'authenticated'|'error'; user: PortalProfile | null; ticketId: number | null; isGuest: boolean; notice: string | null; error: unknown; login(email, password): Promise<void>; adopt(session: PortalSession): void; logout(): Promise<void>; retry(): void }` (`adopt` stores a session obtained from a token exchange).
  - `RequirePortalUser` (redirect to `/portal/login` with `from`), `RequirePortalAccount` (guests → `/portal/tickets/<ticketId>`).
  - `PortalShell` (route element with `Outlet`, or `children`), `usePortalSubNav(items)`.
  - `portalNav.ts`: `PORTAL_TABS = [{ label: 'Support Center Home', to: '/portal' }, { label: 'Open a New Ticket', to: '/portal/open' }, { label: 'Check Ticket Status', to: '/portal/login' }]`; when signed in the third tab becomes `{ label: 'My Tickets', to: '/portal/tickets' }`.

- [ ] **Step 1: Write the failing session-store test**

`app/src/api/sessionStore.test.ts`:

```ts
import { http, HttpResponse } from 'msw'
import { server } from '../test/setup'
import { createSessionStore } from './sessionStore'

const staffKey = 'ticket.refresh_token'
const portalKey = 'ticket.portal_refresh_token'

it('keeps two stores on separate storage keys and refresh paths', async () => {
  const calls: string[] = []
  server.use(
    http.post('/api/v1/auth/refresh', () => { calls.push('staff'); return HttpResponse.json({ access_token: 'a1', refresh_token: 'r1', expires_in: 900, staff: {} }) }),
    http.post('/api/v1/portal/auth/refresh', () => { calls.push('portal'); return HttpResponse.json({ access_token: 'p1', refresh_token: 'pr1', expires_in: 900, user: {}, ticket_id: null }) }),
  )
  const staff = createSessionStore({ storageKey: staffKey, base: '/api/v1', refreshPath: '/auth/refresh' })
  const portal = createSessionStore({ storageKey: portalKey, base: '/api/v1/portal', refreshPath: '/auth/refresh' })
  localStorage.setItem(portalKey, 'pr0')
  expect(await portal.refreshSession()).toBe(true)
  expect(calls).toEqual(['portal'])
  expect(localStorage.getItem(portalKey)).toBe('pr1')
  expect(localStorage.getItem(staffKey)).toBeNull()
  expect(await staff.refreshSession()).toBe(false) // no staff token stored
  expect(portal.tokens.access).toBe('p1')
  expect(staff.tokens.access).toBeNull()
})

it('clears only its own key on session loss', async () => {
  server.use(http.post('/api/v1/portal/auth/refresh', () => HttpResponse.json({ error: { code: 'unauthorized', message: 'no' } }, { status: 401 })))
  const portal = createSessionStore({ storageKey: portalKey, base: '/api/v1/portal', refreshPath: '/auth/refresh' })
  const lost = vi.fn()
  portal.tokens.setOnSessionLost(lost)
  localStorage.setItem(portalKey, 'pr0')
  localStorage.setItem(staffKey, 'r0')
  expect(await portal.refreshSession()).toBe(false)
  expect(lost).toHaveBeenCalled()
  expect(localStorage.getItem(portalKey)).toBeNull()
  expect(localStorage.getItem(staffKey)).toBe('r0')
})
```

Run: `cd app && npx vitest run src/api/sessionStore.test.ts` — Expected: FAIL (module missing).

- [ ] **Step 2: Extract the factory**

Move the body of `api/client.ts` into `api/sessionStore.ts` as:

```ts
export interface SessionLike { access_token: string; refresh_token: string }
export interface SessionStoreOptions { storageKey: string; base: string; refreshPath: string }

export function createSessionStore(o: SessionStoreOptions) {
  let accessToken: string | null = null
  let refreshing: Promise<boolean> | null = null
  let sessionLostHandler: (() => void) | null = null
  let generation = 0
  const tokens = { /* identical to today's object but using o.storageKey */ }
  function buildUrl(path: string, query?: Query): string { /* identical, using o.base */ }
  async function doRefresh(canRetry: boolean): Promise<boolean> { /* identical; posts to buildUrl(o.refreshPath) and reads the JSON as SessionLike */ }
  function refreshSession(): Promise<boolean> { /* identical */ }
  function waitForRefresh(): Promise<unknown> { return refreshing ?? Promise.resolve() }
  async function fetchWithAuth(method: string, path: string, init: FetchInit = {}, retry = true): Promise<Response> { /* identical */ }
  async function request<T>(method: string, path: string, opts: FetchInit = {}): Promise<T> { /* identical */ }
  return { tokens, refreshSession, waitForRefresh, fetchWithAuth, request }
}
```

`ApiError`, `Query`, `FetchInit`, `toError` and `safeStorage` stay exported from `sessionStore.ts` (they are shared). `api/client.ts` becomes:

```ts
import { createSessionStore } from './sessionStore'
export { ApiError, type FetchInit, type Query } from './sessionStore'
export const REFRESH_KEY = 'ticket.refresh_token'
const staff = createSessionStore({ storageKey: REFRESH_KEY, base: '/api/v1', refreshPath: '/auth/refresh' })
export const tokens = staff.tokens
export const refreshSession = staff.refreshSession
export const waitForRefresh = staff.waitForRefresh
export const fetchWithAuth = staff.fetchWithAuth
export const request = staff.request
```

`tokens.setSession` accepts `SessionLike` (both `Session` and `PortalSession` satisfy it). Run the existing `client.test.ts` and `auth.test.tsx` to prove nothing moved.

`api/portalClient.ts`:

```ts
import { createSessionStore } from './sessionStore'
export const PORTAL_REFRESH_KEY = 'ticket.portal_refresh_token'
export const portal = createSessionStore({ storageKey: PORTAL_REFRESH_KEY, base: '/api/v1/portal', refreshPath: '/auth/refresh' })
```

- [ ] **Step 3: Types, portal API module, msw handlers**

Append the types from the Interfaces block to `api/types.ts`. `api/portal.ts` implements each function with `portal.request` (login → `POST /auth/login` also calls `portal.tokens.setSession(s)`; `exchange` → `POST /auth/exchange` `{ token }` and `setSession`; `logout` → `POST /auth/logout` `{ refresh_token }` then `tokens.clear()` in a `finally`; `uploadPortalFile` → `POST /files` with `formData`; `downloadPortalFile` mirrors `api/files.ts`'s `downloadFile` but through `portal.fetchWithAuth('GET', `/tickets/${ticketId}/files/${fileId}`)`).

`test/portal.ts` exports `portalFixtures` (`profile`, `session`, `guestSession` with `ticket_id: 7`, `reference` `{ site_name: 'Ticket Desk', departments: [{id:1,name:'Support'}], topics: [{id:1,name:'General'}] }`, `ticketRow`, `ticket` with two entries (message + response with one attachment)) and `portalHandlers` for every route above: `POST /api/v1/portal/auth/login` (200 for `pat@example.test`/`secret123`, else 401), `POST …/auth/link|reset|/access` → 202, `POST …/auth/exchange` (token `good-signin` → session, `good-access` → guestSession, `good-confirm` → session, `good-reset` → session with `reset: true` flag in `user`? no: return the session and let the page route by the `kind` query the API adds — see Step 4), `POST …/auth/register` → 201, `POST …/auth/refresh` (refresh token starting `prefresh` → rotated session, else 401), `POST …/auth/logout` → 204, `GET …/me` → profile, `PATCH …/me`, `POST …/me/password` → 204, `GET …/reference`, `POST …/tickets` → 201 `{ id: 8, number: '000008' }`, `POST …/files` → 201 `{ id: 42, … }`, `GET …/tickets` → list, `GET …/tickets/:id` → ticket (id 7) else 404, `POST …/tickets/:id/reply` → 204, `POST …/tickets/:id/close|reopen` → ticket with state toggled, `GET …/tickets/:id/files/:fid` → text body. Append `...portalHandlers` to the `handlers` array in `test/handlers.ts`.

Exchange response shape (fixed here so Task 3's Go handler and the pages agree): `{ access_token, refresh_token, expires_in, user, ticket_id, kind: 'confirm'|'signin'|'access'|'reset' }` — the page routes by `kind`. Add `kind` to `PortalSession` as optional.

- [ ] **Step 4: Auth context, guards, shell**

`portal/PortalAuthContext.tsx` mirrors `auth/AuthContext.tsx` with `portal.tokens`, `portal.refreshSession`, `getMe`, and the extra `adopt(session)` that calls `portal.tokens.setSession(session)`, sets `user`, `ticketId = session.ticket_id`, status authenticated. Session-lost notice text: `'Your session expired. Please sign in again.'`. `isGuest = ticketId !== null`.

`RequirePortalUser`: loading → `LoadingScreen`; error → `Banner` with Retry; anonymous → `<Navigate to="/portal/login" replace state={{ from }} />`; else `<Outlet />`. `RequirePortalAccount`: `isGuest ? <Navigate to={`/portal/tickets/${ticketId}`} replace /> : <Outlet />`.

`portal/PortalShell.tsx`:

```tsx
export function PortalShell({ children }: { children?: ReactNode }) {
  const { status, user, isGuest, ticketId, logout } = usePortalAuth()
  const [sub, setSub] = useState<SubNavItem[] | null>(null)
  const tabs = status === 'authenticated' && !isGuest
    ? [PORTAL_TABS[0], PORTAL_TABS[1], { label: 'My Tickets', to: '/portal/tickets' }]
    : PORTAL_TABS
  return (
    <div className={s.page}>
      <header className={s.header}>
        <div className={s.headerInner}>
          <Link to="/portal" className={s.brand}>Ticket Desk</Link>
          <div className={s.user}>
            {status !== 'authenticated' && <><span className={s.muted}>Guest User</span><Link to="/portal/login" className={s.headerLink}>Sign In</Link></>}
            {status === 'authenticated' && isGuest && <><span className={s.muted}>Ticket access</span><Link to={`/portal/tickets/${ticketId}`} className={s.headerLink}>My Ticket</Link><button type="button" className={s.headerBtn} onClick={() => void logout()}>Sign Out</button></>}
            {status === 'authenticated' && !isGuest && user && <><span><strong>{user.name || user.email}</strong></span><Link to="/portal/profile" className={s.headerLink}>Profile</Link><Link to="/portal/tickets" className={s.headerLink}>Tickets</Link><button type="button" className={s.headerBtn} onClick={() => void logout()}>Sign Out</button></>}
          </div>
        </div>
      </header>
      <TabBar tabs={tabs} />
      {sub && <SubNav items={sub} />}
      <main className={s.content}>
        <PortalSubNavCtx.Provider value={setSub}>
          <BannerProvider>{children ?? <Outlet />}</BannerProvider>
        </PortalSubNavCtx.Provider>
      </main>
      <footer className={s.footer}>Ticket Desk · Support Center</footer>
    </div>
  )
}
```

`usePortalSubNav(items)` mirrors `useSubNav` (`useLayoutEffect`, JSON key). `TabBar` matches by prefix, so the Home tab (`/portal`) must use exact matching: pass `end` for that tab — extend `TabBar`'s tab type with optional `end?: boolean` forwarded to `NavLink` (one-line change in `ui/TabBar.tsx`, with a test).

`PortalShell.module.css` copies `AppShell.module.css` (tokens only) with a narrower `--content-max` override on `.content` (`max-width: 960px`) to read like osTicket's client site.

- [ ] **Step 5: Mount the subtree**

In `App.tsx`, before the staff routes:

```tsx
<Route path="/portal" element={<PortalAuthProvider><PortalShell /></PortalAuthProvider>}>
  <Route index element={<h2>Support Center</h2>} />
  <Route path="open" element={<h2>Open a New Ticket</h2>} />
  <Route path="login" element={<h2>Sign In</h2>} />
  <Route path="register" element={<h2>Register</h2>} />
  <Route path="reset" element={<h2>Reset</h2>} />
  <Route path="t/:token" element={<h2>Token</h2>} />
  <Route element={<RequirePortalUser />}>
    <Route path="tickets/:id" element={<h2>Ticket</h2>} />
    <Route element={<RequirePortalAccount />}>
      <Route path="tickets" element={<h2>My Tickets</h2>} />
      <Route path="profile" element={<h2>Profile</h2>} />
    </Route>
  </Route>
</Route>
```

The placeholders are replaced by Tasks 6–9. The staff `AuthProvider` must not wrap the portal subtree (it would try to restore a staff session on portal pages): restructure `App.tsx` so `<AuthProvider>` wraps only the staff routes and the portal `<Route>` sits beside it inside `<Routes>`. React Router requires a single `<Routes>`; nest as `<Routes><Route path="/portal/*" element={<PortalApp />} /><Route path="/*" element={<StaffApp />} /></Routes>` where `StaffApp` is today's tree (with paths relative) and `PortalApp` the block above.

- [ ] **Step 6: Tests**

`PortalAuthContext.test.tsx`: restores a session from `PORTAL_REFRESH_KEY` (calls `/portal/auth/refresh` then `/portal/me`); never touches `REFRESH_KEY`; `adopt` sets the user and ticketId; guest flag. `PortalShell.test.tsx`: anonymous header shows "Guest User" and "Sign In" and three tabs; signed-in header shows name, Profile, Tickets, Sign Out and the "My Tickets" tab; guest header shows "My Ticket" and "Sign Out"; `RequirePortalUser` redirects anonymous to `/portal/login`; `RequirePortalAccount` redirects a guest to `/portal/tickets/7`. `App.test.tsx`: `/portal` renders "Support Center" without calling the staff `/me`; `/tickets` still renders the staff login redirect.

Run: `cd app && npm test && npm run lint && npm run build` — Expected: green, including every pre-existing staff test.

- [ ] **Step 7: Commit**

```bash
git add -A app/src
git commit -m "feat(app): portal session store, shell, guards, and API client

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 6: Landing, open-ticket, and check-email pages (React, lane C)

**Files:**
- Create: `app/src/portal/pages/LandingPage.tsx` + `.module.css`, `OpenTicketPage.tsx` + `.module.css`, `CheckEmailPage.tsx`, tests for each
- Modify: `app/src/App.tsx` placeholders for `index`, `open`; add `opened/:number` route

**Interfaces:**
- Consumes: `usePortalAuth`, `getReference`, `openTicket`, `uploadPortalFile`, `FormTable`, `FormActions` (cancelTo `/portal`), `Button`/`LinkButton`, `Banner`, `useBanner`, `FileUpload` (which calls the staff `uploadFile`; add an optional `upload?: (file: File) => Promise<FileInfo>` prop to `FileUpload`, defaulting to the staff one — one-line change with a test), `splitErrors`.

- [ ] **Step 1: Failing tests**

`LandingPage.test.tsx`: renders the welcome heading "Welcome to the Support Center", two big links "Open a New Ticket" → `/portal/open` and "Check Ticket Status" → `/portal/login`; when signed in the second reads "My Tickets" → `/portal/tickets`.

`OpenTicketPage.test.tsx` (mount through `PortalAuthProvider` + `PortalShell` at `/portal/open`):
- anonymous: form has Name, Email (required), Help Topic (from reference), Department (only when reference has departments), Subject (required), Message (required), Attachments; submit posts `{ name, email, subject, message, format: 'text', topic_id, dept_id, file_ids: [] }` and navigates to `/portal/opened/000008` which shows "Ticket #000008 opened" and "We emailed you a link to follow this ticket".
- signed in: Email is prefilled from the user and read-only; success page has a "View ticket" link to `/portal/tickets/8` and no email note.
- API 400 with `fields.subject` shows under Subject; 429 shows the banner "Too many attempts, try again in N minutes" (from `retry_after: 600` → 10 minutes).

- [ ] **Step 2: Implement**

`LandingPage`: `<h2>Welcome to the Support Center</h2>`, a paragraph ("Open a ticket for any question, or check the status of one you already have."), and two `LinkButton`s (`variant="primary"` and `variant="default"`, class for the large layout in the module CSS).

`OpenTicketPage`: `useQuery(['portal','reference'], getReference)`; form state `{ name, email, subject, message, topic_id: 0, dept_id: 0 }` seeded from `user` when signed in; `pending: PendingFile[]`; `useMutation(openTicket)`; on success `navigate(`/portal/opened/${res.number}`, { state: { id: res.id, anonymous: !user } })`; errors via `splitErrors(err, ['name','email','subject','message','topic_id','dept_id','file_ids'])`; 429: `err.status === 429` → banner with `Math.ceil((err.fields.retry_after ?? 0) / 60)` minutes (the API puts `retry_after` in the envelope's `fields` as a string; parse with `Number`). `FormActions saveLabel="Open Ticket" cancelTo="/portal"`.

`CheckEmailPage` (route `opened/:number` and reused by Tasks 7's register/reset with props): heading from props, body text, optional link.

- [ ] **Step 3: Gate and commit**

Run: `cd app && npm test && npm run lint && npm run build`

```bash
git add -A app/src
git commit -m "feat(app): portal landing and open-ticket pages

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 7: Login, register, token, and reset pages (React, lane C)

**Files:**
- Create: `app/src/portal/pages/LoginPage.tsx` + `.module.css`, `RegisterPage.tsx`, `TokenPage.tsx`, `ResetPage.tsx`, tests for each
- Modify: `app/src/App.tsx` placeholders

**Interfaces:**
- Consumes: `usePortalAuth().login/adopt`, `requestLink`, `requestAccess`, `register`, `requestReset`, `exchange`, `CheckEmailPage`, `Banner`, `useBanner`, `FormTable`, `Button`.

- [ ] **Step 1: Failing tests**

`LoginPage.test.tsx`: two columns with headings "Sign in" and "Check a ticket as a guest"; password login posts `{ email, password }` and navigates to `from` or `/portal/tickets`; wrong password shows "Invalid email or password" (generic); "Email me a sign-in link" posts `{ email }` and shows the check-email page ("If that address has an account, we sent a sign-in link"); guest form posts `{ email, number }` to `/access` and shows "If the ticket and email match, we sent an access link"; "Create an account" links to `/portal/register`; "Forgot password" links to `/portal/reset`.

`RegisterPage.test.tsx`: Name, Email, Password, Confirm; mismatch shows a field error without a request; success posts `{ email, name, password }` and shows "Check your email to confirm your account"; 409 shows "An account with that email already exists" with a link to sign in.

`TokenPage.test.tsx`: for token `good-signin` calls exchange, adopts the session, and navigates to `/portal/tickets`; `good-access` → `/portal/tickets/7`; `good-confirm` → `/portal/tickets` with flash "Email confirmed"; `good-reset` → `/portal/profile#password`; a 410 shows "This link has expired or was already used" with buttons "Email me a new sign-in link" (→ `/portal/login`) and "Open a new ticket".

`ResetPage.test.tsx`: Email → posts `/auth/reset` → check-email page.

- [ ] **Step 2: Implement**

`LoginPage`: two `<section>`s in a two-column grid (stacks under 768px), each a form; left uses `login(email, password)` then `navigate(state.from ?? '/portal/tickets')`; the link button `type="button"` calls `requestLink(email)` (requires the email field to be filled; otherwise focus it with a field error). Right form calls `requestAccess(email, number)`. Both "sent" states render `CheckEmailPage` content inline.

`TokenPage`: `useEffect` once: `exchange(token)` → `adopt(session)` → route by `session.kind` (default `/portal/tickets`); catch → error state. Guard StrictMode double-invocation with a ref so the token is exchanged once.

- [ ] **Step 3: Gate and commit**

```bash
git add -A app/src
git commit -m "feat(app): portal sign-in, registration, token, and reset pages

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 8: My tickets and ticket page (React, lane D)

**Files:**
- Create: `app/src/portal/pages/TicketListPage.tsx`, `TicketPage.tsx` + `.module.css`, `PortalThread.tsx`, tests
- Modify: `app/src/App.tsx` placeholders

**Interfaces:**
- Consumes: `listMyTickets`, `getMyTicket`, `replyTicket`, `closeTicket`, `reopenTicket`, `downloadPortalFile`, `uploadPortalFile`, `ListTable`, `Pagination`, `Badge`, `Button`, `Banner`, `useBanner`, `usePortalSubNav`, `FileUpload` (with the `upload` prop), `sanitizeHtml`, `formatDateTime`, `relativeTime`, `usePortalAuth().isGuest/ticketId`.

- [ ] **Step 1: Failing tests**

`TicketListPage.test.tsx`: sub-nav Open | Closed (`?state=open` default, `?state=closed`); columns Number, Date, Subject, Department, Status, Last Message; row link to `/portal/tickets/7`; pagination footer; empty state "You have no open tickets".

`TicketPage.test.tsx`: info table rows Number, Status, Department, Help Topic, Created, Last Updated; thread renders the message (left) and response (right) with poster names and the attachment download button (calls `/portal/tickets/7/files/…`); reply form posts `{ body, format: 'text', file_ids }` then refetches and flashes "Reply posted"; on a closed ticket the button reads "Reopen" and the reply form notes "Replying will reopen this ticket"; Close posts `/close`, Reopen posts `/reopen`; guest session (ticketId 7) can view and reply but the page shows no "My Tickets" link; 404 shows "Ticket not found".

- [ ] **Step 2: Implement**

`TicketListPage`: `useSearchParams` for `state` and `page`; `usePortalSubNav([{label:'Open', to:'/portal/tickets?state=open'}, {label:'Closed', to:'/portal/tickets?state=closed'}])`; `useQuery(['portal','tickets', {state, page}], …, placeholderData: keepPreviousData)`; `ListTable` with `rowHref`.

`TicketPage`: `useQuery(['portal','ticket', id], () => getMyTicket(id))`; `PortalThread` renders entries with classes `message`/`response` (module CSS: message left with `--page-bg` header, response right-aligned header with `--accent-bg`), body via `sanitizeHtml` when html else `<pre className="pre">`, attachments as buttons calling `downloadPortalFile(ticket.id, a.file_id, a.name)`; reply form (`textarea` labelled "Reply", `FileUpload` with `upload={uploadPortalFile}`, `Button` "Post Reply"); status control: `state === 'closed'` → `Button` "Reopen" else "Close ticket" (confirm via a second click like `ConfirmDelete`, or a `window.confirm`-free two-step button); after each mutation `invalidateQueries(['portal','ticket', id])` and `['portal','tickets']`.

- [ ] **Step 3: Gate and commit**

```bash
git add -A app/src
git commit -m "feat(app): portal ticket list and ticket page

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 9: Profile page (React, lane D)

**Files:**
- Create: `app/src/portal/pages/ProfilePage.tsx`, test
- Modify: `app/src/App.tsx` placeholder

**Interfaces:**
- Consumes: `getMe`, `updateMe`, `setPassword`, `usePortalAuth`, `FormTable`, `FormActions`, `useBanner`.

- [ ] **Step 1: Failing test**

`ProfilePage.test.tsx`: Name form patches `{ name }` and flashes "Profile saved"; Password section: when `has_password` shows Current password, New password, Confirm and posts `{ password, current_password }`; when not, heading "Set a password" and posts `{ password }`; mismatch is a field error; the `#password` hash focuses the password section (scrolls into view; assert the section has focus or `scrollIntoView` was called).

- [ ] **Step 2: Implement and commit**

Two `<form>`s each with a `FormTable` section and `FormActions` (cancelTo `/portal/tickets`). After a successful password set, refetch `me` so `has_password` flips.

```bash
git add -A app/src
git commit -m "feat(app): portal profile and password pages

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---
### Task 10: Wiring, staff-side touches, outbox detail, e2e leg, docs (integration, after all lanes)

**Files:**
- Modify: `gin/cmd/api/main.go` (construct `client.NewTokens`, `client.NewService`, `client.NewPortalService`, `client.NewHandler`, `client.NewLimiter`; mount), `gin/internal/server/server.go` (no change needed if the handler builds its own groups from `public`: `portal := public.Group("/portal")`, `user := portal.Group("", client.RequireUser(...))`, `account := user.Group("", client.RequireAccount())`)
- Modify: `gin/internal/mail/handler.go` (+ `GET /email/outbox/:id` returning the full row incl. `body_text` and `body_html`, admin only) + test; `gin/db/queries/mail.sql` (`GetOutbox` already exists per Task 1 of the email slice; reuse)
- Modify: `gin/internal/ticket/thread.go` (thread entries expose `user_id` and the poster name for customer messages — `Entry.UserID *int64 `json:"user_id"``), `app/src/components/ThreadEntry.tsx` (poster shown as-is; nothing else), `app/src/pages/TicketDetailPage` untouched
- Modify: `app/src/api/email.ts` + `OutboxPage` (a "View" action opening the outbox detail in a modal or a route `/admin/email/outbox/:id` showing the text body; needed so the e2e walk can read the link and so admins can inspect a queued mail)
- Modify: `app/e2e/walk.mjs` (portal leg), `app/e2e/README.md`, `app/README.md` (Portal section), `gin/README.md` (portal API section, `V5`, env)
- Modify: `docs/superpowers/specs/2026-09-26-customer-portal-design.md` only if execution rulings changed anything (list them)

**Interfaces:** consumes everything; produces `GET /api/v1/email/outbox/:id` → outbox JSON plus `body_text`, `body_html`.

- [ ] **Step 1: Wire the API**

In `serve`:

```go
ctokens := client.NewTokens(cfg.JWTSecret, accessTTL)
ident := client.NewService(pool, ctokens, refreshTTL, notifier, cfg.Mail.BaseURL, cfg.Mail.SiteName)
fileSvc := attachment.NewService(pool, store, cfg.MaxUploadBytes, cfg.AllowedMIME)
portalSvc := client.NewPortalService(pool, ticket.NewService(pool, ticket.WithNotifier(notifier)), fileSvc, ident, cfg.Mail.SiteName)
portalH := client.NewHandler(ident, portalSvc, client.NewLimiter(10, time.Minute), client.NewLimiter(10, time.Hour), ctokens, cfg.MaxUploadBytes)
```

and in `Mount`: `portalH.Mount(public)` which builds the `portal`/`user`/`account` groups and calls `MountAuth` and `MountPortal`. `cfg.Mail.BaseURL` may be empty when mail is disabled; the identity service then builds links from `APP_BASE_URL` — check `config` for a non-mail base URL field; if none, add `AppBaseURL` read from `APP_BASE_URL` regardless of `MAIL_ENABLED` (config test). Update `gin/cmd/api/main_test.go` route table test if one exists.

- [ ] **Step 2: Outbox detail endpoint and page**

`mail/handler.go`: `admin.GET("/email/outbox/:id", h.getOutbox)` returning the existing outbox JSON plus `body_text` and `body_html`; test. `app/src/api/email.ts`: `getOutboxItem(id)`; `OutboxPage`: subject cell links to `/admin/email/outbox/:id`, a small page `OutboxItemPage` showing headers and the text body in a `<pre>`; route + test.

- [ ] **Step 3: Staff-side thread poster**

`ticket.Thread` entries: add `UserID *int64` to `Entry` and read `user_id` in the thread query row (extend `ListThreadEntries` in `db/queries/thread.sql` to select `user_id`); the staff `ThreadEntry` already shows `poster`, and `AppendMessage` sets the poster to the customer's name, so no UI change beyond the JSON field. Test: `thread_test.go` asserts `user_id` round-trips.

- [ ] **Step 4: Gates**

Run: `cd gin && go build ./... && go vet ./... && gofmt -l ./cmd ./internal && make test` → record the coverage line. Run: `cd app && npm test && npm run lint && npm run build`.

- [ ] **Step 5: e2e portal leg**

Append to `app/e2e/walk.mjs` after the staff leg (same browser, new context so the staff session is not reused):

```js
const ctx2 = await browser.newContext({ viewport: { width: 1280, height: 900 } })
const portal = await ctx2.newPage()
await portal.goto(`${BASE}/portal`)
await shotP('07-portal-landing')
await portal.getByRole('link', { name: 'Open a New Ticket' }).click()
await portal.getByLabel(/Name/).fill('Walk Customer')
await portal.getByLabel(/Email/).fill('walk-customer@example.test')
await portal.getByLabel(/Subject/).fill('Portal walk ticket')
await portal.getByLabel(/Message/).fill('Opened from the portal walk')
await portal.getByRole('button', { name: 'Open Ticket' }).click()
await portal.getByText(/Ticket #\d+ opened/).waitFor()
await shotP('08-portal-opened')
const number = (await portal.getByText(/Ticket #\d+ opened/).textContent()).match(/#(\d+)/)[1]

// Request a guest access link, then read the emailed link from the outbox via the admin API
// (the staff page is still signed in on the first context).
await portal.goto(`${BASE}/portal/login`)
await portal.getByLabel(/Ticket Number/).fill(number)
await portal.getByLabel(/^Email/).last().fill('walk-customer@example.test')
await portal.getByRole('button', { name: /access link/i }).click()
await portal.getByText(/sent an access link/i).waitFor()
await page.goto(`${BASE}/admin/email/outbox?status=pending`)
await page.getByRole('link', { name: /Access link/ }).first().click()
const body = await page.locator('pre').first().textContent()
const link = body.match(/https?:\S+\/portal\/t\/[0-9a-f]+/)[0]
await portal.goto(link)
await portal.getByRole('heading', { name: /Portal walk ticket/ }).waitFor()
await shotP('09-portal-ticket')
await portal.getByLabel('Reply').fill('Customer reply from the walk')
await portal.getByRole('button', { name: 'Post Reply' }).click()
await portal.getByText('Reply posted').waitFor()
await portal.getByRole('button', { name: 'Close ticket' }).click()
await portal.getByRole('button', { name: 'Confirm' }).click()
await portal.getByText(/Closed/).first().waitFor()
await shotP('10-portal-closed')
await ctx2.close()
```

where `shotP` screenshots the portal page. The outbox link text match depends on the subject "[#n] Access link for …" — the outbox page's subject cell must be the link to the detail page (Step 2). The staff leg ends signed in as the admin; keep `page` open until the portal leg has read the outbox. Run the walk against the local stack (API rebuilt from this branch, `POSTGRES_PORT=5433` for any compose command, migrations applied) and keep the ten screenshots.

- [ ] **Step 6: Docs and commit**

`app/README.md`: "Customer portal" section (routes, session key, guest sessions, how the walk reads links from the outbox). `gin/README.md`: portal API table, `V5` notes (backfill, nullable outbox ticket), env `APP_BASE_URL` used for portal links even when mail is off. `app/e2e/README.md`: the portal leg.

```bash
git add -A app gin docs
git commit -m "feat: wire the customer portal, outbox detail, e2e leg, and docs

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

## Self-review notes

- Spec coverage: §2 → Task 1 (+ Task 2's outbox nullability, an addition the spec did not foresee: account mail has no ticket); §3 → Tasks 2–3; §4 → Task 4 (+ Task 10 wiring); §5 → Tasks 5–9; §6 → error mapping in Task 2, pages in 6–9; §7 → per task plus Task 10 gates and e2e leg; staff-side touches → Task 10.
- Plan-level deviations to rule on at execution: `email_outbox.ticket_id` becomes nullable (spec silent); the `reset` session is an access token with a `pwr` claim and no refresh token (spec said "short-lived session flagged reset"); `via: "portal"` in the created event; anonymous uploads store `uploaded_by = NULL` and are GC'd by the existing job; `ExternalCreateInput` gains `Source`, `UserID`, `TopicID`.
- Type consistency: `client.Session.Kind` ↔ `PortalSession.kind` (Task 5 handlers and Task 7 TokenPage); `Profile` ↔ `PortalProfile`; `TicketRow`/`TicketView`/`EntryView` ↔ `PortalTicketRow`/`PortalTicket`/`PortalEntry`; `Reference` ↔ `PortalReference`; `Opened` ↔ `{ id, number }`; error codes `token_invalid`, `guest_session`, `rate_limited` + `retry_after` used by the pages.
- Review Focus pins: 1 → Task 2/3 (`TestUpsertByEmailIsCaseInsensitive`, `TestRegisterAttachesToAnonymousUser`); 2 → Task 3 `TestGuestSessionScopedToTicket`, Task 4 `TestGuestCannotListTickets`/`TestGuestCanViewAndReplyOwnTicketOnly`; 3 → Task 2 `TestClientTokenRoundTripAndAudience`, `TestRequireUserAndAccount`; 4 → Task 4 `TestReplyReopensClosedTicket`; 5 → Task 3 `TestRegisterConfirmLogin` (second exchange → 410).
