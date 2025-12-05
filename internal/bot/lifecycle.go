package bot

import (
	"context"
	"net/http"

	"github.com/go-telegram/bot/models"
	"go.uber.org/zap"
)

// Start starts the bot in polling mode
func (b *Bot) Start(ctx context.Context) error {
	b.logger.Info("Starting bot in polling mode")

	// Start HTTP server in a goroutine
	go func() {
		if err := b.httpServer.Start(); err != nil && err != http.ErrServerClosed {
			b.logger.Error("HTTP server error", zap.Error(err))
		}
	}()

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

	// Start HTTP server in a goroutine
	go func() {
		if err := b.httpServer.Start(); err != nil && err != http.ErrServerClosed {
			b.logger.Error("HTTP server error", zap.Error(err))
		}
	}()

	// Note: Actual webhook setup with Telegram API would be done here
	// For now, we'll just log it as the webhook endpoint handles incoming requests
	return nil
}

// Shutdown gracefully shuts down the bot and HTTP server
func (b *Bot) Shutdown(ctx context.Context) error {
	b.logger.Info("Shutting down bot")

	// Shutdown HTTP server
	if b.httpServer != nil {
		if err := b.httpServer.Shutdown(ctx); err != nil {
			b.logger.Error("Failed to shutdown HTTP server", zap.Error(err))
			return err
		}
	}

	return nil
}
