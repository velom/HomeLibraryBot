# Deployment Guide - Google Cloud Run

This guide explains how to deploy the Home Library Bot to Google Cloud Run using the free tier.

## Prerequisites

1. **Google Cloud Account**: Sign up at https://cloud.google.com/free
   - Free tier includes: 2 million requests/month, 360,000 GB-seconds/month
   - Always-free tier available

2. **Install Google Cloud SDK**:
   ```bash
   # macOS
   brew install google-cloud-sdk

   # Or download from: https://cloud.google.com/sdk/docs/install
   ```

3. **Docker** (for local testing):
   ```bash
   # macOS
   brew install docker
   ```

4. **ClickHouse Database**: You'll need a ClickHouse instance accessible from the internet
   - Options: ClickHouse Cloud, self-hosted on VM, or use mock database
   - For free tier testing, you can use `USE_MOCK_DB=true`

## Important: Bot Modes (Polling vs Webhook)

✅ **Good News**: The bot now supports **webhook mode** for Cloud Run!

### Webhook Mode (Recommended for Cloud Run)
- ✅ **Cost**: FREE within Cloud Run free tier
- ✅ **Efficiency**: Container scales to zero when idle
- ✅ **Performance**: Instant message processing
- Bot receives updates via HTTPS POST from Telegram
- Set `WEBHOOK_MODE=true` and `WEBHOOK_URL=your-service-url`

### Polling Mode (For VMs/Local Dev)
- ⚠️ **Cost**: ~$15-20/month on Cloud Run (requires min-instances=1)
- ⚠️ **Efficiency**: Always running, continuously polling
- ✅ **Simplicity**: Easier setup, no webhook configuration
- Bot polls Telegram servers every 60 seconds
- Set `WEBHOOK_MODE=false`

**See [BOT_MODES.md](BOT_MODES.md) for detailed comparison.**

### Free Tier Recommendations

For truly free hosting:
1. **Best: Cloud Run with Webhook Mode** (instructions below)
2. **Alternative: Compute Engine e2-micro** (polling mode, 744 hours/month free)
3. Local server / Raspberry Pi (polling mode)

## Step 1: Set Up Google Cloud Project

```bash
# Login to Google Cloud
gcloud auth login

# Create a new project (or use existing)
gcloud projects create library-bot-project --name="Library Bot"

# Set the project
gcloud config set project library-bot-project

# Enable required APIs
gcloud services enable run.googleapis.com
gcloud services enable cloudbuild.googleapis.com
gcloud services enable containerregistry.googleapis.com

# Set default region (us-central1 is in free tier)
gcloud config set run/region us-central1
```

## Step 2: Prepare Environment Variables

Cloud Run needs your environment variables as secrets or direct env vars.

### Option A: Using Environment Variables (Simpler, Less Secure)

```bash
# Set your variables (replace with your actual values)
export TELEGRAM_BOT_TOKEN="your_telegram_bot_token"
export ALLOWED_USER_IDS="123456789,987654321"

# For production with ClickHouse
export USE_MOCK_DB="false"
export CLICKHOUSE_HOST="your_clickhouse_host"
export CLICKHOUSE_PORT="9000"
export CLICKHOUSE_DATABASE="default"
export CLICKHOUSE_USER="default"
export CLICKHOUSE_PASSWORD="your_password"
export CLICKHOUSE_USE_TLS="true"

# For testing with mock database
export USE_MOCK_DB="true"
```

### Option B: Using Secret Manager (More Secure, Recommended)

```bash
# Enable Secret Manager API
gcloud services enable secretmanager.googleapis.com

# Create secrets
echo -n "your_telegram_bot_token" | gcloud secrets create telegram-bot-token --data-file=-
echo -n "your_clickhouse_password" | gcloud secrets create clickhouse-password --data-file=-

# Grant Cloud Run service account access to secrets
PROJECT_NUMBER=$(gcloud projects describe library-bot-project --format="value(projectNumber)")
gcloud secrets add-iam-policy-binding telegram-bot-token \
    --member="serviceAccount:${PROJECT_NUMBER}-compute@developer.gserviceaccount.com" \
    --role="roles/secretmanager.secretAccessor"
```

## Step 3: Build and Deploy

### ⭐ RECOMMENDED: Webhook Mode (Free Tier Compatible)

Deploy in two steps: first without webhook to get URL, then enable webhook.

#### Step 3.1: Initial Deploy
```bash
cd /path/to/library

# Deploy without webhook first (to get service URL)
gcloud run deploy library-bot \
    --source . \
    --region europe-west4 \
    --platform managed \
    --allow-unauthenticated \
    --min-instances=0 \
    --max-instances=1 \
    --memory=256Mi \
    --cpu=1 \
    --set-env-vars="TELEGRAM_BOT_TOKEN=your_token" \
    --set-env-vars="ALLOWED_USER_IDS=your_user_ids" \
    --set-env-vars="USE_MOCK_DB=false" \
    --set-env-vars="WEBHOOK_MODE=false"
```

#### Step 3.2: Get Service URL
```bash
SERVICE_URL=$(gcloud run services describe library-bot \
    --region europe-west4 \
    --format="value(status.url)")
echo "Your service URL: $SERVICE_URL"
```

#### Step 3.3: Enable Webhook
```bash
# Update service to use webhook mode
gcloud run services update library-bot \
    --region europe-west4 \
    --set-env-vars="WEBHOOK_MODE=true" \
    --set-env-vars="WEBHOOK_URL=$SERVICE_URL"

# Verify
curl $SERVICE_URL/health
# Should return: OK

# Check logs to see webhook setup
gcloud run services logs read library-bot --region europe-west4 --limit=20
```

Done! Bot is now running in webhook mode and will scale to zero when idle. ✅

---

### Method 1 (Alternative): Polling Mode Deploy

```bash
# Navigate to project directory
cd /path/to/library

# Deploy directly from source (Cloud Build will build the container)
gcloud run deploy library-bot \
    --source . \
    --region europe-west4 \
    --platform managed \
    --allow-unauthenticated \
    --min-instances 1 \
    --max-instances 1 \
    --memory 256Mi \
    --cpu 1 \
    --set-env-vars "TELEGRAM_BOT_TOKEN=asd" \
    --set-env-vars "ALLOWED_USER_IDS=asd" \
    --set-env-vars "CLICKHOUSE_HOST=qwe" \
    --set-env-vars "CLICKHOUSE_PORT=9000" \
    --set-env-vars "CLICKHOUSE_PASSWORD=qwerty" \
    --set-env-vars "USE_MOCK_DB=false"
```

### Method 2: Build Docker Image Manually

```bash
# Build the Docker image locally
docker build -t gcr.io/library-bot-project/library-bot:v1 .

# Test locally (optional)
docker run -p 8080:8080 \
    -e TELEGRAM_BOT_TOKEN="${TELEGRAM_BOT_TOKEN}" \
    -e ALLOWED_USER_IDS="${ALLOWED_USER_IDS}" \
    -e USE_MOCK_DB="true" \
    gcr.io/library-bot-project/library-bot:v1

# Push to Google Container Registry
gcloud auth configure-docker
docker push gcr.io/library-bot-project/library-bot:v1

# Deploy to Cloud Run
gcloud run deploy library-bot \
    --image gcr.io/library-bot-project/library-bot:v1 \
    --region us-central1 \
    --platform managed \
    --allow-unauthenticated \
    --min-instances 1 \
    --max-instances 1 \
    --memory 256Mi \
    --cpu 1 \
    --set-env-vars "TELEGRAM_BOT_TOKEN=${TELEGRAM_BOT_TOKEN}" \
    --set-env-vars "ALLOWED_USER_IDS=${ALLOWED_USER_IDS}" \
    --set-env-vars "USE_MOCK_DB=true"
```

### Method 3: Automated with Cloud Build (CI/CD)

```bash
# Submit build to Cloud Build (uses cloudbuild.yaml)
gcloud builds submit --config cloudbuild.yaml

# Note: You'll need to set secrets in Cloud Build or modify cloudbuild.yaml
```

## Step 4: Configure Environment Variables

After initial deployment, update environment variables:

```bash
# Update with ClickHouse configuration
gcloud run services update library-bot \
    --region us-central1 \
    --set-env-vars "USE_MOCK_DB=false" \
    --set-env-vars "CLICKHOUSE_HOST=your_host" \
    --set-env-vars "CLICKHOUSE_PORT=9000" \
    --set-env-vars "CLICKHOUSE_DATABASE=default" \
    --set-env-vars "CLICKHOUSE_USER=default" \
    --set-env-vars "CLICKHOUSE_PASSWORD=your_password" \
    --set-env-vars "CLICKHOUSE_USE_TLS=true"

# Or use secrets (recommended)
gcloud run services update library-bot \
    --region us-central1 \
    --set-secrets "TELEGRAM_BOT_TOKEN=telegram-bot-token:latest" \
    --set-secrets "CLICKHOUSE_PASSWORD=clickhouse-password:latest"
```

## Step 5: Verify Deployment

```bash
# Get service URL
gcloud run services describe library-bot --region us-central1 --format="value(status.url)"

# Check logs
gcloud run services logs read library-bot --region us-central1 --limit=50

# Test health endpoint
SERVICE_URL=$(gcloud run services describe library-bot --region us-central1 --format="value(status.url)")
curl ${SERVICE_URL}/health
```

## Step 6: Monitor and Manage

### View Logs
```bash
# Real-time logs
gcloud run services logs tail library-bot --region us-central1

# View in Cloud Console
# Navigate to: Cloud Run > library-bot > Logs
```

### Update Deployment
```bash
# After code changes, redeploy
gcloud run deploy library-bot --source .
```

### Scale Down (Save Costs)
```bash
# Reduce to 0 minimum instances (bot will stop polling)
gcloud run services update library-bot --min-instances 0

# Delete service entirely
gcloud run services delete library-bot --region us-central1
```

## Cost Optimization

### Free Tier Budget (Always-Free)
- **50 hours/month** of continuous operation (with 1 vCPU)
- **150 hours/month** (with 0.33 vCPU, use `--cpu-throttling`)

### Recommendations for Free Tier
1. Use minimum resources:
   ```bash
   --memory 256Mi --cpu 1
   ```

2. Enable CPU throttling (no requests between polls):
   ```bash
   --cpu-throttling  # Only allocate CPU during requests
   ```

3. Consider alternatives:
   - **Compute Engine e2-micro** (744 hours/month free in us-central1)
   - **Google Kubernetes Engine Autopilot** (small workloads)
   - **Cloud Run Jobs** (if you can schedule periodic checks)

## Troubleshooting

### Bot Not Responding
```bash
# Check if service is running
gcloud run services list

# Check logs for errors
gcloud run services logs read library-bot --limit=100

# Common issues:
# 1. Environment variables not set correctly
# 2. Telegram token invalid
# 3. User IDs not in allowed list
# 4. ClickHouse connection issues
```

### Service Keeps Restarting
```bash
# Check container logs
gcloud run services logs read library-bot --region us-central1

# Common causes:
# - Database connection failures
# - Invalid configuration
# - Memory limits exceeded
```

### High Costs
```bash
# Check current pricing
gcloud run services describe library-bot --region us-central1 --format="value(spec.template.spec.containers[0].resources)"

# Monitor usage
# Go to Cloud Console > Billing > Reports
```

## Security Best Practices

1. **Use Secret Manager** for sensitive data
2. **Restrict API access** with firewall rules
3. **Enable VPC** for ClickHouse connections
4. **Regular updates** - rebuild container monthly
5. **Monitor logs** for suspicious activity

## Alternative: Deploy to Compute Engine (Truly Free)

For 24/7 operation within free tier:

```bash
# Create e2-micro instance (free tier)
gcloud compute instances create library-bot-vm \
    --zone=us-central1-a \
    --machine-type=e2-micro \
    --image-family=cos-stable \
    --image-project=cos-cloud \
    --boot-disk-size=10GB

# SSH into instance
gcloud compute ssh library-bot-vm --zone=us-central1-a

# Install Docker and run container
docker pull gcr.io/library-bot-project/library-bot:v1
docker run -d --restart always \
    -e TELEGRAM_BOT_TOKEN="..." \
    -e ALLOWED_USER_IDS="..." \
    gcr.io/library-bot-project/library-bot:v1
```

## Support

For issues:
- Check logs: `gcloud run services logs read library-bot`
- Cloud Run docs: https://cloud.google.com/run/docs
- Pricing calculator: https://cloud.google.com/products/calculator
