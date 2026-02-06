# GitHub Actions CI/CD Setup Guide

This guide explains how to set up automated deployment to Google Cloud Run using GitHub Actions.

## Overview

The CI/CD pipeline automatically deploys your bot to Cloud Run when you create a new release (tag) on GitHub.

**What it does:**
1. Builds Docker image on release
2. Pushes to Google Artifact Registry
3. Deploys to Cloud Run
4. Configures Telegram webhook automatically
5. Scales to zero when not in use (cost-effective!)

## Prerequisites

- Google Cloud account with billing enabled
- Google Cloud project created
- GitHub repository with admin access
- Docker Desktop installed (for local testing)

## Step 1: Set Up Google Cloud Project

### 1.1 Create a Google Cloud Project

```bash
# Set your project ID (change this!)
export PROJECT_ID="my-library-bot"

# Create project
gcloud projects create $PROJECT_ID

# Set as active project
gcloud config set project $PROJECT_ID
```

### 1.2 Enable Required APIs

```bash
# Enable necessary APIs
gcloud services enable \
  run.googleapis.com \
  artifactregistry.googleapis.com \
  cloudbuild.googleapis.com
```

### 1.3 Create Artifact Registry Repository

```bash
# Set region (must match workflow REGION)
export REGION="europe-west4"

# Create Artifact Registry repository for Docker images
gcloud artifacts repositories create cloud-run-source-deploy \
  --repository-format=docker \
  --location=$REGION \
  --description="Docker repository for library bot"

# Verify repository was created
gcloud artifacts repositories list --location=$REGION
```

### 1.4 Create a Service Account

Create a service account for GitHub Actions to use:

```bash
# Create service account
gcloud iam service-accounts create github-actions \
  --display-name="GitHub Actions Deployer"

# Get service account email
export SA_EMAIL="github-actions@${PROJECT_ID}.iam.gserviceaccount.com"

# Grant necessary permissions
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:${SA_EMAIL}" \
  --role="roles/run.admin"

gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:${SA_EMAIL}" \
  --role="roles/artifactregistry.writer"

gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:${SA_EMAIL}" \
  --role="roles/iam.serviceAccountUser"
```

### 1.5 Create and Download Service Account Key

```bash
# Create key file
gcloud iam service-accounts keys create github-actions-key.json \
  --iam-account=$SA_EMAIL

# This creates github-actions-key.json in your current directory
# ⚠️ Keep this file secure! Do not commit to git!
```

## Step 2: Configure GitHub Secrets

Go to your GitHub repository and add these secrets:

**Settings → Secrets and variables → Actions → New repository secret**

### Required Secrets

| Secret Name | Description | Example |
|-------------|-------------|---------|
| `GCP_PROJECT_ID` | Your Google Cloud project ID | `my-library-bot` |
| `GCP_SA_KEY` | Service account key JSON (entire file content) | `{"type": "service_account", ...}` |
| `TELEGRAM_BOT_TOKEN` | Bot token from @BotFather | `1234567890:ABCdefGHIjklMNOpqrsTUVwxyz` |
| `ALLOWED_USER_IDS` | Comma-separated Telegram user IDs | `123456789,987654321` |

### Optional Secrets (if using external ClickHouse)

| Secret Name | Description | Default |
|-------------|-------------|---------|
| `CLICKHOUSE_HOST` | ClickHouse server hostname | `localhost` |
| `CLICKHOUSE_PORT` | ClickHouse server port | `9000` |
| `CLICKHOUSE_DATABASE` | Database name | `default` |
| `CLICKHOUSE_USER` | Database user | `default` |
| `CLICKHOUSE_PASSWORD` | Database password | *(empty)* |
| `CLICKHOUSE_USE_TLS` | Use TLS connection | `false` |

### Optional Secrets (Notifications)

| Secret Name | Description | Default |
|-------------|-------------|---------|
| `NOTIFICATION_CHAT_ID` | Telegram chat ID for event notifications from web-app | *(disabled)* |
| `NOTIFICATION_THREAD_ID` | Thread/topic ID for forum groups | *(general chat)* |

### How to Add Secrets

1. **GCP_SA_KEY:**
   ```bash
   # Copy the entire content of github-actions-key.json
   cat github-actions-key.json | pbcopy  # macOS
   cat github-actions-key.json | xclip   # Linux
   ```
   Paste the entire JSON content into GitHub secret

2. **TELEGRAM_BOT_TOKEN:**
   - Get from [@BotFather](https://t.me/botfather)
   - Use `/newbot` command to create a bot
   - Copy the token

3. **ALLOWED_USER_IDS:**
   - Get your Telegram user ID from [@userinfobot](https://t.me/userinfobot)
   - Add multiple IDs separated by commas: `123456789,987654321`

4. **NOTIFICATION_CHAT_ID** (optional):
   - For private chats: use your user ID (same as above)
   - For groups: add the bot to the group, then use [@getidsbot](https://t.me/getidsbot) or forward a message from the group to [@userinfobot](https://t.me/userinfobot)
   - Group IDs are typically negative numbers (e.g., `-1001234567890`)
   - Leave empty to disable notifications

5. **NOTIFICATION_THREAD_ID** (optional, for forum groups with topics):
   - Only needed if your group has Topics/Forum mode enabled
   - To get the topic ID: right-click on the topic → Copy Link → the number after the last `/` is the thread ID
   - Example: `https://t.me/c/1234567890/42` → thread ID is `42`
   - Leave empty to send to the general chat (no specific topic)

## Step 3: Configure Deployment Settings

Edit `.github/workflows/deploy.yml` if you need to change:

- **Region:** Default is `europe-west4` (change `REGION` env var)
- **Service name:** Default is `library-bot` (change `SERVICE_NAME` env var)
- **Resources:** Memory (512Mi) and CPU (1) settings in `gcloud run deploy` command

## Step 4: Create Your First Release

### Option A: Using GitHub UI

1. Go to your repository on GitHub
2. Click "Releases" → "Create a new release"
3. Click "Choose a tag" → type a version (e.g., `v1.0.0`)
4. Click "Create new tag"
5. Add release title and description
6. Click "Publish release"

### Option B: Using Git Command Line

```bash
# Create and push a tag
git tag -a v1.0.0 -m "First release"
git push origin v1.0.0

# Create release on GitHub (requires gh CLI)
gh release create v1.0.0 \
  --title "v1.0.0" \
  --notes "Initial release with automated deployment"
```

## Step 5: Monitor Deployment

1. Go to **Actions** tab in your GitHub repository
2. Watch the deployment progress
3. Check the deployment summary for service URL
4. Your bot should be live at the Cloud Run URL!

## Step 6: Verify Deployment

```bash
# Check service status
gcloud run services describe library-bot --region europe-west4

# Get service URL
gcloud run services describe library-bot \
  --region europe-west4 \
  --format='value(status.url)'

# Test health endpoint
curl https://your-service-url.run.app/health
```

## Manual Deployment

You can also trigger deployment manually:

1. Go to **Actions** tab
2. Select "Deploy to Cloud Run" workflow
3. Click "Run workflow"
4. Select branch and click "Run workflow"

## Cost Optimization

The deployment is configured to be cost-effective:

- **Min instances:** 0 (scales to zero when idle)
- **Max instances:** 1 (prevents runaway costs)
- **Memory:** 512Mi (sufficient for bot)
- **CPU:** 1 (adequate performance)

**Expected costs:** FREE or ~$1-2/month for typical personal use (within Cloud Run free tier).

## Troubleshooting

### Deployment fails with permission errors

Make sure your service account has all required roles:
```bash
gcloud projects get-iam-policy $PROJECT_ID \
  --flatten="bindings[].members" \
  --filter="bindings.members:serviceAccount:github-actions@*"
```

### Webhook not working

Check that `WEBHOOK_MODE=true` is set and the webhook URL is correct:
```bash
gcloud run services describe library-bot \
  --region europe-west4 \
  --format='value(spec.template.spec.containers[0].env)'
```

### Bot not responding

1. Check logs:
   ```bash
   gcloud run services logs read library-bot --region europe-west4
   ```

2. Verify environment variables are set correctly

3. Check that bot token is valid

### Container fails to start

Check the build logs in the Actions tab and Cloud Run logs:
```bash
gcloud run services logs read library-bot \
  --region europe-west4 \
  --limit 50
```

## Security Best Practices

1. ✅ Never commit `github-actions-key.json` to git
2. ✅ Rotate service account keys periodically
3. ✅ Use least-privilege IAM roles
4. ✅ Keep secrets in GitHub Secrets (never in code)
5. ✅ Use TLS for ClickHouse connections in production
6. ✅ Regularly update dependencies

## Updating the Deployment

To deploy updates:
1. Make your code changes
2. Create a new release/tag
3. CI/CD automatically deploys the new version

Or manually trigger deployment from Actions tab.

## Rollback

If something goes wrong, roll back to previous version:

```bash
# List revisions
gcloud run revisions list \
  --service library-bot \
  --region europe-west4

# Roll back to previous revision
gcloud run services update-traffic library-bot \
  --region europe-west4 \
  --to-revisions REVISION-NAME=100
```

## Next Steps

- Set up monitoring and alerting
- Configure custom domain
- Add staging environment
- Set up automated testing before deployment
