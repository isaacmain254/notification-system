.PHONY: run-api run-worker test vet fmt

run-api:
	go run ./cmd/api

run-worker:
	go run ./cmd/worker

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w $$(go list -f '{{ join .GoFiles " " }}' ./...)
