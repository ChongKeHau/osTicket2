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
