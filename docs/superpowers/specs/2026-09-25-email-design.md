# Email: outbound notifications and inbound mail

Date: 2026-09-25
Status: approved design, pending implementation plan
Depends on: `docs/superpowers/specs/2026-09-24-go-ticket-api-design.md` (the Go API), its
PostgreSQL schema (`gin/db/migrations/V1__init.sql`, `V2__seed.sql`, frozen; this slice adds
`V3__email.sql`).

## 1. Purpose

Make the ticket system usable by email. The API sends mail when a ticket is created, when an
agent replies, when a ticket is assigned, and when a requester writes back; and it reads one
mailbox so requesters can open tickets and reply by email, with their messages landing in
the right ticket thread. Nothing loops on auto-replies or bounces, and no notification is
lost when the mail server is down.

### What was agreed

- One SMTP account and one IMAP mailbox, configured by environment variables. The target is
  Zoho Mail: IMAP `imappro.zoho.com:993` (implicit TLS), SMTP `smtppro.zoho.com` on 465
  (implicit TLS) or 587 (STARTTLS), authenticated with an application-specific password
  supplied as `ZOHO_APP_TOKEN`.
- Outbound and inbound in one slice, built in that order (inbound matches replies on the
  message ids that outbound stamps).
- Notifications are written to an outbox table in the same transaction as the ticket change
  and sent by a worker loop with retries. No external queue.
- Inbound mail is polled over IMAP on an interval; no IMAP IDLE, no piping.
- Templates live in the database, seeded by the migration, editable through an admin API.
  No frontend screens in this slice.
- New Go dependencies: `github.com/emersion/go-imap/v2` and `github.com/emersion/go-message`.
  SMTP uses the standard library.

### Assumptions

- The mailbox in `MAIL_FROM` is the same account the IMAP poller reads, so replies to any
  notification come back to us.
- Requesters are identified by email address only; the API has no user accounts.
- A ticket's requester is the only outside party who may append to it by mail. Mail from
  anyone else about an existing ticket opens a new ticket.
- The React app is reachable at `APP_BASE_URL`; ticket links are `<APP_BASE_URL>/tickets/<id>`.
- Zoho requires an app-specific password when two-factor auth is on; the operator creates it
  in Zoho (app name `ticketing`) and never commits it.

### Success criteria

With `MAIL_ENABLED=true` and a working Zoho account: creating a ticket with a requester
email sends them an auto-response carrying the ticket number; an agent reply reaches the
requester and, when they answer it, their answer appears as a message on the same ticket
and the assigned agent is emailed; a ticket assigned to an agent emails that agent; a fresh
email to the mailbox opens a ticket with the sender as requester and the body as the first
message; an auto-reply or bounce sent to the mailbox is recorded as ignored and creates
nothing. `make test` stays above 75% coverage.

## 2. Configuration and processes

All keys are read by `config.Load`. Mail keys are validated only when `MAIL_ENABLED=true`.

| Variable | Default | Notes |
|---|---|---|
| `MAIL_ENABLED` | `false` | nothing is sent or fetched until true |
| `MAIL_FROM` | required when enabled | `Support <support@example.com>` or a bare address; the address's domain is used in message ids |
| `ZOHO_APP_TOKEN` | none | fallback password for SMTP and IMAP |
| `SMTP_HOST` | `smtppro.zoho.com` | |
| `SMTP_PORT` | `465` | |
| `SMTP_TLS` | `implicit` for 465, `starttls` otherwise | `implicit`, `starttls`, or `none` (tests only) |
| `SMTP_USER` | the `MAIL_FROM` address | |
| `SMTP_PASSWORD` | `ZOHO_APP_TOKEN` | |
| `IMAP_HOST` | `imappro.zoho.com` | |
| `IMAP_PORT` | `993` | implicit TLS always; `IMAP_TLS=none` allowed for tests only |
| `IMAP_USER` | the `MAIL_FROM` address | |
| `IMAP_PASSWORD` | `ZOHO_APP_TOKEN` | |
| `IMAP_FOLDER` | `INBOX` | |
| `MAIL_POLL_INTERVAL` | `60s` | minimum `10s` |
| `MAIL_SEND_INTERVAL` | `5s` | sender wake-up |
| `APP_BASE_URL` | required when enabled | no trailing slash |
| `MAIL_SITE_NAME` | `Ticket Desk` | template variable |
| `MAIL_DEFAULT_DEPT_ID` | the seed department | department for tickets opened by mail |

Secrets are never logged. Startup logs the SMTP and IMAP hosts and users only.

Processes:

- `api serve` starts the sender loop and the IMAP poller as goroutines when mail is
  enabled and stops them on shutdown (they finish the message in flight, then return).
- `api mail-worker` runs the same two loops without the HTTP server, for deployments that
  want mail out of the web process. Running both at once is safe: outbox rows are claimed
  with `FOR UPDATE SKIP LOCKED`, and inbound processing is idempotent on `Message-ID`.
- `api mail-test --to ADDRESS` renders a fixed test message, sends it synchronously through
  the configured SMTP settings, and exits 0 on success or 1 with the SMTP error.

## 3. Data model (`V3__email.sql`)

```sql
ALTER TYPE ticket_source ADD VALUE 'email';

CREATE TABLE email_template (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  key        text NOT NULL UNIQUE,
  subject    text NOT NULL,
  body_html  text NOT NULL,
  body_text  text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TYPE email_status AS ENUM ('pending', 'sent', 'failed');

CREATE TABLE email_outbox (
  id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  ticket_id       bigint NOT NULL REFERENCES ticket(id) ON DELETE CASCADE,
  entry_id        bigint REFERENCES thread_entry(id) ON DELETE SET NULL,
  template_key    text NOT NULL,
  to_address      text NOT NULL,
  to_name         text NOT NULL DEFAULT '',
  subject         text NOT NULL,
  body_html       text NOT NULL,
  body_text       text NOT NULL,
  message_id      text NOT NULL UNIQUE,
  in_reply_to     text,
  status          email_status NOT NULL DEFAULT 'pending',
  attempts        int NOT NULL DEFAULT 0,
  last_error      text,
  next_attempt_at timestamptz NOT NULL DEFAULT now(),
  sent_at         timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX email_outbox_pending_idx ON email_outbox (next_attempt_at) WHERE status = 'pending';

CREATE TYPE inbound_outcome AS ENUM ('created', 'replied', 'ignored');

CREATE TABLE inbound_message (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  message_id   text NOT NULL UNIQUE,
  from_address text NOT NULL,
  from_name    text NOT NULL DEFAULT '',
  subject      text NOT NULL DEFAULT '',
  ticket_id    bigint REFERENCES ticket(id) ON DELETE SET NULL,
  entry_id     bigint REFERENCES thread_entry(id) ON DELETE SET NULL,
  outcome      inbound_outcome NOT NULL,
  reason       text NOT NULL DEFAULT '',
  received_at  timestamptz NOT NULL DEFAULT now()
);
```

Seed rows in the same migration for the four template keys below, with plain wording that
names the site, the ticket number, the subject, the link, and the message.

`ALTER TYPE … ADD VALUE` may run inside a transaction on Postgres 12 and later as long as
the new value is not used in the same transaction; V3 only adds the value and never
references it, so both Flyway and the test helper (which applies each file with one
`Exec`) accept it. The project targets Postgres 16.

## 4. Templates and rendering

Package `gin/internal/mail` owns rendering. Templates are Go `text/template` for
`subject` and `body_text`, `html/template` for `body_html`, with this variable set only:

| Variable | Value |
|---|---|
| `.SiteName` | `MAIL_SITE_NAME` |
| `.Number`, `.Subject` | ticket number and subject |
| `.RequesterName`, `.RequesterEmail` | ticket requester |
| `.AgentName` | the acting or assigned agent's display name, or empty |
| `.Message` | the entry body: text as-is; HTML converted to text for `body_text` by stripping tags and collapsing whitespace, and sanitised (existing sanitiser rules: no script, style, forms, event handlers) for `body_html` |
| `.Link` | `<APP_BASE_URL>/tickets/<id>` |

Template keys and when they fire:

| Key | Recipient | Trigger |
|---|---|---|
| `ticket_autoresp` | requester | ticket created with a requester email, unless the ticket was opened by an auto-submitted mail |
| `ticket_reply` | requester | an agent posts a `response` entry |
| `assigned_alert` | the assignee | a ticket is assigned to an agent by someone other than that agent |
| `message_alert` | the assignee, or every active staff member with membership in the ticket's department when unassigned | a `message` entry is added by inbound mail |

Internal notes, transfers, status changes, edits, and unassignment send nothing.

`Render(key, vars) (subject, html, text string, err error)` loads the template row (cached
for 60 seconds), executes all three, and rejects a template that fails to parse or that
references an unknown variable at `PATCH` time so a broken edit is refused rather than
silently skipping mail later.

## 5. Outbound

### Enqueue

The ticket service gets a `Notifier` dependency:

```go
type Notifier interface {
    Enqueue(ctx context.Context, q *db.Queries, n Notification) error
}
type Notification struct {
    TemplateKey string
    TicketID    int64
    EntryID     *int64
    To          []Recipient      // one outbox row per recipient
    Vars        Vars
    AutoSubmitted bool           // sets Auto-Submitted: auto-replied
}
```

`mail.NewNotifier(cfg, db)` renders and inserts outbox rows using the same `*db.Queries`
(and therefore the same transaction) the ticket service is in; `mail.Disabled{}` is a
no-op used when `MAIL_ENABLED` is false and in existing ticket tests. Enqueue points:

- `ticket.Create` → `ticket_autoresp` (skipped when `CreateInput.AutoSubmitted` is true,
  a new internal flag set only by inbound processing).
- `ticket.Reply` → `ticket_reply` with the response body.
- `ticket.Assign` when the new assignee differs from the actor → `assigned_alert`.
- `ticket.AppendMessage` (new, see §6) → `message_alert`.

Message ids are `<ticket-<ticket id>-<outbox id>@<MAIL_FROM domain>>`; the outbox id is
taken from the inserted row (`RETURNING id`, then the message id is updated in the same
statement sequence). `in_reply_to` is the message id of the most recent sent outbox row for
the same ticket and recipient, if any, so mail clients thread the conversation.

Rendering happens at enqueue time. A render error is logged with the template key and ticket
id and the notification is dropped; the ticket change still commits.

### Sending

The sender loop, every `MAIL_SEND_INTERVAL`:

1. In a transaction, select up to 20 rows `WHERE status = 'pending' AND next_attempt_at <=
   now() ORDER BY next_attempt_at FOR UPDATE SKIP LOCKED`, and set `attempts = attempts + 1`.
2. For each row build an RFC 5322 message: `From` (`MAIL_FROM`), `To`, `Subject`
   (`[#<number>] <subject>`), `Date`, `Message-ID`, `In-Reply-To` and `References` when
   present, `Auto-Submitted: auto-replied` for auto-responses, `MIME-Version`,
   `Content-Type: multipart/alternative` with `text/plain` and `text/html` parts, both
   UTF-8 quoted-printable. Header values are encoded with `mime.QEncoding` where needed.
3. Send with `net/smtp`: dial with `tls.Dial` for `implicit`, `smtp.Dial` then `StartTLS`
   for `starttls`, `PLAIN` auth, one connection per batch, 30-second deadline per message.
4. Success: `status = 'sent'`, `sent_at = now()`. Failure: `last_error` set,
   `next_attempt_at = now() + backoff(attempts)` where backoff is 1, 5, 15, 60 minutes and
   then 60 minutes thereafter; after the tenth failed attempt `status = 'failed'`.
5. Commit. A worker crash between claim and commit leaves the row pending with `attempts`
   unchanged (the increment rolls back), so it is retried.

Attachments are not sent by mail in this slice; the mail carries the ticket link.

### Admin API (admin role only)

- `GET /api/v1/email/templates` → list of `{key, subject, body_html, body_text, updated_at}`.
- `PATCH /api/v1/email/templates/:key` with any of `subject`, `body_html`, `body_text`;
  each supplied body is parsed and test-rendered with sample variables; a failure returns
  400 `validation_failed` naming the field.
- `GET /api/v1/email/outbox?status=pending|sent|failed&page=&page_size=` → paged rows
  without bodies (`id, ticket_id, template_key, to_address, subject, status, attempts,
  last_error, next_attempt_at, sent_at, created_at`).
- `POST /api/v1/email/outbox/:id/retry` → sets a `failed` or `pending` row to `pending`,
  `attempts = 0`, `next_attempt_at = now()`; 409 when the row is `sent`.

Routes use the existing admin middleware and error envelope.

## 6. Inbound

### Polling

Every `MAIL_POLL_INTERVAL` the poller connects (implicit TLS, PLAIN auth), selects
`IMAP_FOLDER`, runs `SEARCH UNSEEN`, and for each UID fetches `BODY.PEEK[]` (the raw
message without setting flags), processes it, then stores `\Seen`. Transient processing
failures (database or storage errors) leave the message unseen and stop the cycle so the
next poll retries in order; permanent ones (unparseable message, missing `From`) mark it
seen and record `ignored` with the reason. A connection or login failure is logged at warn
with the host and user and retried next cycle. A cycle handles at most 100 messages.

### Processing

`mail.Inbound.Process(ctx, raw []byte) (Outcome, error)`, pure apart from its
dependencies (ticket service, storage, inbound table), in this order:

1. Parse with `go-message`: `Message-ID` (if missing, `sha256` of the raw bytes in the
   same angle-bracket form), `From` (address and display name), `Subject`, `Date`,
   `In-Reply-To`, `References`, `Auto-Submitted`, `Precedence`, `X-Auto-Response-Suppress`.
2. Dedupe: an existing `inbound_message` with the same `message_id` → return its outcome,
   do nothing.
3. Loop and bounce guard, each recorded as `ignored` with a reason: `From` address equals
   the `MAIL_FROM` address; `Auto-Submitted` present and not `no`;
   `Precedence` is `bulk`, `list`, or `junk`; `X-Auto-Response-Suppress` contains `All` or
   `AutoReply`; the `From` local part is `mailer-daemon`, `postmaster`, or `noreply` /
   `no-reply`; the `Content-Type` is `multipart/report`.
4. Body: the first `text/plain` part decoded to UTF-8 becomes the message in `text`
   format; if there is none, the first `text/html` part sanitised becomes the message in
   `html` format; if neither, the body is `(empty message)`. Attachments are the non-inline
   parts with a filename (or a `Content-Disposition: attachment`), subject to the existing
   `MAX_UPLOAD_BYTES` and `ALLOWED_MIME` limits; rejected ones are listed in the inbound
   record's `reason` and the message is still accepted.
5. Match: extract ticket ids from any `<ticket-<id>-…@<domain>>` in `In-Reply-To` and
   `References`; else a `[#<number>]` tag in the subject resolved through the ticket table.
   A match whose ticket is closed is still accepted (osTicket reopens; the API leaves the
   status unchanged and only marks it unanswered).
6. If matched and the sender's address equals the ticket's requester email
   (case-insensitive): `ticket.AppendMessage(ctx, ticketID, MessageInput{Poster,
   Body, Format, FileIDs})`, a new service method that inserts a `message` entry with
   `staff_id` NULL and `poster` = requester name, sets `is_answered = false` and
   `last_message_at`, and enqueues `message_alert`. No `ticket_event` row is added for an
   appended message (see "Events" below). Outcome `replied`.
7. Otherwise `ticket.CreateExternal(ctx, ExternalCreateInput{Subject, Body, Format,
   RequesterName, RequesterEmail, DeptID, FileIDs, AutoSubmitted})`, a new service method
   for tickets with no acting staff: department `MAIL_DEFAULT_DEPT_ID`, priority from the
   seed `normal` (topics are not guessed), source `email`, subject with leading `Re:`,
   `RE:`, `Fwd:`, `FW:` and any `[#…]` tag removed (empty → `(no subject)`), a `created`
   event with `staff` NULL and `{"via": "email"}`, and `ticket_autoresp` unless
   `AutoSubmitted`. Outcome `created`.
8. Record the inbound row with the outcome, ticket and entry ids.

Steps 6–8 run in one transaction with the ticket change, so a crash never leaves a ticket
without its inbound record (which would let the message be processed twice).

Events: appended messages do not add a `ticket_event` row (the thread entry is the record,
as with agent replies today); new tickets get the usual `created` event.

Quoted text in replies is stored as received.

## 7. Package layout

```
gin/internal/mail/
  config.go        MailConfig struct parsed from config.Config
  template.go      Render, cache, variable set, HTML→text
  notifier.go      Notifier interface, NewNotifier, Disabled
  message.go       RFC 5322 builder (headers, multipart, encoding)
  smtp.go          Sender: dial modes, auth, send one message
  sender.go        outbox loop: claim, send, mark, backoff
  imap.go          Fetcher interface over go-imap (Connect, ListUnseen, Fetch, MarkSeen)
  inbound.go       Process: parse, guards, match, create/append, record
  parse.go         MIME extraction: body parts, attachments, header decoding
  handler.go       admin routes for templates and outbox
  loops.go         RunSender / RunPoller with intervals and graceful stop
gin/internal/ticket/  CreateExternal, AppendMessage, Notifier wiring, AutoSubmitted flag
gin/internal/config/  mail keys
gin/db/migrations/V3__email.sql, gin/db/queries/mail.sql (sqlc)
gin/cmd/api/          serve wiring, mail-worker, mail-test
gin/testdata/eml/     fixture messages
```

## 8. Error handling

- Loops never exit on errors; each cycle logs and continues. Panics inside a cycle are
  recovered and logged.
- SMTP and IMAP errors are logged with host and user, never the password.
- A render failure drops that notification and logs it; the ticket change succeeds.
- The outbox `last_error` and the inbound `reason` are the operator's view; both are
  readable through the admin API (`inbound_message` gets `GET /api/v1/email/inbound?page=`
  listing the last rows, admin only).
- A message that cannot be parsed at all is recorded `ignored` with `reason = "unparseable"`
  and its raw source is not stored.

## 9. Testing

Unit (no network, no database):
- template rendering for each key, HTML escaping in `body_html`, HTML→text conversion;
- message builder: header set, `[#number]` subject, Message-ID form, threading headers,
  `Auto-Submitted`, multipart boundaries, quoted-printable UTF-8;
- backoff schedule and the ten-attempt cutoff;
- loop guard table: each rule and a normal message;
- subject cleaning and ticket-id extraction from headers and subject;
- MIME extraction from `gin/testdata/eml/`: plain, HTML-only, multipart alternative,
  multipart with a PDF attachment and an inline image, base64 UTF-8 subject, auto-reply,
  bounce (`multipart/report`), missing Message-ID.

Integration (Postgres testcontainer):
- enqueue points: create, reply, assign, append produce the expected outbox rows inside the
  same transaction, and nothing for notes or self-assignment; disabled notifier writes
  nothing;
- sender: an in-process SMTP test server (a small TCP handler speaking enough SMTP to
  accept or reject) receives the message with the right headers; a rejected send records the
  error and backoff; the tenth failure marks `failed`; retry endpoint resets it; two senders
  claiming concurrently never send the same row twice;
- inbound: processing each fixture creates or appends as specified, dedupes on a second
  pass, ignores loops, stores attachments and rejects oversized ones with a reason;
- IMAP: the poller is tested against a `Fetcher` fake; the real `go-imap` adapter is
  exercised once against a live mailbox in the final manual check, not in the suite.
- admin API handler tests for templates (validation failure on a broken template), outbox
  listing, retry conflict.

`make test` stays above 75%. Final manual check with Zoho: `api mail-test`, then a real
ticket round trip (auto-response received, agent reply received, requester reply appended,
agent alerted).

## 10. Out of scope for this slice

Per-department mailboxes, collaborators and CC handling, attachments in outbound mail,
quote stripping, per-agent notification preferences, frontend template editing, SLA and
overdue mail, digest mail, IMAP IDLE, and piping. Each is a later slice; the outbox and
inbound records carry the department and all addresses so they can be added without
changing this flow.
