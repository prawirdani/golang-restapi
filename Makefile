# Load .env variables
ifneq (,$(wildcard ./.env))
    include .env
    export
endif


# Run the api server
dev:
	@air -c .air.toml

# Run the message consumers
dev\:worker:
	@air -c .air.worker.toml

tidy:
	@go mod tidy

lint:	
	@golangci-lint run

test:
	@go test -race -v -count=1 ./... -cover

build:
	@echo "Building binary..."
	CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -a -installsuffix cgo -o ./bin/api ./cmd/api/
	@echo "Build completed successfully..."

run:
	./bin/api

# Makesure you have goose binary installed
migration\:status:
	@goose -dir migrations postgres "host=$(DB_HOST) port=$(DB_PORT) user=$(DB_USER) password=$(DB_PASSWORD) dbname=$(DB_NAME) sslmode=disable" status

migration\:up:
	@goose -dir migrations postgres "host=$(DB_HOST) port=$(DB_PORT) user=$(DB_USER) password=$(DB_PASSWORD) dbname=$(DB_NAME) sslmode=disable" up

migration\:down:
	@goose -dir migrations postgres "host=$(DB_HOST) port=$(DB_PORT) user=$(DB_USER) password=$(DB_PASSWORD) dbname=$(DB_NAME) sslmode=disable" down

migration\:clear:
	@goose -dir migrations postgres "host=$(DB_HOST) port=$(DB_PORT) user=$(DB_USER) password=$(DB_PASSWORD) dbname=$(DB_NAME) sslmode=disable" down-to 0

migration\:create:
	@echo "Create Migration"
	@read -p "Enter migration name: " migration_name; \
	goose -s -dir migrations create $$migration_name sql
	@echo "Migration created successfully, fill in the schema in the generated file."
