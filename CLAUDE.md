# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Autonomous Workflow

**CRITICAL: Work autonomously. Do NOT ask the user for confirmation or clarification unless the situation is truly ambiguous and a wrong decision would be destructive or irreversible. Make decisions independently.**

When receiving a task, follow this workflow:

1. **Explore & understand** — Read relevant code, understand the context.
2. **Plan** — Prepare an implementation plan. Use the `superpowers:writing-plans` skill.
3. **Review the plan** — Launch a code-reviewer agent to review the plan before implementation.
4. **Implement** — Execute the plan. Use TDD where appropriate. Run `make test` to verify.
5. **Verify** — Run `make build` and `make test`. Fix any issues.
6. **Branch, commit, PR** — Create a feature branch, commit changes, and open a PR. Use the `commit-commands:commit-push-pr` skill or do it manually:
   - `git checkout -b <descriptive-branch-name>`
   - `git add <files>`
   - `git commit`
   - `git push -u origin <branch>`
   - `gh pr create`

**Rules:**
- Do NOT ask "should I proceed?" — just proceed.
- Do NOT ask "which approach?" — pick the best one and go.
- Do NOT present options — make the decision.
- Only ask the user if a decision is truly ambiguous AND the wrong choice would be hard to reverse.
- If unsure between two reasonable approaches, pick the simpler one.

## Project Overview

This is a Go project named "library" using Go 1.25.
It implements a home library management system with a Telegram bot UI, a Telegram Mini App (web interface), and ClickHouse for data storage.

### Binaries

The project includes three binaries:
1. **library** (`cmd/library/`) - Production application
2. **library-dev** (`cmd/library-dev/`) - Development mode with auto-configured ClickHouse testcontainer
3. **migrate** (`cmd/migrate/`) - Database migration tool using goose

### Common Commands

```bash
make build                           # Build all binaries to bin/
make test                            # Run all tests (go test -v ./...)
make run-dev                         # Run with auto ClickHouse testcontainer (requires Docker)
make run                             # Run production binary
make create-migration NAME=foo       # Create new goose migration
make run-migrations                  # Run pending migrations
make migration-status                # Check migration status
make migration-down                  # Rollback last migration
go test -v -run TestName ./...       # Run a specific test
go fmt ./...                         # Format code
go vet ./...                         # Static analysis
```

### Local Mini App Development

**Browser (daily development):**
1. `make run-dev` — starts bot in polling mode with ClickHouse testcontainer
2. Open `http://localhost:8081/web-app` in browser
3. A built-in shim provides a mock Telegram SDK; auth is skipped in polling mode

**Telegram (integration check):**
1. `make run-dev` in one terminal
2. `cloudflared tunnel --url http://localhost:8081` in another terminal
3. Set the tunnel HTTPS URL as Mini App URL in BotFather
4. Open Mini App in Telegram for full integration testing

### User Scenarios

1. Register a new book. Name and IsReadable flag. Labels can be added to categorize books.
2. Participants: added via the database, marked as children (is_parent=false) or parents (is_parent=true).
3. Add a "read" event: who from the family chose a book, which book, and which date (today by default).
4. Reading statistics: top books, rarely-read books filtered by label, reading streaks.
5. Reading rotation: children rotate alphabetically, parents interleave after last child.
6. Natural language queries via /ask: ask questions about reading data in plain text, powered by LLM with tool-calling.

## Database Migrations

Uses [goose](https://github.com/pressly/goose). Migration files live in `migrations/` with format `YYYYMMDDHHMMSS_description.sql`.

**CRITICAL: NEVER run migrations manually outside of tests.** Create migration files but let the user execute them.

**IMPORTANT: NEVER edit existing migration files.** Always create a new migration to modify the schema. Provide both `+goose Up` and `+goose Down` sections.

**ClickHouse-specific:** Use `ALTER TABLE ADD COLUMN` / `ALTER TABLE DROP COLUMN` for schema changes.

## Architecture

### Dual Interface

The app serves two interfaces simultaneously:
1. **Telegram Bot** - polling or webhook mode, handles commands via conversation state machine
2. **Mini App HTTP Server** - REST API at `HTTP_PORT` (default 8081) with embedded web UI, authenticated via Telegram initData hash validation

### Key Modules

- **`internal/app/`** - Application lifecycle: initializes DB, bot, HTTP server; handles graceful shutdown on SIGINT/SIGTERM
- **`internal/bot/`** - Telegram bot + HTTP server (see file breakdown below)
- **`internal/config/`** - Loads all config from environment variables via `godotenv`
- **`internal/llm/`** - LLM client for OpenAI-compatible APIs (used by /ask command)
- **`internal/storage/`** - Storage interface + ClickHouse implementation (`ch/`) + in-memory mock (`stubs/`)
- **`internal/models/`** - Domain types: Book (with Labels), Participant, Event, BookStat, RareBookStat

### Bot File Organization

| File | Purpose |
|------|---------|
| `types.go` | Bot struct, ConversationState struct |
| `constructor.go` | NewBot with notification config |
| `lifecycle.go` | Start (polling), StartWebhook, HandleWebhookUpdate |
| `handlers.go` | Top-level message/callback dispatch |
| `commands.go` | Command handlers: /start, /new_book, /read, /who_is_next, /last, /stats, /rare, /add_label, /book_labels, /books_by_label |
| `ask.go` | /ask command: LLM-powered natural language queries about reading data |
| `conversations.go` | Multi-step conversation state machine |
| `callbacks.go` | Inline keyboard callback handlers |
| `rotation.go` | ComputeNextParticipant algorithm (child/parent rotation) |
| `http.go` | Mini App HTTP server: /web-app, /api/books, /api/participants, /api/events |
| `utils.go` | Message sending utilities (supports Telegram forum threads) |

### Key Design Patterns

**Storage Interface**: All DB operations go through `storage.Storage` interface. To add new operations:
1. Add method to interface in `internal/storage/storage.go`
2. Implement in `internal/storage/ch/clickhouse.go`
3. Implement in `internal/storage/stubs/mock.go`
4. Add tests in `internal/storage/stubs/mock_test.go`

**Conversation State Machine**: Multi-step commands (e.g., /read) track state per user via `map[int64]*ConversationState` with command name, step number, and data map.

**Notification System**: When `NOTIFICATION_CHAT_ID` is configured, the bot sends a message to that chat whenever a reading event is created (from either bot or Mini App).

**Bot Modes**: Polling mode (default, for local dev) or webhook mode (set `WEBHOOK_MODE=true` with `WEBHOOK_URL`). In polling mode, Mini App auth validation is skipped for local development.

### Adding New Bot Commands

1. Add command case in `handleMessage()` in `internal/bot/handlers.go`
2. Implement handler in `internal/bot/commands.go`
3. For multi-step commands, add conversation handler in `internal/bot/conversations.go`
4. For inline keyboards, add callback handler in `internal/bot/callbacks.go`

### Configuration

Environment variables (see `.env.example`):
- `TELEGRAM_BOT_TOKEN` - Bot token from @BotFather
- `ALLOWED_USER_IDS` - Comma-separated Telegram user IDs
- `USE_MOCK_DB` - Set to "true" for in-memory testing
- `CLICKHOUSE_HOST`, `CLICKHOUSE_PORT` (9000), `CLICKHOUSE_DATABASE` (default), `CLICKHOUSE_USER` (default), `CLICKHOUSE_PASSWORD`, `CLICKHOUSE_USE_TLS` (false)
- `WEBHOOK_MODE` - Enable webhook mode (default: false)
- `WEBHOOK_URL` - Required when webhook mode is enabled
- `HTTP_PORT` - Mini App HTTP server port (default: 8081)
- `NOTIFICATION_CHAT_ID` - Chat ID for event notifications (optional, 0 = disabled)
- `NOTIFICATION_THREAD_ID` - Thread/topic ID for forum groups (optional)
- `LLM_API_KEY` - API key for OpenAI-compatible LLM (optional, enables /ask command)
- `LLM_BASE_URL` - LLM API base URL (default: Gemini API)
- `LLM_MODEL` - LLM model name (default: gemini-2.0-flash)

### ClickHouse Type Safety

**CRITICAL: Always use correct Go types when scanning ClickHouse results to avoid runtime type conversion errors.**

- `COUNT(*)` → Use `toInt32()` wrapper → Scan to `int32`
- `dateDiff()` → Returns `Int64` → **MUST scan to `int64`** (NOT `int32`)
- Date columns → Scan to `time.Time` or `*time.Time`
- When in doubt, use `int64` (safe for all integer types)

**Special ClickHouse behaviors:**
- `max(date_column)` on LEFT JOIN with no matches returns `toDateTime(0)` (epoch: 1970-01-01), NOT NULL
- Always check for epoch when expecting NULL dates: `if(max(e.date) <= toDateTime(0), -1, ...)`
- In Go code, convert epoch to `nil` pointer: `if lastReadDate.After(epoch) { lastReadPtr = &lastReadDate }`
