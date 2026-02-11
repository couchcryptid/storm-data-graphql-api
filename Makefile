.PHONY: generate build run test test-unit test-integration test-cover lint fmt vuln clean docker-up docker-down

generate:
	go generate ./...

build:
	go build -o bin/server ./cmd/server

run:
	go run ./cmd/server

test: test-unit test-integration

test-unit:
	go test ./internal/... -v -race -count=1

test-integration:
	go test ./internal/integration/... -v -race -tags=integration -count=1

test-cover:
	go test ./internal/... -coverprofile=coverage.out
	go tool cover -html=coverage.out

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .
	goimports -w .

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

clean:
	rm -rf bin/ coverage.out

docker-up:
	docker compose up -d

docker-down:
	docker compose down
