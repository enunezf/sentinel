.PHONY: build run test lint migrate docker-up docker-down keys

build:
	go build -o bin/auth-service ./cmd/server/

run:
	go run ./cmd/server/

test:
	go test ./... -v -cover

lint:
	golangci-lint run ./...

migrate:
	go run ./cmd/migrate/

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down

keys:
	mkdir -p keys
	openssl genrsa -out keys/private.pem 2048
	openssl rsa -in keys/private.pem -pubout -out keys/public.pem
