package bot

import (
	tgbotapi "github.com/matterbridge/telegram-bot-api/v6"
)

// sendMessage sends a message, handling nil api for testing
func (b *Bot) sendMessage(msg tgbotapi.MessageConfig) {
	if b.api != nil {
		b.api.Send(msg)
	}
}

// sendMessageInThread sends a message to a specific thread/topic in a group
func (b *Bot) sendMessageInThread(chatID int64, text string, messageThreadID int) {
	msg := tgbotapi.NewMessage(chatID, text)
	if messageThreadID != 0 {
		msg.MessageThreadID = messageThreadID
	}
	b.sendMessage(msg)
}

// sendMessageInThreadWithMarkup sends a message with markup to a specific thread/topic in a group
func (b *Bot) sendMessageInThreadWithMarkup(chatID int64, text string, messageThreadID int, markup interface{}) {
	msg := tgbotapi.NewMessage(chatID, text)
	if messageThreadID != 0 {
		msg.MessageThreadID = messageThreadID
	}
	msg.ReplyMarkup = markup
	b.sendMessage(msg)
}
