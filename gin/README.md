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

If port 5432 is already in use on your machine, set `POSTGRES_PORT` (e.g.
`export POSTGRES_PORT=5433`) before `make migrate`/`make db-up`, and point
`DATABASE_URL` at that port instead.

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
