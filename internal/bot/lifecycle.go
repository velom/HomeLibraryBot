package bot

import (
	tgbotapi "github.com/matterbridge/telegram-bot-api/v6"
	"go.uber.org/zap"
)

// Start starts the bot in polling mode
func (b *Bot) Start() error {
	b.logger.Info("Starting bot in polling mode")

	// Remove webhook (if any was set previously)
	_, err := b.api.Request(tgbotapi.DeleteWebhookConfig{})
	if err != nil {
		b.logger.Warn("Failed to delete webhook", zap.Error(err))
	}

	// Create update configuration
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	// Get updates channel
	updates := b.api.GetUpdatesChan(u)

	b.logger.Info("Bot started successfully. Waiting for updates...")

	// Handle updates (blocks here)
	b.handleUpdates(updates)
	return nil
}

// StartWebhook sets up the bot to receive updates via webhook
func (b *Bot) StartWebhook(webhookURL string) error {
	b.logger.Info("Setting up webhook", zap.String("webhook_url", webhookURL))

	// Configure webhook
	webhookConfig, _ := tgbotapi.NewWebhook(webhookURL + "/telegram-webhook")
	webhookConfig.MaxConnections = 40

	_, err := b.api.Request(webhookConfig)
	if err != nil {
		b.logger.Error("Failed to set webhook", zap.Error(err), zap.String("webhook_url", webhookURL))
		return err
	}

	// Get webhook info to verify
	info, err := b.api.GetWebhookInfo()
	if err != nil {
		b.logger.Warn("Failed to get webhook info", zap.Error(err))
	} else {
		b.logger.Info("Webhook set successfully",
			zap.String("url", info.URL),
			zap.Int("pending_updates", info.PendingUpdateCount),
		)
	}

	b.logger.Info("Bot configured for webhook mode")
	return nil
}

// HandleWebhookUpdate processes a single update from webhook
func (b *Bot) HandleWebhookUpdate(update tgbotapi.Update) {
	// Handle regular messages
	if update.Message != nil {
		userID := update.Message.From.ID
		if !b.allowedUsers[userID] {
			b.logger.Warn("Unauthorized access attempt",
				zap.Int64("user_id", userID),
				zap.String("username", update.Message.From.UserName),
				zap.String("first_name", update.Message.From.FirstName),
				zap.String("last_name", update.Message.From.LastName),
				zap.String("text", update.Message.Text),
			)
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Sorry, you are not authorized to use this bot.")
			b.sendMessage(msg)
			return
		}
		b.handleMessage(update.Message)
	}

	// Handle callback queries (inline keyboard button clicks)
	if update.CallbackQuery != nil {
		userID := update.CallbackQuery.From.ID
		if !b.allowedUsers[userID] {
			b.logger.Warn("Unauthorized callback query attempt",
				zap.Int64("user_id", userID),
				zap.String("username", update.CallbackQuery.From.UserName),
				zap.String("callback_data", update.CallbackQuery.Data),
			)
			return
		}
		b.handleCallbackQuery(update.CallbackQuery)
	}
}

// handleUpdates processes incoming updates from polling mode
func (b *Bot) handleUpdates(updates tgbotapi.UpdatesChannel) {
	for update := range updates {
		b.HandleWebhookUpdate(update)
	}
}
