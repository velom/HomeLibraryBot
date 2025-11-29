package bot

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"library/internal/storage"
)

// NewBot creates a new Telegram bot
func NewBot(token string, db storage.Storage, allowedUserIDs []int64) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}

	allowedUsers := make(map[int64]bool)
	for _, id := range allowedUserIDs {
		allowedUsers[id] = true
	}

	return &Bot{
		api:          api,
		db:           db,
		allowedUsers: allowedUsers,
		states:       make(map[int64]*ConversationState),
	}, nil
}

// GetAPI returns the bot API for testing
func (b *Bot) GetAPI() *tgbotapi.BotAPI {
	return b.api
}
