.PHONY: db-up db-down run-api run-worker register-app migrate-up migrate-down sqlc test test-integration vet fmt

db-up:
	docker compose up -d postgres

db-down:
	docker compose down

run-api:
	go run ./cmd/api

run-worker:
	go run ./cmd/worker

register-app:
	go run ./cmd/register-app -name "$(name)"

migrate-up:
	go run ./cmd/migrate

migrate-down:
	go run ./cmd/migrate down

sqlc:
	sqlc generate -f sqlc.yml

test:
	go test ./...

test-integration:
	go test -tags=integration ./internal/...

vet:
	go vet ./...

fmt:
	gofmt -w $$(go list -f '{{ join .GoFiles " " }}' ./...)
