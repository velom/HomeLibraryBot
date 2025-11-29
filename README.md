# Home Library Telegram Bot

A Telegram bot for managing a home library and tracking family reading activities.

## Features

- **Book Management**: Register books with names, authors, and unique IDs
- **Reading Events**: Track who reads which book and when
- **Family Participants**: Manage participants (children and parents) dynamically
- **Statistics**: Determine who should read next based on reading history
- **Telegram Bot Interface**: Easy-to-use conversational interface
- **ClickHouse Backend**: Efficient data storage and querying

## Bot Commands

- `/start` - Show welcome message and available commands
- `/new_book` - Register a new book (asks for name and author)
- `/read` - Record a reading event (asks for date, book, and participant)
- `/who_is_next` - Show who should read next
- `/last` - Display the last 10 reading events

## Architecture

The application follows a clean architecture with the following components:

### Application Layer (`internal/app/`)
- Application initialization and lifecycle management
- HTTP server setup for health checks and webhooks
- Graceful shutdown handling

### Storage Layer (`internal/storage/`)
- **Interface**: Defines storage operations (`storage.go`)
- **ClickHouse Implementation**: Production database (`ch/clickhouse.go`)
- **Mock Implementation**: In-memory implementation for testing (`stubs/mock.go`)

### Bot Layer (`internal/bot/`)
- Handles Telegram bot interactions (polling and webhook modes)
- Manages conversational state for multi-step commands
- Authenticates users via allowed user IDs
- Split into logical modules: types, lifecycle, handlers, commands, conversations, callbacks

### Models (`internal/models/`)
- `Book`: ID, Name, Author, IsReadable
- `Participant`: ID, Name, IsParent
- `Event`: Date, BookName, ParticipantName

### Configuration (`internal/config/`)
- Environment variable management
- Validation and defaults

## Setup

### 1. Prerequisites

- Go 1.25 or higher
- ClickHouse database (or use mock mode for testing)
- Telegram Bot Token (from [@BotFather](https://t.me/botfather))

### 2. Configuration

Copy `.env.example` to `.env` and configure:

```bash
cp .env.example .env
```

Edit `.env` with your settings:

```bash
# Telegram Bot Token from @BotFather
TELEGRAM_BOT_TOKEN=your_bot_token_here

# Allowed Telegram User IDs (comma-separated)
ALLOWED_USER_IDS=123456789,987654321

# Use mock database for testing (true/false)
USE_MOCK_DB=false

# ClickHouse connection (required if USE_MOCK_DB=false)
CLICKHOUSE_HOST=localhost
CLICKHOUSE_PORT=9000
CLICKHOUSE_DATABASE=default
CLICKHOUSE_USER=default
CLICKHOUSE_PASSWORD=
CLICKHOUSE_USE_TLS=false
```

**Note:** The `.env` file is automatically loaded when you run the application.

### 3. Get Your Telegram User ID

Send a message to [@userinfobot](https://t.me/userinfobot) to get your Telegram user ID.

### 4. Build and Run

The application automatically loads `.env` file from the current directory.

#### Option A: With Real ClickHouse

```bash
# Install dependencies
go mod download

# Build
go build -o library ./cmd/library

# Run
./library
```

Or run directly:

```bash
go run ./cmd/library
```

#### Option B: Development Mode with Auto ClickHouse (Recommended)

For easy local development, use the dev binary that automatically starts ClickHouse in a container:

```bash
# Build dev binary
go build -o library-dev ./cmd/library-dev

# Run (starts ClickHouse automatically)
./library-dev
```

Or run directly:

```bash
go run ./cmd/library-dev
```

**Requirements:** Docker must be running for dev mode.

**What it does:**
- Starts ClickHouse in a testcontainer automatically
- Configures all connection parameters
- Cleans up the container on exit
- No manual ClickHouse setup needed!

**Note:** You still need to set `TELEGRAM_BOT_TOKEN` and `ALLOWED_USER_IDS` in your `.env` file.

**Running from GoLand:**
- Simply click the Run button - the `.env` file will be loaded automatically
- Or use `Run` → `Run 'go build ./cmd/library-dev'` for dev mode
- Or use `Run` → `Run 'go build ./cmd/library'` for production mode

## Development

### Running Tests

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run specific package tests
go test -v ./internal/storage/...

# Run with coverage
go test -cover ./...
```

### Using Mock Database

For development and testing without ClickHouse:

```bash
export USE_MOCK_DB=true
go run ./cmd/library
```

### Project Structure

```
.
├── cmd/
│   ├── library/           # Production entry point
│   │   └── main.go        # Minimal main (8 lines of code)
│   └── library-dev/       # Development entry point
│       └── main.go        # Starts ClickHouse in testcontainer
├── internal/
│   ├── app/               # Application initialization and startup
│   │   └── app.go         # App struct, New, Run, Shutdown
│   ├── bot/               # Telegram bot implementation
│   │   ├── types.go       # Bot and ConversationState types
│   │   ├── constructor.go # NewBot constructor
│   │   ├── lifecycle.go   # Start, StartWebhook, HandleWebhookUpdate
│   │   ├── handlers.go    # handleMessage, handleCallbackQuery
│   │   ├── commands.go    # Command handlers (/start, /read, etc)
│   │   ├── conversations.go # Multi-step conversation logic
│   │   ├── callbacks.go   # Inline keyboard callback handlers
│   │   └── utils.go       # Utility functions
│   ├── config/            # Configuration management
│   │   └── config.go
│   ├── storage/           # Storage layer
│   │   ├── storage.go     # Storage interface
│   │   ├── ch/            # ClickHouse implementation
│   │   │   └── clickhouse.go
│   │   └── stubs/         # Mock implementation for testing
│   │       ├── mock.go
│   │       └── mock_test.go
│   └── models/            # Data models
│       └── models.go
├── .env.example           # Example environment variables
├── CLAUDE.md             # Claude Code guidance
└── README.md             # This file
```

## Database Schema

### Books Table
```sql
CREATE TABLE books (
    id String,
    name String,
    author String,
    is_readable Bool
) ENGINE = MergeTree()
ORDER BY id
```

### Participants Table
```sql
CREATE TABLE participants (
    id String,
    name String,
    is_parent Bool
) ENGINE = MergeTree()
ORDER BY id
```

### Events Table
```sql
CREATE TABLE events (
    date DateTime,
    book_name String,
    participant_name String
) ENGINE = MergeTree()
ORDER BY date
```

## Deployment

The bot supports **two modes**: polling (for local/VMs) and webhook (for Cloud Run). See [BOT_MODES.md](BOT_MODES.md) for details.

### Automated Deployment with GitHub Actions (Recommended)

Deploy automatically to Cloud Run on every release using GitHub Actions CI/CD:

✨ **Features:**
- Automatic deployment on git tags/releases
- Zero-downtime deployments
- Automatic webhook configuration
- Scales to zero (FREE within Cloud Run free tier)
- Full deployment logs and monitoring

📚 **Setup Guides:**
- [Quick Start (10 min)](.github/QUICKSTART_CI.md) - Fast setup guide
- [Detailed Setup](.github/SETUP_CI.md) - Complete documentation

**Quick Setup:**
1. Create Google Cloud project and service account
2. Add secrets to GitHub repository
3. Create a release (tag)
4. GitHub Actions deploys automatically!

See [QUICKSTART_CI.md](.github/QUICKSTART_CI.md) for step-by-step instructions.

### Manual Deploy to Google Cloud Run (Webhook Mode)

**Webhook mode** scales to zero and stays within free tier! See [DEPLOYMENT.md](DEPLOYMENT.md) for detailed instructions.

```bash
# Step 1: Initial deploy
gcloud run deploy library-bot \
    --source . \
    --region europe-west4 \
    --min-instances=0 \
    --set-env-vars="TELEGRAM_BOT_TOKEN=your_token" \
    --set-env-vars="ALLOWED_USER_IDS=your_user_id" \
    --set-env-vars="WEBHOOK_MODE=false"

# Step 2: Get URL and enable webhook
SERVICE_URL=$(gcloud run services describe library-bot --region europe-west4 --format="value(status.url)")
gcloud run services update library-bot \
    --region europe-west4 \
    --set-env-vars="WEBHOOK_MODE=true,WEBHOOK_URL=$SERVICE_URL"
```

### Docker Deployment

#### With Docker Compose (Includes ClickHouse)

```bash
# Copy and configure .env file
cp .env.example .env
# Edit .env with your Telegram bot token and user IDs

# Start both bot and ClickHouse
docker-compose up -d

# View logs
docker-compose logs -f bot

# Stop
docker-compose down
```

#### Standalone Docker

```bash
# Build Docker image
docker build -t library-bot .

# Run with mock database
docker run -p 8080:8080 \
    -e TELEGRAM_BOT_TOKEN="your_token" \
    -e ALLOWED_USER_IDS="your_user_id" \
    -e USE_MOCK_DB="true" \
    library-bot

# Or use the helper script
./docker-run.sh
```

### Other Deployment Options

The bot can run anywhere with Docker support:

1. **Google Cloud Run**: Managed serverless containers (see DEPLOYMENT.md)
2. **Google Compute Engine**: Free e2-micro instance (744 hours/month)
3. **Local Machine**: Run directly with Go or Docker
4. **VPS**: Any Linux server (DigitalOcean, Linode, etc.)
5. **Raspberry Pi**: Perfect for home deployment

### Polling Mode Advantages

- No public HTTPS endpoint required
- No domain name needed
- Works behind firewalls and NAT
- Simpler configuration
- Health check endpoint for cloud platforms

## Reading Logic

The `/who_is_next` command implements dynamic rotation based on participants in the database:

1. **Participants are separated** into children (is_parent=false) and parents (is_parent=true)
2. **Rotation order**: Children rotate alphabetically, then a parent reads
3. **Example**: If children are Alice and Bob, and parents are Mom and Dad:
   - First: Alice
   - Next: Bob
   - Next: Mom or Dad (suggested)
   - Next: Back to Alice (restart)
4. If no events exist, starts with first child (alphabetically)
5. After any parent reads, rotation returns to first child

## Contributing

This is a personal project, but suggestions and improvements are welcome!

## License

MIT License - feel free to use and modify for your own family library.
