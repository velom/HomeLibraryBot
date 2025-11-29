package bot

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// sendMessage sends a message, handling nil api for testing
func (b *Bot) sendMessage(msg tgbotapi.MessageConfig) {
	if b.api != nil {
		b.api.Send(msg)
	}
}
