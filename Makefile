-include .env
export

.PHONY: run test fmt vet tidy db-up migrate-up migrate-down dev dev-down dev-logs dev-migrate dev-setup

run:
	go run ./cmd/api

test:
	go test ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

tidy:
	go mod tidy

db-up:
	docker compose up -d db

migrate-up:
	docker compose run --rm migrate

migrate-down:
	docker compose run --rm migrate -path=/migrations -database="postgres://$(DB_USER):$(DB_PASSWORD)@db:5432/$(DB_NAME)?sslmode=disable" down 1

dev:
	docker compose -f docker-compose.dev.yml up --build

dev-setup:
	python3 scripts/setup_minio.py

dev-down:
	docker compose -f docker-compose.dev.yml down

dev-logs:
	docker compose -f docker-compose.dev.yml logs -f app

dev-migrate:
	docker compose -f docker-compose.dev.yml run --rm migrate
