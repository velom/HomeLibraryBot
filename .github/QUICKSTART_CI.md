# Quick Start: GitHub Actions + Cloud Run

Deploy your bot to Google Cloud Run automatically with every release.

## TL;DR Setup (10 minutes)

### 1. Google Cloud Setup

```bash
# Set your project ID
export PROJECT_ID="my-library-bot"

# Create project and enable APIs
gcloud projects create $PROJECT_ID
gcloud config set project $PROJECT_ID
gcloud services enable run.googleapis.com containerregistry.googleapis.com

# Create service account
gcloud iam service-accounts create github-actions \
  --display-name="GitHub Actions"

export SA_EMAIL="github-actions@${PROJECT_ID}.iam.gserviceaccount.com"

# Grant permissions
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:${SA_EMAIL}" \
  --role="roles/run.admin"

gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:${SA_EMAIL}" \
  --role="roles/storage.admin"

gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:${SA_EMAIL}" \
  --role="roles/iam.serviceAccountUser"

# Download key
gcloud iam service-accounts keys create github-actions-key.json \
  --iam-account=$SA_EMAIL
```

### 2. GitHub Secrets

Go to **Settings → Secrets and variables → Actions** and add:

| Secret | Where to get it | Example |
|--------|----------------|---------|
| `GCP_PROJECT_ID` | Your project ID | `my-library-bot` |
| `GCP_SA_KEY` | Content of `github-actions-key.json` | `{"type": "service_account"...}` |
| `TELEGRAM_BOT_TOKEN` | [@BotFather](https://t.me/botfather) | `123456:ABCdef...` |
| `ALLOWED_USER_IDS` | [@userinfobot](https://t.me/userinfobot) | `123456789` |

### 3. Create Release

```bash
# Create and push tag
git tag -a v1.0.0 -m "First release"
git push origin v1.0.0

# Or use GitHub UI: Releases → Create new release
```

### 4. Watch Magic Happen

1. Go to **Actions** tab
2. Watch deployment
3. Bot deploys automatically to Cloud Run
4. Done! 🎉

## Using Mock Database

If you don't have ClickHouse, the bot can use mock storage:

Just **don't add** the ClickHouse secrets (`CLICKHOUSE_*`), and the workflow will deploy with mock storage.

## Using External ClickHouse

If you have ClickHouse (Aiven, ClickHouse Cloud, self-hosted), add these secrets:

- `CLICKHOUSE_HOST`
- `CLICKHOUSE_PORT`
- `CLICKHOUSE_DATABASE`
- `CLICKHOUSE_USER`
- `CLICKHOUSE_PASSWORD`
- `CLICKHOUSE_USE_TLS`

## Cost

**FREE** for personal use (within Cloud Run free tier):
- 2 million requests/month free
- Scales to zero when idle
- Only pay for actual usage

Expected: **$0-2/month** for typical bot usage.

## Troubleshooting

**Deployment failed?**
- Check Actions logs tab
- Verify all secrets are set correctly
- Make sure billing is enabled on GCP

**Bot not responding?**
```bash
# Check logs
gcloud run services logs read library-bot --region europe-west4
```

**Need help?** See [SETUP_CI.md](.github/SETUP_CI.md) for detailed guide.

## What Gets Deployed

- ✅ Docker container with your bot
- ✅ Cloud Run service (auto-scaling)
- ✅ Webhook configured automatically
- ✅ HTTPS endpoint
- ✅ Automatic rollback on failures

## Manual Deploy

Trigger deployment anytime from **Actions → Deploy to Cloud Run → Run workflow**
