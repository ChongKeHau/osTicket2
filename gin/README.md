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

`create-admin` also takes optional `--first-name`/`--last-name` (first name
defaults to `--username`, last name defaults to empty, when left out).

## Importing an existing osTicket database

`import-osticket` copies an osTicket (1.18) MySQL database into a fresh API database:
departments, staff (passwords carry over), help topics, priorities, statuses, tickets with
their threads and attachments, and ticket history. Run it once, against a target that has
only the seed rows (it refuses otherwise), with the API stopped:

    DATABASE_URL=postgres://... STORAGE_DIR=./storage \
      go run ./cmd/api import-osticket \
        --mysql-dsn 'ost:secret@tcp(127.0.0.1:3306)/osticket' \
        --prefix ost_ --timezone Asia/Kuala_Lumpur \
        --files-dir /var/www/osticket/attachments   # only if the filesystem storage plugin was used

Flags: `--prefix` (default `ost_`), `--timezone` (zone of the MySQL datetimes, default `UTC`),
`--files-dir` (an existing directory; needed for files stored by the filesystem plugin at any
nesting depth up to 3; database-stored files need nothing), `--batch` (rows per insert, default 500), `--dry-run` (validate and report only).

Ids and ticket numbers are preserved unless a source id collides with a seed row, in which
case the row is renumbered and the report says so. New tickets created by the API afterwards
are numbered above the highest imported number. The report lists, per entity, rows read,
written, merged into seed rows, and skipped with reasons; it is printed on stdout, and logs go
to stderr. Text containing NUL bytes or invalid UTF-8 (which PostgreSQL rejects) is cleaned
and noted in the report. Exit codes: 0 done; 1 aborted (earlier steps stay committed: drop
and re-migrate the target before retrying; the importer removes the attachment files it wrote
in the failed step, while files from earlier committed steps stay with their rows and go when
you drop the target and empty `STORAGE_DIR`); 2 target not empty;
3 done but some tickets or attachments were skipped for reasons other than a deleted
status, listed in the report. Not imported: SLAs, teams, collaborators, custom form fields
other than subject and priority, canned responses, knowledge base, email settings.

`POST /auth/login` rate-limits by `username + client IP`: 10 attempts per 60s
window, then `429 rate_limited` until the window rolls over; a successful
login resets the counter.

If port 5432 is already in use on your machine, set `POSTGRES_PORT` (e.g.
`export POSTGRES_PORT=5433`) before `make migrate`/`make db-up`, and point
`DATABASE_URL` at that port instead.

`db/migrations/V1__init.sql` and `V2__seed.sql` are frozen once this branch
merges: Flyway checksums them, so editing either file after that point will
make every existing database fail migration with a checksum mismatch. Future
schema changes go in new `V3__*.sql` and up files instead. If you have a
`pgdata` volume left over from an earlier state of this branch (before the
migrations settled), you'll hit that same checksum error on `make migrate`;
run `docker compose down -v` to drop the volume and start clean.

## Configuration (environment)

| Variable | Default | Notes |
|---|---|---|
| DATABASE_URL | required | pgx connection string |
| JWT_SECRET | required | at least 32 bytes |
| PORT | 8080 | |
| STORAGE_DIR | ./storage | attachment files |
| CORS_ORIGINS | none | comma-separated allowed origins |
| TRUSTED_PROXIES | none | comma-separated IPs/CIDRs allowed to set X-Forwarded-For/X-Real-IP; empty trusts none, so `ClientIP()` (used by the login rate limiter) is always the socket address |
| MAX_UPLOAD_BYTES | 10485760 | per file |
| ALLOWED_MIME | images, pdf, text, csv, zip, office | comma-separated |
| APP_BASE_URL | none | public origin of the web app, e.g. `https://desk.example.com`; portal links (`${APP_BASE_URL}/portal/t/<token>`) are built from it whether or not mail is on; required when `MAIL_ENABLED=true` |
| MAIL_EXPOSE_LINKS | false | **dev/e2e only.** `true` shows portal links in `client_*` outbox mail unredacted to admins (see Email), which lets any admin sign in as that customer; the e2e walk needs it. Never set it in production; the API logs a warning at start when it is on |

## Email

Set `MAIL_ENABLED=true` to send notifications and read a mailbox. For Zoho Mail only four
values are needed; everything else defaults to Zoho's servers:

    MAIL_ENABLED=true
    MAIL_FROM="Support <support@yourdomain.com>"
    ZOHO_APP_TOKEN=<application-specific password created in Zoho for app "ticketing">
    APP_BASE_URL=https://desk.yourdomain.com

| Variable | Default | Notes |
|---|---|---|
| SMTP_HOST / SMTP_PORT | smtppro.zoho.com / 465 | |
| SMTP_TLS | `implicit` on port 465, `starttls` on any other port | `implicit`, `starttls` or `none` |
| SMTP_USER / SMTP_PASSWORD | MAIL_FROM address / ZOHO_APP_TOKEN | |
| IMAP_HOST / IMAP_PORT | imappro.zoho.com / 993 | |
| IMAP_TLS | implicit | `implicit` or `none` (no STARTTLS) |
| IMAP_USER / IMAP_PASSWORD / IMAP_FOLDER | MAIL_FROM address / ZOHO_APP_TOKEN / INBOX | |
| MAIL_POLL_INTERVAL / MAIL_SEND_INTERVAL | 60s / 5s | Go durations; at least 10s / at least 1s |
| MAIL_SITE_NAME | Ticket Desk | used in templates |
| MAIL_DEFAULT_DEPT_ID | the seed department | department for tickets opened by mail |

What is sent: an auto-response when a ticket is created with a requester email, the agent's
reply to the requester, an alert to an agent when a ticket is assigned to them, and an alert to
the assignee (or the whole department when unassigned) when a requester writes back by mail.
Notes, transfers and status changes send nothing. Mail is queued in the `email_outbox` table in
the same transaction as the ticket change and delivered by a background loop with retries
(1, 5, 15, then 60 minutes, `failed` after 10 attempts).

What is read: unseen messages in the mailbox, every `MAIL_POLL_INTERVAL`. A reply from the
ticket's requester (matched by our `Message-ID`s or a `[#number]` subject tag) is appended to
the ticket; anything else opens a new ticket with the sender as requester. Auto-replies,
bounces, and mail from our own address are ignored and logged. Attachments obey
`MAX_UPLOAD_BYTES` and `ALLOWED_MIME`.

Commands: `api mail-test --to you@example.com` sends one test message through the configured
SMTP settings. `api mail-worker` runs the sender and poller without the HTTP server, for
deployments that want mail out of the web process (running it alongside `serve` is safe).

Admin API (admin staff only): `GET /api/v1/email/templates`,
`PATCH /api/v1/email/templates/:key` (Go `text/template` and `html/template` with `.SiteName
.Number .Subject .RequesterName .RequesterEmail .AgentName .Link .Message .MessageHTML`),
`GET /api/v1/email/outbox?status=`, `GET /api/v1/email/outbox/:id` (one row plus its
`body_text` and `body_html`), `POST /api/v1/email/outbox/:id/retry`,
`GET /api/v1/email/inbound`. An outbox row's `ticket_id` is `null` for account mail (confirm,
sign-in and reset links). In `client_*` rows every `/portal/t/<token>` link is shown as
`/portal/t/[redacted]` (bodies and subject, list and detail): those links sign the customer
in, so an admin reading the outbox must not be able to use them. The stored row is not
changed; the mail still goes out with the live link.

## Customer portal

Package `internal/client`, mounted under `/api/v1/portal` (spec:
`docs/superpowers/specs/2026-09-26-customer-portal-design.md`). End users are a separate
identity from staff: JWTs carry `aud: "client"` (the staff middleware rejects them and vice
versa), 15-minute access tokens, rotating refresh tokens stored hashed in
`client_refresh_token`. Emailed tokens are 32 random bytes, stored as SHA-256, single use;
`confirm` and `reset` last 24 h, `signin` and `access` 1 h.

| Route | Session | Notes |
|---|---|---|
| `POST auth/login` | none | `{email, password}` → session; 401 generic message |
| `POST auth/link`, `POST auth/reset`, `POST access` | none | `{email}` / `{email, number}` → always 202 `{}`; the lookup and mail run in the background |
| `POST auth/register` | none | `{email, name}` → always 201 `{}`; mails `client_confirm` unless the address already has a password |
| `POST auth/exchange` | none | `{token}` → session with `kind` (`confirm`, `signin`, `access`, `reset`); 410 `token_invalid`. `confirm`/`reset` give a password-setting session (no refresh token) |
| `POST auth/refresh`, `POST auth/logout` | refresh / any | as for staff |
| `GET me` | any (also password-setting) | profile plus `ticket_id` (the guest's ticket; `null` for an account) |
| `POST me/password` | account (also password-setting) | `{password, current_password?}` → a fresh full session; every earlier refresh token of the user is revoked |
| `PATCH me` | account | `{name}` |
| `GET reference` | none | public departments, active topics, site name |
| `POST tickets` | optional | `{name, email, subject, message, format, topic_id?, dept_id?, file_ids?, file_tokens?}` → 201 `{id, number}`; `0` means no topic / department |
| `POST files` | optional | multipart `file` → file JSON plus `token`, which must be sent back in `file_tokens` |
| `GET tickets/:id`, `POST tickets/:id/reply`, `GET tickets/:id/files/:fileId` | any (guest: its ticket only) | reply 204; replying to a closed or resolved ticket reopens it |
| `POST tickets/:id/close`, `POST tickets/:id/reopen` | any (guest: its ticket only) | 200 ticket; 409 when already in that state (resolved counts as closed) |
| `GET tickets?state=&page=&page_size=` | account | own tickets; `state=closed` lists resolved tickets too (the portal has no Resolved tab); guests get 403 `guest_session` |

Rate limits (fixed window, 429 with `retry_after`): sign-in, link, reset, access and register
share 10 a minute per email and per IP; ticket opens 10 an hour per email (the session's
address when signed in) and per IP; uploads 10 an hour per IP. A session (account or guest)
does not lift the open or upload budgets.

Migration `V5__portal.sql` adds `end_user`, `client_token`, `client_refresh_token`,
`ticket.user_id`, `thread_entry.user_id` (staff thread entries expose it as `user_id`),
`file.access_token`, the four `client_*` email templates, and makes
`email_outbox.ticket_id` nullable (account mail belongs to no ticket). It backfills one end
user per distinct requester address (named from that address's latest ticket) and links
existing tickets to them. From then on every new ticket (staff, API, inbound mail or portal)
is linked to the end user for its `requester_email` (matched case-insensitively, created when
missing), and a staff edit of `requester_email` moves the ticket to the new address's end user.

## Dashboard

`GET /api/v1/dashboard/stats?start=YYYY-MM-DD&period=30` (any signed-in staff) counts ticket
events per UTC day in the window `[start, start+period)`.

| Param | Default | Notes |
|---|---|---|
| `period` | 30 | days; one of 7, 14, 30, 90 |
| `start` | today − (period − 1), so the window ends today | UTC date |

A bad `period` or `start` returns 400 with the usual error envelope, naming the field.
Non-admin staff only see events of tickets in their own departments.

Response: `start`, `period`, `series` (exactly `period` entries of `{date, opened, assigned,
closed, reopened}`, zero-filled), and three breakdowns `by_department`, `by_topic`,
`by_staff`, each a list of `{id, name, opened, assigned, closed, reopened}`. Rows whose
counts are all zero are left out; a ticket without a help topic is counted under `id: null,
name: "— none —"`, and an event with no staff actor under `id: null, name: "— system —"`.

## Commands

    make test               # full suite with coverage gate (> 75%)
    make sqlc               # regenerate internal/db from db/queries
    go run ./cmd/api gc-files --older-than 24h

## Layout

`cmd/api` entrypoint; `internal/<feature>` packages each with `handler.go` and
`service.go`; `internal/db` is sqlc output plus pool and transaction helpers;
`db/migrations` is owned by Flyway.

The React frontend lives in ../app (see its README).
