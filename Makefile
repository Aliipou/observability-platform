.PHONY: run test up down build lint

run:
	go run ./cmd/server

test:
	go test ./... -race -count=1

build:
	CGO_ENABLED=0 go build -o bin/server ./cmd/server

up:
	docker compose -f deployments/docker-compose.yml up -d

down:
	docker compose -f deployments/docker-compose.yml down

lint:
	golangci-lint run ./...
