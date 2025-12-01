package bot

import (
	"fmt"

	tgbotapi "github.com/matterbridge/telegram-bot-api/v6"
	"go.uber.org/zap"
	"library/internal/storage"
)

// NewBot creates a new Telegram bot
func NewBot(token string, db storage.Storage, allowedUserIDs []int64, logger *zap.Logger) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		logger.Error("Failed to create bot API", zap.Error(err))
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}

	allowedUsers := make(map[int64]bool)
	for _, id := range allowedUserIDs {
		allowedUsers[id] = true
	}

	logger.Info("Bot created", zap.String("bot_username", api.Self.UserName))

	return &Bot{
		api:          api,
		db:           db,
		allowedUsers: allowedUsers,
		states:       make(map[int64]*ConversationState),
		logger:       logger,
	}, nil
}

// GetAPI returns the bot API for testing
func (b *Bot) GetAPI() *tgbotapi.BotAPI {
	return b.api
}
