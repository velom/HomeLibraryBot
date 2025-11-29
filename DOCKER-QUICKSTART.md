# Docker Quick Start Guide

## Local Development with Docker Compose

The easiest way to run the bot locally with ClickHouse:

```bash
# 1. Copy environment template
cp .env.example .env

# 2. Edit .env and add your Telegram bot token
# Get token from @BotFather on Telegram
# Get your user ID from @userinfobot

# 3. Start everything
docker-compose up -d

# 4. Check logs
docker-compose logs -f bot

# 5. Test the bot
# Send /start to your bot on Telegram

# 6. Stop when done
docker-compose down
```

## Testing Without ClickHouse (Mock Database)

```bash
# Build image
docker build -t library-bot .

# Run with mock database (no ClickHouse needed)
docker run -p 8080:8080 \
    -e TELEGRAM_BOT_TOKEN="your_bot_token" \
    -e ALLOWED_USER_IDS="your_user_id" \
    -e USE_MOCK_DB="true" \
    library-bot

# Or use the helper script
./docker-run.sh
```

## Health Check

Once running, check the health endpoint:

```bash
curl http://localhost:8080/health
# Should return: OK

curl http://localhost:8080/
# Should return: Home Library Bot is running
```

## Troubleshooting

### Bot not responding
- Check logs: `docker-compose logs bot`
- Verify TELEGRAM_BOT_TOKEN is correct
- Verify your user ID is in ALLOWED_USER_IDS

### ClickHouse connection errors
- Check ClickHouse is running: `docker-compose ps`
- Check ClickHouse logs: `docker-compose logs clickhouse`
- Try restarting: `docker-compose restart`

### Container won't start
- Check environment variables: `docker-compose config`
- View build logs: `docker-compose build --no-cache`

## Files Overview

- **Dockerfile**: Container image definition
- **docker-compose.yaml**: Local development stack (bot + ClickHouse)
- **docker-run.sh**: Helper script for standalone Docker
- **.dockerignore**: Files to exclude from Docker build
- **.gcloudignore**: Files to exclude from Cloud deployments
- **cloudbuild.yaml**: Google Cloud Build configuration
- **DEPLOYMENT.md**: Full Cloud Run deployment guide

## Next Steps

- For production deployment: See [DEPLOYMENT.md](DEPLOYMENT.md)
- For development: See [README.md](README.md)
