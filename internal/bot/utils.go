package bot

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// sendMessageInThread sends a message to a specific thread/topic in a group
func (b *Bot) sendMessageInThread(ctx context.Context, chatID int64, text string, messageThreadID int) {
	if b.api == nil {
		return // For testing
	}

	params := &bot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	}
	if messageThreadID != 0 {
		params.MessageThreadID = messageThreadID
	}

	b.api.SendMessage(ctx, params)
}

// sendMessageInThreadWithMarkup sends a message with markup to a specific thread/topic in a group
func (b *Bot) sendMessageInThreadWithMarkup(ctx context.Context, chatID int64, text string, messageThreadID int, markup models.ReplyMarkup) {
	if b.api == nil {
		return // For testing
	}

	params := &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        text,
		ReplyMarkup: markup,
	}
	if messageThreadID != 0 {
		params.MessageThreadID = messageThreadID
	}

	b.api.SendMessage(ctx, params)
}
