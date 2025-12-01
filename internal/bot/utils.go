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

// sendReply sends a message as a reply to another message, keeping it in the same thread
func (b *Bot) sendReply(chatID int64, text string, replyToMessageID int) {
	msg := tgbotapi.NewMessage(chatID, text)
	if replyToMessageID != 0 {
		msg.ReplyToMessageID = replyToMessageID
	}
	b.sendMessage(msg)
}

// sendReplyWithMarkup sends a message with a reply markup as a reply to another message
func (b *Bot) sendReplyWithMarkup(chatID int64, text string, replyToMessageID int, markup interface{}) {
	msg := tgbotapi.NewMessage(chatID, text)
	if replyToMessageID != 0 {
		msg.ReplyToMessageID = replyToMessageID
	}
	msg.ReplyMarkup = markup
	b.sendMessage(msg)
}
