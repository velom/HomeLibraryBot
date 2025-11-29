.PHONY: help test build run run-dev create-migration run-migrations migration-status clean

# Default target
help:
	@echo "Available targets:"
	@echo "  make test              - Run all tests"
	@echo "  make build             - Build all binaries"
	@echo "  make run               - Build and run the main application"
	@echo "  make run-dev           - Build and run the dev application (with testcontainers)"
	@echo "  make create-migration  - Create a new migration file (usage: make create-migration NAME=migration_name)"
	@echo "  make run-migrations    - Run pending migrations"
	@echo "  make migration-status  - Show migration status"
	@echo "  make clean             - Remove built binaries"

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Build all binaries
build:
	@echo "Building binaries..."
	go build -o bin/library cmd/library/main.go
	go build -o bin/library-dev cmd/library-dev/main.go
	go build -o bin/migrate cmd/migrate/main.go
	@echo "Binaries built successfully in ./bin/"

# Build and run the main application
run: build
	@echo "Starting library bot..."
	./bin/library

# Build and run the dev application (with testcontainers)
run-dev: build
	@echo "Starting library bot in dev mode (with testcontainers)..."
	./bin/library-dev

# Create a new migration file
# Usage: make create-migration NAME=add_users_table
create-migration:
	@if [ -z "$(NAME)" ]; then \
		echo "Error: NAME is required. Usage: make create-migration NAME=migration_name"; \
		exit 1; \
	fi
	@echo "Creating migration: $(NAME)"
	go run cmd/migrate/main.go create $(NAME)

# Run pending migrations
run-migrations:
	@echo "Running migrations..."
	go run cmd/migrate/main.go up

# Show migration status
migration-status:
	@echo "Migration status:"
	go run cmd/migrate/main.go status

# Rollback last migration
migration-down:
	@echo "Rolling back last migration..."
	go run cmd/migrate/main.go down

# Clean built binaries
clean:
	@echo "Cleaning binaries..."
	rm -rf bin/
	rm -f library library-dev migrate
	@echo "Clean complete"
