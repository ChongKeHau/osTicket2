# Go Ticket API: core ticketing slice

Date: 2026-09-24
Status: approved design, pending implementation plan

## 1. Purpose

Replace the osTicket PHP backend with a Go service that exposes a JSON API for a new
React frontend. This document covers the first sub-project only: core ticketing for
internal support agents. Later sub-projects (customer portal, email, custom forms,
knowledge base, tasks, SLAs, MySQL import) each get their own spec.

### What was agreed

- Go with Gin. Module root is the `gin/` directory at the repository root, using the
  standard `internal/` layout.
- PostgreSQL. Flyway owns the schema; nothing else creates or alters tables.
- JSON API only. The service never renders HTML.
- First users are internal agents. Roles are `admin` and `agent`.
- Fixed ticket schema plus a reserved `extra jsonb` column for future custom fields.
- JWT access token plus a server-side, revocable refresh token.
- Feature-oriented packages with a thin service layer; `sqlc` generates typed query code.
- Test coverage must stay above 75% of statements across `internal/`, enforced by
  `make test`.

### Assumptions

- The database starts empty. Importing agents or tickets from MySQL is a later project.
- Attachments are stored on local disk behind a storage interface so S3 can be added later.
- No email or notifications of any kind in this slice.

### Success criteria

An agent can log in from the React app, see the tickets in their departments, open one,
read its thread, reply, add an internal note, attach files, assign, transfer, and change
status. An admin can manage departments, help topics, and staff. All of this is covered by
automated tests that run against a real Postgres.

## 2. Project layout and tooling

```
gin/
  go.mod                      module github.com/grandpine/ticket-api
  cmd/api/main.go             subcommands: serve, create-admin, gc-files
  internal/
    config/                   env-based config, validated at startup
    server/                   Gin engine, middleware, route registration
    httpx/                    error envelope, binding, pagination helpers
    auth/                     login, refresh, logout, JWT issue/verify, middleware
    staff/                    agents: CRUD, password hashing, department membership
    dept/                     departments
    topic/                    help topics
    ticket/                   tickets, status transitions, assignment, threads, events
    attachment/               upload, download, storage interface, local disk backend
    db/                       sqlc-generated code, pgx pool, WithTx helper
  db/
    migrations/               Flyway: V1__init.sql, V2__seed.sql
    queries/                  sqlc input: one .sql file per feature
  sqlc.yaml
  flyway.conf
  Makefile                    migrate, sqlc, test, cover, run
  docker-compose.yml          Postgres for local dev
```

Rules:

- Each feature package has `handler.go` (Gin handlers), `service.go` (business rules),
  and calls the generated `db` package directly. There is no hand-written repository layer.
- Handlers never import `db`. Services never import `gin`.
- Services expose an interface that handlers depend on, so handler tests can use a fake.

Stack: Go 1.22 or newer, `gin-gonic/gin`, `jackc/pgx/v5`, `sqlc`, `golang-jwt/jwt/v5`,
`golang.org/x/crypto/bcrypt`, Gin's built-in `go-playground/validator` binding,
`log/slog`. Flyway runs as the official Docker image through `make migrate`.

## 3. Data model

Flyway `V1__init.sql` creates the schema; `V2__seed.sql` inserts reference data.
All ids are `bigint generated always as identity`. Every table has `created_at` and
`updated_at` as `timestamptz not null default now()`.

| Table | Columns |
|---|---|
| `staff` | `username` text unique, `email` text unique, `password_hash` text, `first_name`, `last_name`, `is_admin` bool, `is_active` bool default true, `primary_dept_id` references `department` |
| `staff_department` | `staff_id`, `dept_id`, primary key (`staff_id`, `dept_id`). Additional departments an agent may see. |
| `refresh_token` | `token_hash` text unique, `staff_id`, `expires_at`, `revoked_at` nullable |
| `department` | `name` text unique, `is_public` bool, `manager_id` nullable references `staff` |
| `help_topic` | `name` text, `dept_id` nullable, `priority_id` nullable, `is_active` bool, `sort_order` int |
| `ticket_priority` | `name`, `urgency` int, `color` text. Seeded: low, normal, high, emergency. |
| `ticket_status` | `name` text, `state` enum (`open`, `resolved`, `closed`), `sort_order`. Seeded: Open, Resolved, Closed. |
| `ticket` | `number` text unique, `subject` text, `status_id`, `dept_id`, `topic_id` nullable, `priority_id`, `assigned_staff_id` nullable, `requester_name`, `requester_email`, `source` enum (`web`, `api`, `phone`, `other`), `is_answered` bool, `due_at` nullable, `closed_at` nullable, `last_message_at`, `last_response_at` nullable, `extra jsonb not null default '{}'`, `search tsvector` generated from `subject` |
| `thread_entry` | `ticket_id`, `type` enum (`message`, `response`, `note`), `staff_id` nullable, `poster` text, `title` text nullable, `body` text, `format` enum (`html`, `text`), `parent_id` nullable self-reference |
| `ticket_event` | `ticket_id`, `staff_id` nullable, `kind` enum (`created`, `assigned`, `unassigned`, `status_changed`, `transferred`, `closed`, `reopened`, `edited`), `data jsonb` |
| `file` | `key` text unique, `name`, `mime`, `size` bigint, `sha256` text, `backend` text, `uploaded_by` nullable references `staff` |
| `attachment` | `thread_entry_id`, `file_id`, `inline` bool, primary key (`thread_entry_id`, `file_id`) |

Indexes: `ticket (status_id)`, `ticket (dept_id)`, `ticket (assigned_staff_id)`,
`ticket (last_message_at desc)`, GIN on `ticket.search`, `thread_entry (ticket_id, id)`,
`ticket_event (ticket_id, id)`, `refresh_token (staff_id)`.

Decisions:

- The requester is stored inline on the ticket. A `user` table belongs to the portal
  sub-project; adding `user_id` then is a single migration.
- Ticket numbers come from a Postgres sequence `ticket_number_seq`, formatted as six
  zero-padded digits. Configurable number formats are out of scope.
- Search covers the subject only. Thread bodies are not indexed in this slice.
- Circular reference between `staff.primary_dept_id` and `department.manager_id` is
  handled by creating `department` first with `manager_id` added via `alter table`
  after `staff` exists.
- V2 seeds priorities, statuses, one department named Support, one help topic named
  General Inquiry, and no staff. The first admin is created with
  `cmd/api create-admin --username --email --password` so no credential lands in a
  migration.

## 4. API surface

Base path `/api/v1`. JSON request and response bodies. Ids are numeric in paths.
Timestamps are RFC 3339 in UTC. Field names are `snake_case`.

Unauthenticated routes: `POST /auth/login`, `POST /auth/refresh`, `GET /health`.
Everything else requires `Authorization: Bearer <access token>`.

### Auth

- `POST /auth/login` body `{username, password}`. Returns
  `{access_token, refresh_token, expires_in, staff}`. Access tokens live 15 minutes,
  refresh tokens 14 days. Wrong credentials or inactive staff return 401 without
  distinguishing which.
- `POST /auth/refresh` body `{refresh_token}`. Revokes the presented token, issues a
  new pair. Expired, revoked, or unknown tokens return 401.
- `POST /auth/logout` body `{refresh_token}`. Revokes it. Always 204.
- `GET /me` returns the current staff record plus `department_ids`.

### Reference data

Reads are open to any authenticated agent. Writes require admin.

- `GET /departments`, `POST /departments`, `GET /departments/:id`,
  `PATCH /departments/:id`, `DELETE /departments/:id`. Delete returns 409 if any
  ticket or staff references the department.
- `GET /topics`, `POST /topics`, `GET /topics/:id`, `PATCH /topics/:id`,
  `DELETE /topics/:id`. Delete returns 409 if any ticket references the topic.
- `GET /priorities`, `GET /statuses`. Read-only seeded lists.
- `GET /staff`, `POST /staff`, `GET /staff/:id`, `PATCH /staff/:id`,
  `POST /staff/:id/password` body `{password}`. No hard delete; set `is_active` false.
  `PATCH /staff/:id` accepts `department_ids` to replace `staff_department` rows.
  Deactivating a staff member revokes all their refresh tokens.

### Tickets

- `GET /tickets` query params: `status` (status id), `state` (open, resolved, closed),
  `dept_id`, `assigned_to` (staff id, `me`, or `none`), `q` (subject search),
  `sort` (`created_at`, `last_message_at`, `priority`, each with optional `-` prefix
  for descending; default `-last_message_at`), `page` (default 1), `page_size`
  (default 25, max 100). Returns `{items, page, page_size, total}`. Each item embeds
  `department`, `topic`, `priority`, `status`, and `assignee` as `{id, name}` objects.
- `POST /tickets` body `{subject, message, message_format, requester_name,
  requester_email, dept_id, topic_id, priority_id, source, due_at, extra, file_ids}`.
  `subject`, `message`, and `requester_email` are required. If `topic_id` is given,
  its `dept_id` and `priority_id` are used as defaults for any omitted values. If no
  department can be determined, 400. Creates the ticket, its first `message` entry,
  links files, and writes a `created` event, all in one transaction. Returns 201 with
  the full ticket.
- `GET /tickets/:id` returns the full ticket with embedded reference objects.
- `PATCH /tickets/:id` accepts `subject`, `priority_id`, `topic_id`, `due_at`, `extra`,
  `requester_name`, `requester_email`.
  Writes an `edited` event containing the changed field names.
- `POST /tickets/:id/reply` body `{body, format, status_id, file_ids}`. Adds a
  `response` entry, sets `is_answered` true and `last_response_at`, optionally changes
  status using the same rules as the status endpoint, in one transaction.
- `POST /tickets/:id/notes` body `{title, body, format, file_ids}`. Adds a `note` entry.
- `GET /tickets/:id/thread` query `after` (entry id cursor), `limit` (default 50,
  max 200). Entries oldest first, each with `attachments` `[{file_id, name, mime, size}]`.
- `POST /tickets/:id/status` body `{status_id}`. Rules: moving from `open` state to
  `resolved` or `closed` sets `closed_at` and writes a `closed` event; moving from
  `resolved` or `closed` to `open` clears `closed_at` and writes a `reopened` event;
  any other change writes `status_changed`. Same status is a no-op returning 200.
- `POST /tickets/:id/assign` body `{staff_id}` or `{staff_id: null}`. Assignee must be
  active and able to see the ticket's department. Writes `assigned` or `unassigned`.
- `POST /tickets/:id/transfer` body `{dept_id}`. The caller must be able to see the
  target department. If the current assignee cannot see the new department, the ticket
  is unassigned in the same transaction. Writes `transferred`.
- `GET /tickets/:id/events` returns the audit trail oldest first, each with the acting
  staff as `{id, name}` when present.

### Files

- `POST /files` multipart field `file`. Enforces a size cap and MIME allowlist from
  config. Stores the file under the local backend, computes sha256, returns 201 with
  `{id, name, mime, size}`.
- `GET /files/:id` streams the file with `Content-Disposition: attachment`. Allowed only
  if the file is attached to a ticket the caller can see, or if the caller uploaded it
  and it is not yet attached. A file's uploader is recorded in `file.uploaded_by`
  (nullable `staff_id`).
- `cmd/api gc-files` deletes files older than 24 hours that have no `attachment` row.
- `file_ids` on create, reply, and notes must reference files that are not already
  attached; otherwise 409.

### Conventions

- Error envelope:
  `{"error": {"code": "...", "message": "...", "fields": {"subject": "required"}}}`.
  Codes and statuses: `validation_failed` 400, `unauthorized` 401, `forbidden` 403,
  `not_found` 404, `conflict` 409, `payload_too_large` 413, `internal` 500.
- List envelope: `{items, page, page_size, total}` for offset lists;
  `{items, next_after}` for the thread cursor.
- CORS allows the React origin(s) from config. No CSRF protection is needed because
  auth is a bearer header, never a cookie.

## 5. Authorization

- The JWT carries `sub` (staff id), `role` (`admin` or `agent`), `exp`, `iat`, and a
  `jti`. Department membership is not in the token; it is loaded per request from
  `staff` and `staff_department` so changes take effect immediately.
- Admin may do anything.
- Agent visibility is the set of `primary_dept_id` plus `staff_department` rows. Agents
  may list, read, create, reply, note, assign, transfer, edit, and change status on
  tickets in visible departments. Creating a ticket into a department the agent cannot
  see is 403. Transfer targets must be visible to the caller.
- The department filter is applied inside the ticket service on every query, never in
  handlers, so a new endpoint cannot forget it.
- A ticket outside the caller's visibility returns 404, not 403, to avoid leaking
  existence.

## 6. Error handling, transactions, observability

- Services return typed errors: `ErrNotFound`, `ErrForbidden`, `ErrConflict`, and
  `*ValidationError{Fields map[string]string}`. `httpx` maps them to the envelope.
  Any other error becomes 500, is logged with the request id, and the client sees only
  `internal`.
- `db.WithTx(ctx, pool, fn)` wraps multi-row writes. Ticket create, reply with status
  change, transfer, and staff deactivation each run in a single transaction.
- Middleware: request id (`X-Request-Id`, generated if absent), structured `slog`
  request logging, panic recovery returning the 500 envelope, CORS, auth.
- `GET /health` pings the pool with a one-second timeout; returns 503 on failure.
- Config is read from environment variables (`DATABASE_URL`, `JWT_SECRET`, `PORT`,
  `STORAGE_DIR`, `CORS_ORIGINS`, `MAX_UPLOAD_BYTES`, `ALLOWED_MIME`). Startup fails
  fast if `DATABASE_URL` or `JWT_SECRET` is missing or `JWT_SECRET` is shorter than
  32 bytes.

## 7. Testing

Coverage requirement: `make test` runs `go test ./... -coverprofile` across `internal/`
and fails if total statement coverage is 75% or lower. Generated code under
`internal/db` is excluded from the coverage denominator.

- Service tests use a real Postgres from `testcontainers-go`. One container per
  package test binary, Flyway applied once via the Flyway Docker image (or a Go
  helper that applies the `db/migrations` files in order for speed). Each test runs
  inside a transaction that is rolled back. These tests cover status transition rules,
  department visibility, assignment validation, transfer unassignment, thread ordering
  and cursoring, ticket number generation, and file linking conflicts.
- Handler tests use `httptest` against the real Gin router with fake service
  implementations. They cover request binding, validation errors, the error envelope,
  auth middleware, role checks, and pagination parameter parsing. No database.
- Auth tests cover login success and failure, token expiry, refresh rotation, reuse of
  a revoked refresh token, and logout.
- Attachment tests cover size cap, MIME allowlist, download authorization, and gc.
- Migration test boots Postgres, applies all migrations from empty, and asserts that a
  second apply is a no-op.
- Docker is required for the database-backed suites. CI configuration is out of scope,
  but `make test` is the single entry point it would call.

## 8. Out of scope for this slice

Customer portal and user accounts, email in and out, notifications, custom forms,
canned responses, SLAs, teams, tasks, knowledge base, ticket filters, plugins, MySQL
data import, full osTicket role permissions, ticket locking, collaborators, configurable
ticket number formats, and full-text search over thread bodies.
