# Bot Modes: Polling vs Webhook

The bot supports two modes of operation: **Polling** and **Webhook**. Choose based on your deployment environment.

## Quick Comparison

| Feature | Polling Mode | Webhook Mode |
|---------|-------------|--------------|
| **Best For** | Local development, VMs | Cloud Run, serverless platforms |
| **Connection** | Bot polls Telegram servers | Telegram pushes updates to bot |
| **CPU Usage** | Continuous (always checking) | On-demand (only when messages arrive) |
| **Cost on Cloud Run** | High (always running) | Low (scales to zero) |
| **Setup** | Simple (just run) | Requires public HTTPS URL |
| **Response Time** | 1-60 seconds delay | Instant |
| **Free Tier** | ❌ Not sustainable | ✅ Fully supported |

## Polling Mode (Default)

### How It Works
The bot continuously asks Telegram "any new messages?" every few seconds. Simple and reliable, but keeps the CPU busy.

### When to Use
- ✅ Local development
- ✅ Virtual machines (VPS, Compute Engine)
- ✅ Raspberry Pi or home servers
- ✅ Any environment with persistent processes
- ❌ Cloud Run (expensive - keeps container running)

### Configuration
```bash
WEBHOOK_MODE=false
# WEBHOOK_URL not needed
```

### Example - Local Development
```bash
# .env file
TELEGRAM_BOT_TOKEN=your_token
ALLOWED_USER_IDS=your_id
WEBHOOK_MODE=false
USE_MOCK_DB=true

# Run
go run ./cmd/library
# Or with Docker
docker-compose up
```

### Example - Compute Engine (Free Tier)
```bash
# Deploy to e2-micro instance
gcloud compute instances create library-bot \
    --zone=us-central1-a \
    --machine-type=e2-micro \
    --image-family=cos-stable \
    --image-project=cos-cloud

# SSH and run
gcloud compute ssh library-bot --zone=us-central1-a
docker run -e WEBHOOK_MODE=false -e TELEGRAM_BOT_TOKEN=... library-bot
```

## Webhook Mode (Cloud Run)

### How It Works
You tell Telegram your bot's URL. When a message arrives, Telegram makes an HTTPS POST request to your bot. Your bot handles the request and responds. Container can sleep when idle.

### When to Use
- ✅ Google Cloud Run
- ✅ AWS Lambda / Azure Functions
- ✅ Any serverless platform
- ✅ Cost-sensitive deployments
- ❌ Local development (no public URL)
- ❌ Environments without HTTPS

### Configuration
```bash
WEBHOOK_MODE=true
WEBHOOK_URL=https://your-service-abcdef.run.app
```

### How to Deploy

#### Step 1: Deploy to Cloud Run (without webhook first)
```bash
gcloud run deploy library-bot \
    --source . \
    --region europe-west4 \
    --allow-unauthenticated \
    --min-instances=0 \
    --set-env-vars="TELEGRAM_BOT_TOKEN=your_token" \
    --set-env-vars="ALLOWED_USER_IDS=your_id" \
    --set-env-vars="WEBHOOK_MODE=false" \
    --set-env-vars="USE_MOCK_DB=true"
```

#### Step 2: Get the service URL
```bash
SERVICE_URL=$(gcloud run services describe library-bot \
    --region europe-west4 \
    --format="value(status.url)")
echo "Service URL: $SERVICE_URL"
```

#### Step 3: Update to webhook mode
```bash
gcloud run services update library-bot \
    --region europe-west4 \
    --set-env-vars="WEBHOOK_MODE=true" \
    --set-env-vars="WEBHOOK_URL=$SERVICE_URL"
```

#### Step 4: Verify
```bash
# Check webhook is set
curl $SERVICE_URL/health
# Should return: OK

# Test bot on Telegram
# Send /start to your bot
```

### Webhook Endpoint

The bot exposes: `https://your-service.run.app/telegram-webhook`

This endpoint:
- Accepts POST requests from Telegram
- Validates and processes updates
- Returns 200 OK quickly (required by Telegram)
- Processes messages asynchronously

### Troubleshooting Webhook Mode

#### Bot not responding
```bash
# Check logs
gcloud run services logs read library-bot --region europe-west4 --limit=50

# Verify webhook is set
# Visit https://api.telegram.org/bot<YOUR_TOKEN>/getWebhookInfo
```

#### "Webhook URL not set" error
The WEBHOOK_URL environment variable must match your Cloud Run service URL exactly.

#### Telegram shows "webhook error"
- Ensure your service allows unauthenticated requests
- Check that HTTPS is working (Cloud Run provides this automatically)
- Telegram requires 2xx response within 60 seconds

## Switching Between Modes

### From Polling to Webhook
1. Set `WEBHOOK_MODE=true`
2. Set `WEBHOOK_URL=https://your-service.run.app`
3. Restart the bot
4. Bot will call Telegram API to set webhook

### From Webhook to Polling
1. Set `WEBHOOK_MODE=false`
2. Restart the bot
3. Bot will delete webhook and start polling

## Cost Comparison (Cloud Run)

### Polling Mode (min-instances=1)
```
Always running: 720 hours/month
Free tier: 180 vCPU-hours/month (25% of needed)
Estimated cost: ~$15-20/month
```

### Webhook Mode (min-instances=0)
```
Active only when messages arrive
Typical family use: ~100 messages/day = ~10 minutes CPU time
Free tier: Fully covered
Estimated cost: $0/month (within free tier)
```

## Technical Details

### Polling Mode Flow
```
Bot → [Poll Telegram] → [Get Updates] → [Process] → [Send Response]
     ↑_______________Loop every 60s_____________________________↑
```

### Webhook Mode Flow
```
User sends message → Telegram → [POST to webhook] → Bot processes → Response
                                                   ↓
                                           Container wakes up if needed
```

### Code Paths

#### Polling (internal/app/app.go)
```go
if !cfg.WebhookMode {
    bot.Start()  // Starts GetUpdatesChan loop
}
```

#### Webhook (internal/app/app.go)
```go
if cfg.WebhookMode {
    bot.StartWebhook(cfg.WebhookURL)  // Configures Telegram webhook
    // HTTP server handles incoming POST requests
}
```

## Recommendations

### For Development
Use **Polling Mode**:
- Simple setup
- No need for public URL
- Easy debugging

### For Production (Cloud)
Use **Webhook Mode** on Cloud Run:
- Cost-effective (free tier)
- Scales to zero
- Instant responses
- Better for Telegram's rate limits

### For Production (Self-Hosted)
Use **Polling Mode** on Compute Engine:
- e2-micro is free 24/7
- Simple to manage
- No webhook configuration needed

## Summary

- **Local development**: WEBHOOK_MODE=false (polling)
- **Cloud Run**: WEBHOOK_MODE=true (webhook)
- **Compute Engine/VPS**: WEBHOOK_MODE=false (polling)
- **Cost matters**: Use webhook mode
- **Simplicity matters**: Use polling mode
