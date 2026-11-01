# Notification System

## Local PostgreSQL development

Copy `.env.example` to `.env` and adjust the development credentials if needed. Docker Compose reads `.env`; Go reads process environment variables and does not load `.env` files itself.

```powershell
Copy-Item .env.example .env
docker compose up -d postgres rabbitmq
go run ./cmd/migrate
go run ./cmd/api
```

The database listens on the configured `DB_PORT` (default `5432`), RabbitMQ on `RABBITMQ_PORT` (default `5672`), and its management UI on port `15672`. The API listens on `SERVICE_PORT` (default `5000`) and verifies the database connection at startup. Start `go run ./cmd/worker` in a second terminal to publish outbox messages and consume events. The worker retries RabbitMQ connections while the broker is unavailable.

Useful Make targets are `make db-up`, `make rabbitmq-up`, `make infra-up`, `make db-down`, `make migrate-up`, `make migrate-down`, `make sqlc`, `make test`, and `make vet`. The `migrate-down` target drops the current schema migration; use it only with disposable development data.

## SQL generation and integration tests

Install sqlc, then run `make sqlc` after changing a migration or query. Generated files are under `internal/database/sqlc` and should not be edited by hand.

Run the PostgreSQL integration test against a dedicated database after applying migrations:

```powershell
$env:TEST_DATABASE_URL = "postgres://postgres:password@localhost:5432/notification-system?sslmode=disable"
go test -tags=integration ./internal/...
```

The integration test is skipped unless `TEST_DATABASE_URL` is set. It inserts isolated rows and removes them through the application foreign key cascade.

RabbitMQ integration tests use a dedicated broker URL and verify persistent confirmed publishing, unroutable-message detection, and redelivery after a negative acknowledgement:

```powershell
$env:TEST_RABBITMQ_URL = "amqp://notification:notification-dev-password@localhost:5672/"
go test -tags=integration ./internal/messaging
```

The RabbitMQ integration test is skipped unless `TEST_RABBITMQ_URL` is set.

## Register an integrating application

Create an application from a trusted operator shell. The generated API key is printed once; store it in the integrating application's secret manager because the service persists only its SHA-256 hash.

```powershell
go run ./cmd/register-app -name "orders-service"
```

Submit an event with the returned key and an idempotency key. Repeating the same request and key returns the original event ID; reusing that key for changed content returns `409`.

```powershell
curl.exe -X POST http://localhost:5000/api/v1/events `
  -H "X-API-Key: <api-key>" `
  -H "Idempotency-Key: order-123-created" `
  -H "Content-Type: application/json" `
  -d '{"event_type":"order.created","recipient":{"external_user_id":"user-123","email":"customer@example.com"},"data":{"order_id":"ORD-123","amount":2500,"currency":"KES"}}'
```
