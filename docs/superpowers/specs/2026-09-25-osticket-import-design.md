# osTicket MySQL import

Date: 2026-09-25
Status: approved design, pending implementation plan
Depends on: `docs/superpowers/specs/2026-09-24-go-ticket-api-design.md` (the Go API and its
PostgreSQL schema, `gin/db/migrations/V1__init.sql` and `V2__seed.sql`, both frozen).

## 1. Purpose

A one-shot command that copies an existing osTicket installation's MySQL data into the Go
API's PostgreSQL database, for the entities the API supports, so a site can switch to the new
API and React app without losing tickets, threads, attachments, staff, or configuration.

### What was agreed

- Source: a live MySQL connection (DSN plus table prefix). No dump parsing.
- Attachments: both osTicket storage backends, database chunks (`bk = 'D'`) and the
  filesystem plugin (`bk = 'F'`, read from a directory given by `--files-dir`).
- Implemented as a subcommand of the existing CLI in `gin/cmd/api`, in Go, with one new
  dependency: `github.com/go-sql-driver/mysql`.
- Target must be an empty API database (fresh migrations with only seed rows). No merging into
  a live database, no incremental sync.
- Original ids and ticket numbers are preserved.

### Assumptions

- The osTicket source is v1.18 line (the schema in this repository's
  `setup/inc/streams/core/install-mysql.sql`), with the standard ticket form fields named
  `subject` and `priority`.
- MySQL `datetime` values are wall-clock times in one zone; `--timezone` (default `UTC`) says
  which.
- osTicket staff passwords are PHP `password_hash` bcrypt strings (`$2y$…`), which Go's
  `golang.org/x/crypto/bcrypt` verifies unchanged.
- The filesystem plugin stores each file at `<uploads dir>/<key>` (flat). If a file is not at
  that path, `<uploads dir>/<first two chars of key>/<key>` is tried, then the attachment is
  skipped and reported.

### Success criteria

Against a MySQL database loaded from osTicket, `api import-osticket` completes, prints a report
with read, written, and skipped counts per entity, and afterwards: every non-deleted ticket
appears in the API with its original number, subject, status, department, priority, assignee,
requester, dates, and full thread; every referenced attachment downloads with identical bytes;
every staff member can log in with their old password; departments, topics, priorities, and
statuses match by name. `make test` stays above 75% coverage.

## 2. Command

```
api import-osticket --mysql-dsn 'user:pass@tcp(host:3306)/osticket?parseTime=true' \
    [--prefix ost_] [--files-dir DIR] [--timezone UTC] [--batch 500] [--dry-run]
```

- Target Postgres and storage come from `DATABASE_URL` and `STORAGE_DIR`, as for `serve`.
- `--prefix` defaults to `ost_`.
- `--files-dir` is required only if any referenced file has backend `F`; otherwise those
  files are reported as skipped with reason `files-dir not given`.
- `--batch` is the insert batch size, default 500, minimum 1.
- `--dry-run` reads and validates everything, prints the report, writes nothing (no Postgres
  rows, no storage files).
- Pre-flight: the command connects to both databases and refuses to run (exit code 2) unless
  the target has zero rows in `staff`, `ticket`, `thread_entry`, `file`, and `help_topic`
  other than the seed topic, and zero rows in `department` other than the seed department. It
  prints which table is non-empty.
- Exit codes: 0 success; 1 aborted on an error (report printed up to the failed step); 2
  pre-flight refused; 3 completed but with skipped tickets or attachments for reasons other
  than the `deleted` state (the report says which).

## 3. Package layout

```
gin/internal/importer/
  importer.go     Run(ctx, Options) (*Report, error): orchestrates the steps in order
  options.go      Options struct, validation, timezone loading
  report.go       Report: per-entity counters and up to 20 sample skip reasons; String()
  source.go       Source: wraps *sql.DB (MySQL), prefix-aware table names, streaming row readers
  sink.go         Sink: wraps pgx (Beginner) with batch inserts, OVERRIDING SYSTEM VALUE, sequence reset
  lookup.go       Lookup: name-matching for seed priorities/statuses/departments/topics, id maps
  priorities.go   read + map ticket_priority
  statuses.go     read + map ticket_status
  departments.go  read + map department (two passes)
  staff.go        read + map staff and staff_dept_access
  topics.go       read + map help_topic
  tickets.go      read + map ticket, user, user_email, form entries (subject, priority)
  thread.go       read + map thread and thread_entry
  files.go        read file/attachment rows; copy bytes via storage.Put
  events.go       read + map thread_event/event
  *_test.go       unit tests per mapping; importer_test.go integration test
gin/cmd/api/main.go   subcommand wiring, flags, exit codes
gin/db/testdata/osticket-mysql/  trimmed osTicket schema + fixture rows for the integration test
```

`Source` uses plain SQL through `database/sql`; sqlc is not involved for MySQL. `Sink` uses
the existing `db.Beginner` and pgx batches; it does not go through the sqlc query layer,
because the import writes columns (ids, timestamps) the API's queries never set.

## 4. Import order and transactions

Each step runs in its own Postgres transaction and commits before the next step starts.
Later steps only read the id maps built by earlier steps.

1. Priorities
2. Statuses
3. Departments, pass one (without `manager_id`)
4. Staff, then staff department memberships
5. Departments, pass two (set `manager_id` where the manager was imported)
6. Help topics
7. Tickets (with requester and form data)
8. Thread entries, then files and attachments per entry batch
9. Ticket events
10. Sequence reset: for every table with an identity column, `setval` to `max(id)`.

Ids are preserved with `INSERT … OVERRIDING SYSTEM VALUE`. Where a source row is merged into a
seed row by name (priorities, statuses, the seed department and topic), the id map points to
the seed row's id and the source id is not inserted. Ids allocated for suffix-renamed
duplicates (see departments) keep the source id.

A failure inside a step aborts that step's transaction, stops the run, and prints the report
so far with the error. Earlier steps stay committed. The README documents that re-running
requires dropping and re-migrating the target (`docker compose run flyway clean migrate`
equivalent for the user's setup).

## 5. Field mapping

Types are the Postgres enums from V1: `ticket_state` (`open`, `resolved`, `closed`),
`ticket_source` (`web`, `api`, `phone`, `other`), `thread_entry_type` (`message`, `response`,
`note`), `body_format` (`html`, `text`), `ticket_event_kind` (`created`, `assigned`,
`unassigned`, `status_changed`, `transferred`, `closed`, `reopened`, `edited`).

### Priorities (`ticket_priority`)

`priority` → `name`, `priority_urgency` → `urgency`, `priority_color` → `color`. Matched to
seed rows by case-insensitive name (seed: low, normal, high, emergency); a match maps the id
and updates nothing; otherwise inserted with the source id.

### Statuses (`ticket_status`)

`name` → `name`, `sort` → `sort_order`. State mapping: source `open` → `open`; source
`closed` → `closed`, except a status whose name is `Resolved` (case-insensitive) → `resolved`;
source `archived` → `closed`; source `deleted` → not imported (id map marks it so tickets in
that status are skipped with reason `deleted status`). Matched to seed rows (Open, Resolved,
Closed) by name.

### Departments (`department`)

`name` → `name` (trimmed), `ispublic` → `is_public`, `manager_id` → `manager_id` in pass two
if that staff id was imported, else null. `pid` (hierarchy), signatures, SLA, email, and
templates are dropped. The seed department "Support" is matched by name. Duplicate names
after trimming (case-insensitive, Postgres `name` is UNIQUE) get a numeric suffix: second
occurrence " (2)", third " (3)", and the rename is reported.

### Staff (`staff`, `staff_dept_access`)

`username` → `username`, `email` → `email`, `passwd` → `password_hash`, `firstname` →
`first_name`, `lastname` → `last_name`, `isadmin` → `is_admin`, `isactive` → `is_active`,
`dept_id` → `primary_dept_id` (if the department was not imported, the seed department, reported),
`created`/`updated` → `created_at`/`updated_at`. Empty or duplicate email (case-insensitive)
becomes `<username>@imported.invalid`, reported. Empty `passwd` (external auth backends)
becomes a fresh bcrypt hash of 32 random bytes that nobody knows, reported as `password reset
required`. Memberships: every `staff_dept_access` row whose department was imported, plus the
primary department, deduplicated.

### Help topics (`help_topic`)

`topic` → `name`, `dept_id` → `dept_id` (0 or unknown → null), `priority_id` → `priority_id`
(0 or unknown → null), `sort` → `sort_order`, active = `flags & 2` (osTicket's
`Topic::FLAG_ACTIVE = 0x0002`), `created`/`updated`. Parent topics and forms are dropped. The seed topic
"General Inquiry" is matched by name.

### Tickets (`ticket`, `user`, `user_email`, `form_entry`, `form_entry_values`, `form_field`)

- `number` → `number` (if empty, the zero-padded ticket id, reported). Duplicate numbers get
  the ticket id appended after a dash, reported.
- Subject: the form answer where `form_entry.object_type = 'T'`,
  `form_entry.object_id = ticket_id`, `form_field.name = 'subject'`, taking
  `form_entry_values.value`; empty → `(no subject)`, reported.
- Priority: same join with `form_field.name = 'priority'`, taking `value_id`; if null or
  unknown, the topic's priority, else the seed "normal".
- `status_id` → mapped; a deleted-state status skips the ticket.
- `dept_id` → mapped; unknown → the seed department, reported.
- `topic_id` → mapped or null. `staff_id` → `assigned_staff_id`, 0 or unknown → null.
- Requester: `user.name` → `requester_name`; `user_email.address` for `ticket.user_email_id`,
  falling back to the user's `default_email_id`, → `requester_email`; if no email is found,
  `unknown-<ticket_id>@imported.invalid`, reported.
- `source`: Web → `web`, Phone → `phone`, API → `api`, Email and Other → `other`.
- `isanswered` → `is_answered`, `duedate` → `due_at`, `closed` → `closed_at`, `created`,
  `updated`. `last_message_at` = `lastupdate` if set, else `created`. `last_response_at` =
  the newest imported `response` entry's `created`, set after step 8 with one UPDATE per
  ticket that has a response.
- `extra` = JSON object with the dropped columns `sla_id`, `team_id`, `flags`, `ip_address`,
  `source_extra`, `source` (the original enum text), `email_id`, and `user_id`; only non-zero,
  non-empty values are included, and the object is `{}` when none apply.

### Thread entries (`thread`, `thread_entry`)

`thread.object_type = 'T'` and `object_id` gives the ticket. Per entry: `type` M → `message`,
R → `response`, N → `note`, anything else skipped; `staff_id` → `staff_id` (0 or unknown →
null); `poster`, `title` (empty → null), `body`, `format` (`text` → `text`, else `html`),
`created`, `updated`. `pid` → `parent_id` when it names an entry of the same thread that was
imported, else null. Entries whose ticket was skipped are skipped silently (counted under the
ticket's reason).

### Files and attachments (`attachment`, `file`, `file_chunk`)

For each `attachment` row with `type = 'H'` whose `object_id` is an imported thread entry:
read the `file` row; bytes come from `file_chunk` ordered by `chunk_id` when `bk = 'D'`, or
from `--files-dir` when `bk = 'F'`; other backends are skipped with the backend letter as the
reason. Bytes stream through the API's `LocalStorage.Put` under a fresh random
32-byte hex key (the same scheme the upload service uses); `Put` returns the size and sha256,
which fill `file.size` and `file.sha256`. A size differing from the source `file.size` is
reported, and the attachment is still imported with the actual size. `file.name` = `attachment.name` if set else `file.name`; `mime` = `file.type`;
`uploaded_by` = the entry's staff id; `created_at` = `file.created`. One `file` row per source
file id (deduplicated across attachments), `attachment` rows per entry with `inline`.

### Events (`thread_event`, `event`)

Non-annulled rows whose thread maps to an imported ticket, joined to `event.name`: `created`
→ `created`; `closed` → `closed`; `reopened` → `reopened`; `assigned` → `assigned`, or
`unassigned` when the decoded data has an empty or zero staff; `transferred` → `transferred`;
`edited` → `edited`; everything else skipped (not reported individually, counted). `staff_id`
→ `staff_id` (0/unknown → null). `data`: the source `data` column parsed as JSON if it
parses to an object, else `{"raw": "<string>"}`. `timestamp` → `created_at`.

### Timestamps

All MySQL `datetime` values are interpreted in `--timezone` and stored as `timestamptz`.
Zero dates (`0000-00-00`) are treated as null where the column allows it, else the row's
`created`, else the import start time.

## 6. Report and errors

`Report` holds, per entity (priorities, statuses, departments, staff, topics, tickets, entries,
files, attachments, events): `Read`, `Written`, `Merged` (matched to seed), `Skipped`, and a
map of reason → count plus the first 20 `(source id, reason)` samples. `String()` renders a
table and the samples. The CLI prints it on every exit path that reached step 1.

Row-level problems never abort the run; they skip the row with a reason. Database and
storage errors abort as described in section 4. `--dry-run` runs the same code with a Sink
that records but does not write and a storage that discards bytes, so validation and file
readability are exercised.

## 7. Testing

- Unit tests (no databases) for every mapping function: state and source mapping, status
  name special case, department suffixing, staff email placeholders and empty password
  handling, topic active flag, ticket number fallback and dedupe, requester email fallback
  chain, thread entry type and parent resolution, event name and data decoding, zero-date
  handling, timezone interpretation.
- `Report` rendering test.
- Integration test (`importer_test.go`) using a MySQL testcontainer (`mysql:8`) loaded from
  `gin/db/testdata/osticket-mysql/schema.sql` (the relevant tables copied from
  `install-mysql.sql` with the prefix `ost_`) and `fixtures.sql` (two departments, three
  staff including one with an empty password, two topics, four tickets across open, resolved,
  closed, and deleted statuses, threads with message/response/note and a parent link, two
  attachments: one chunked in two chunks and one filesystem file written to a temp
  `--files-dir`, events including an assigned and a transferred). It imports into the Postgres
  testcontainer, then asserts the report counts, round-trips tickets and threads through the
  existing sqlc queries, downloads both attachments through `LocalStorage.Open` and compares
  bytes, verifies the imported staff password with `auth`'s bcrypt check, and checks that
  creating a new ticket afterwards gets an id above the imported ones (sequence reset).
- Pre-flight refusal test (non-empty target) and dry-run test (no rows written, report
  populated).
- Coverage stays above 75% under `make test`.

## 8. Out of scope

MySQL dump files, incremental or repeated syncs, merging into a non-empty target, users and
organizations as first-class records, custom form fields other than subject and priority,
SLAs, teams, collaborators, referrals, canned responses, knowledge base, email accounts and
templates, plugins, and osTicket versions before 1.18.
