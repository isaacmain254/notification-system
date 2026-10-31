# Notification System

## Local PostgreSQL development

Copy `.env.example` to `.env` and adjust the development credentials if needed. Docker Compose reads `.env`; Go reads process environment variables and does not load `.env` files itself.

```powershell
Copy-Item .env.example .env
docker compose up -d postgres
go run ./cmd/migrate
go run ./cmd/api
```

The database listens on the configured `DB_PORT` (default `5432`). The API listens on `SERVICE_PORT` (default `5000`) and verifies the database connection at startup.

Useful Make targets are `make db-up`, `make db-down`, `make migrate-up`, `make migrate-down`, `make sqlc`, `make test`, and `make vet`. The `migrate-down` target drops the current schema migration; use it only with disposable development data.

## SQL generation and integration tests

Install sqlc, then run `make sqlc` after changing a migration or query. Generated files are under `internal/database/sqlc` and should not be edited by hand.

Run the PostgreSQL integration test against a dedicated database after applying migrations:

```powershell
$env:TEST_DATABASE_URL = "postgres://postgres:password@localhost:5432/notification-system?sslmode=disable"
go test -tags=integration ./internal/database/...
```

The integration test is skipped unless `TEST_DATABASE_URL` is set. It inserts isolated rows and removes them through the application foreign key cascade.
