package bot

import (
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Start starts the bot in polling mode
func (b *Bot) Start() error {
	log.Println("Starting bot in polling mode...")

	// Remove webhook (if any was set previously)
	_, err := b.api.Request(tgbotapi.DeleteWebhookConfig{})
	if err != nil {
		log.Printf("Warning: failed to delete webhook: %v", err)
	}

	// Create update configuration
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	// Get updates channel
	updates := b.api.GetUpdatesChan(u)

	log.Printf("Bot started successfully. Waiting for updates...")

	// Handle updates (blocks here)
	b.handleUpdates(updates)
	return nil
}

// StartWebhook sets up the bot to receive updates via webhook
func (b *Bot) StartWebhook(webhookURL string) error {
	log.Printf("Setting up webhook at: %s", webhookURL)

	// Configure webhook
	webhookConfig, _ := tgbotapi.NewWebhook(webhookURL + "/telegram-webhook")
	webhookConfig.MaxConnections = 40

	_, err := b.api.Request(webhookConfig)
	if err != nil {
		return err
	}

	// Get webhook info to verify
	info, err := b.api.GetWebhookInfo()
	if err != nil {
		log.Printf("Warning: failed to get webhook info: %v", err)
	} else {
		log.Printf("Webhook set successfully: %s", info.URL)
		log.Printf("Pending updates: %d", info.PendingUpdateCount)
	}

	log.Println("Bot configured for webhook mode")
	return nil
}

// HandleWebhookUpdate processes a single update from webhook
func (b *Bot) HandleWebhookUpdate(update tgbotapi.Update) {
	// Handle regular messages
	if update.Message != nil {
		userID := update.Message.From.ID
		if !b.allowedUsers[userID] {
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
