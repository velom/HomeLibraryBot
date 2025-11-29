# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go project named "library" using Go 1.25.
It implements a home library management system with a Telegram bot UI and ClickHouse for data storage.

### Binaries

The project includes three binaries:
1. **library** (`cmd/library/`) - Production application
2. **library-dev** (`cmd/library-dev/`) - Development mode with auto-configured ClickHouse testcontainer
3. **migrate** (`cmd/migrate/`) - Database migration tool using goose

### Makefile

A Makefile is provided with common development tasks:
- `make build` - Build all binaries
- `make run` / `make run-dev` - Run the application
- `make test` - Run tests
- `make run-migrations` - Run database migrations
- `make create-migration NAME=...` - Create new migration
- `make help` - Show all available commands

### User scenarios

1. Register a new book. Name, Author and uniq auto-generated ID. Additional flag: is available to read.
2. Participants: Users can add participants dynamically via the database, marking them as children (is_parent=false) or parents (is_parent=true). 
3. Add a "read" event: who from the family chose a book, which book (name) and which date (today by default).
4. Compute and show some statistic. Detailed reports will be described later.

### Technical details

#### Storage

Main implementation is built over ClickHouse. Storage layer (`internal/storage/`) contains an Interface, ClickHouse implementation (`ch/`), and mocks (`stubs/`).
Most of tests should run over mocks, implementation tests should run over testcontainer with clickhouse.

Three tables: 
1. Books (ID, Name, Author, IsReadable)
2. Participants (ID, Name, IsParent)
3. Events (Date, BookName, ParticipantName)

#### Telegram Bot

Only configured accounts must be able to communicate with the bot.
A few commands:
1. /new_book - register new book. Ask a Name and an Author
2. /read - create and event. Ask a date (suggested: today), ask a book name (should pickable from the list of books with IsReadable flag), ask a Participant (from the list).
3. /who_is_next - identify next Participant.
4. /last - show 10 last events

Bot works in polling mode (bot pulls updates from Telegram)

## Database Migrations

The project uses [goose](https://github.com/pressly/goose) for database schema migrations.

### Migration Files

- Location: `migrations/` directory
- Naming: `YYYYMMDDHHMMSS_description.sql`
- Format: SQL with goose directives (`+goose Up` and `+goose Down`)

### Migration Commands

```bash
# Run all pending migrations
make run-migrations

# Check migration status
make migration-status

# Create a new migration
make create-migration NAME=add_column_to_books

# Rollback the last migration
make migration-down
```

### Migration Binary

The `cmd/migrate` binary reads database credentials from `.env` file and runs migrations.

## Building and Running

### Using Makefile (Recommended)

```bash
# Show all available commands
make help

# Build all binaries
make build

# Run the application
make run

# Run in dev mode (with auto ClickHouse)
make run-dev

# Run tests
make test

# Clean built binaries
make clean
```

### Production Mode

```bash
# Run the application
go run ./cmd/library

# Build the binary
go build -o library ./cmd/library

# Run the built binary
./library
```

### Development Mode (with auto ClickHouse)

For local development, use the dev binary that automatically manages ClickHouse in a container:

```bash
# Run directly (recommended for development)
go run ./cmd/library-dev

# Or use Makefile
make run-dev
```

**Requirements:** Docker must be running.

**What it does:**
- Automatically starts ClickHouse in a testcontainer
- Configures connection parameters
- Cleans up on exit
- No manual ClickHouse setup needed!

## Testing

```bash
# Run all tests
make test

# Or use go test directly
go test ./...

# Run tests with verbose output
go test -v ./...

# Run a specific test
go test -v -run TestName ./...

# Run tests with coverage
go test -cover ./...
```

## Dependencies

```bash
# Add a new dependency
go get <package>

# Update dependencies
go get -u ./...

# Tidy up go.mod and go.sum
go mod tidy

# Verify dependencies
go mod verify
```

## Code Quality

```bash
# Format code
go fmt ./...

# Run linter (requires golangci-lint)
golangci-lint run

# Run static analysis
go vet ./...
```

## Architecture

The application follows a clean architecture pattern with clear separation of concerns:

### Project Structure
```
.
├── cmd/
│   ├── library/              # Production entry point
│   │   └── main.go           # Minimal main (8 lines of code)
│   ├── library-dev/          # Development entry point
│   │   └── main.go           # Starts ClickHouse in testcontainer
│   └── migrate/              # Database migration tool
│       └── main.go           # Runs goose migrations (reads .env)
├── migrations/               # SQL migration files
│   └── 20250101000000_initial_schema.sql
├── internal/
│   ├── app/                  # Application initialization and lifecycle
│   │   └── app.go           # App struct, New, Run, Shutdown
│   ├── bot/                  # Telegram bot implementation (split by logic)
│   │   ├── types.go         # Bot and ConversationState types
│   │   ├── constructor.go   # NewBot constructor
│   │   ├── lifecycle.go     # Start, StartWebhook, HandleWebhookUpdate
│   │   ├── handlers.go      # handleMessage, handleCallbackQuery
│   │   ├── commands.go      # Command handlers (/start, /read, etc)
│   │   ├── conversations.go # Multi-step conversation logic
│   │   ├── callbacks.go     # Inline keyboard callback handlers
│   │   └── utils.go         # Utility functions
│   ├── config/              # Configuration management
│   │   └── config.go        # Environment variable loading
│   ├── storage/             # Storage layer (interface + implementations)
│   │   ├── storage.go       # Storage interface definition
│   │   ├── ch/              # ClickHouse implementation
│   │   │   └── clickhouse.go
│   │   └── stubs/           # In-memory mock for testing
│   │       ├── mock.go
│   │       └── mock_test.go
│   └── models/              # Domain models
│       └── models.go        # Book, Participant, Event
└── .env.example             # Environment configuration template
```

### Key Design Patterns

**Dependency Injection**: The bot receives a `storage.Storage` interface, allowing for easy testing and swapping implementations.

**Conversation State**: Multi-step bot commands maintain state in memory using a `map[int64]*ConversationState` structure.

**Environment-Based Configuration**: All configuration is loaded from environment variables via `internal/config`.

### Configuration

Required environment variables (see `.env.example`):
- `TELEGRAM_BOT_TOKEN` - Bot token from @BotFather
- `ALLOWED_USER_IDS` - Comma-separated Telegram user IDs
- `USE_MOCK_DB` - Set to "true" for in-memory testing
- `CLICKHOUSE_HOST` - ClickHouse server hostname
- `CLICKHOUSE_PORT` - ClickHouse port (default: 9000)
- `CLICKHOUSE_DATABASE` - Database name (default: "default")
- `CLICKHOUSE_USER` - Username (default: "default")
- `CLICKHOUSE_PASSWORD` - Password (optional)
- `CLICKHOUSE_USE_TLS` - Enable TLS encryption (default: false)

### Adding New Bot Commands

1. Add command case in `handleMessage()` in `internal/bot/handlers.go`
2. Implement handler function in `internal/bot/commands.go` (e.g., `handleCommandName()`)
3. For multi-step commands, add conversation handler in `internal/bot/conversations.go`
4. For inline keyboards, add callback handler in `internal/bot/callbacks.go`

### Storage Operations

All storage operations go through the `storage.Storage` interface. To add new operations:
1. Add method to interface in `internal/storage/storage.go`
2. Implement in `internal/storage/ch/clickhouse.go`
3. Implement in `internal/storage/stubs/mock.go`
4. Add tests in `internal/storage/stubs/mock_test.go`