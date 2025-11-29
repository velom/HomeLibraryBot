#!/bin/bash

# Simple script to build and run the Docker container locally
# Usage: ./docker-run.sh

set -e

echo "Building Docker image..."
docker build -t library-bot:local .

echo ""
echo "Starting container..."
echo "Make sure you have set environment variables or use .env file"
echo ""

# Check if .env exists
if [ -f .env ]; then
    echo "Loading environment from .env file..."
    docker run -p 8080:8080 --env-file .env library-bot:local
else
    echo "No .env file found. Using system environment variables..."
    docker run -p 8080:8080 \
        -e TELEGRAM_BOT_TOKEN="${TELEGRAM_BOT_TOKEN}" \
        -e ALLOWED_USER_IDS="${ALLOWED_USER_IDS}" \
        -e USE_MOCK_DB="${USE_MOCK_DB:-true}" \
        -e CLICKHOUSE_HOST="${CLICKHOUSE_HOST}" \
        -e CLICKHOUSE_PORT="${CLICKHOUSE_PORT}" \
        -e CLICKHOUSE_DATABASE="${CLICKHOUSE_DATABASE}" \
        -e CLICKHOUSE_USER="${CLICKHOUSE_USER}" \
        -e CLICKHOUSE_PASSWORD="${CLICKHOUSE_PASSWORD}" \
        -e CLICKHOUSE_USE_TLS="${CLICKHOUSE_USE_TLS}" \
        library-bot:local
fi
