package bot

import (
	"context"

	"github.com/go-telegram/bot/models"
	"go.uber.org/zap"
)

// Start starts the bot in polling mode
func (b *Bot) Start(ctx context.Context) error {
	b.logger.Info("Starting bot in polling mode")

	// Start the bot (blocks here)
	b.api.Start(ctx)

	return nil
}

// HandleWebhookUpdate processes a webhook update
func (b *Bot) HandleWebhookUpdate(ctx context.Context, update *models.Update) {
	b.handleUpdate(ctx, b.api, update)
}

// StartWebhook configures the bot for webhook mode
func (b *Bot) StartWebhook(webhookURL string) error {
	b.logger.Info("Configuring webhook", zap.String("url", webhookURL))
	// Note: Actual webhook setup with Telegram API would be done here
	// For now, we'll just log it as the webhook endpoint handles incoming requests
	return nil
}
