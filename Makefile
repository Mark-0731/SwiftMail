.PHONY: dev build test lint migrate-up migrate-down docker-up docker-down api worker smtp clean

# ===== Development =====

dev: docker-up
	@echo "Starting SwiftMail services..."

docker-up:
	docker-compose up -d

docker-down:
	docker-compose down

docker-reset:
	docker-compose down -v
	docker-compose up -d

# ===== Build =====

build: build-api build-worker build-smtp

build-api:
	go build -o bin/api ./cmd/api

build-worker:
	go build -o bin/worker ./cmd/worker

build-smtp:
	go build -o bin/smtp ./cmd/smtp

build-migrate:
	go build -o bin/migrate ./cmd/migrate

# ===== Run =====

api:
	go run ./cmd/api

worker:
	go run ./cmd/worker

smtp:
	go run ./cmd/smtp

# ===== Database =====

migrate-up:
	go run ./cmd/migrate -direction=up

migrate-down:
	go run ./cmd/migrate -direction=down

migrate-create:
	@read -p "Migration name: " name; \
	migrate create -ext sql -dir migrations -seq $$name

# ===== Test =====

test:
	go test -v -race ./...

test-cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# ===== Lint =====

lint:
	golangci-lint run ./...

# ===== Clean =====

clean:
	rm -rf bin/
	rm -f coverage.out coverage.html
