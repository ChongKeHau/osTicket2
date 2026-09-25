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
`--files-dir` (needed for files stored by the filesystem plugin; database-stored files need
nothing), `--batch` (rows per insert, default 500), `--dry-run` (validate and report only).

Ids and ticket numbers are preserved. The report lists, per entity, rows read, written,
merged into seed rows, and skipped with reasons. Exit codes: 0 done; 1 aborted (earlier
steps stay committed: drop and re-migrate the target before retrying); 2 target not empty;
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

## Commands

    make test               # full suite with coverage gate (> 75%)
    make sqlc               # regenerate internal/db from db/queries
    go run ./cmd/api gc-files --older-than 24h

## Layout

`cmd/api` entrypoint; `internal/<feature>` packages each with `handler.go` and
`service.go`; `internal/db` is sqlc output plus pool and transaction helpers;
`db/migrations` is owned by Flyway.

The React frontend lives in ../app (see its README).
