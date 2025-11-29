# GitHub Configuration

This directory contains GitHub-specific configuration files.

## Workflows

- **deploy.yml** - Automated deployment to Google Cloud Run on releases

## Documentation

- **[QUICKSTART_CI.md](QUICKSTART_CI.md)** - Quick setup guide (10 minutes)
- **[SETUP_CI.md](SETUP_CI.md)** - Detailed CI/CD setup documentation

## How It Works

1. You create a release (tag) on GitHub
2. GitHub Actions automatically:
   - Builds Docker image
   - Pushes to Google Container Registry
   - Deploys to Cloud Run
   - Configures Telegram webhook
3. Your bot is live with zero downtime!

## Required Secrets

Set these in **Settings → Secrets and variables → Actions**:

### Essential
- `GCP_PROJECT_ID` - Google Cloud project ID
- `GCP_SA_KEY` - Service account key JSON
- `TELEGRAM_BOT_TOKEN` - Bot token from @BotFather
- `ALLOWED_USER_IDS` - Comma-separated user IDs

### Optional (for external ClickHouse)
- `CLICKHOUSE_HOST`
- `CLICKHOUSE_PORT`
- `CLICKHOUSE_DATABASE`
- `CLICKHOUSE_USER`
- `CLICKHOUSE_PASSWORD`
- `CLICKHOUSE_USE_TLS`

## Getting Started

See [QUICKSTART_CI.md](QUICKSTART_CI.md) for setup instructions.
