package bot

import (
	"context"
	"fmt"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"go.uber.org/zap"
	"library/internal/storage"
)

// NewBot creates a new Telegram bot
func NewBot(token string, db storage.Storage, allowedUserIDs []int64, httpPort int, logger *zap.Logger) (*Bot, error) {
	allowedUsers := make(map[int64]bool)
	for _, id := range allowedUserIDs {
		allowedUsers[id] = true
	}

	// Create bot wrapper first (without API)
	botWrapper := &Bot{
		db:           db,
		allowedUsers: allowedUsers,
		states:       make(map[int64]*ConversationState),
		logger:       logger,
	}

	// Create bot with handlers
	opts := []bot.Option{
		bot.WithDefaultHandler(botWrapper.handleUpdate),
	}

	api, err := bot.New(token, opts...)
	if err != nil {
		logger.Error("Failed to create bot API", zap.Error(err))
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}

	botWrapper.api = api

	// Create HTTP server for Mini App
	botWrapper.httpServer = NewHTTPServer(botWrapper, httpPort)

	// Get bot info
	me, err := api.GetMe(context.Background())
	if err == nil {
		logger.Info("Bot created", zap.String("bot_username", me.Username))
	} else {
		logger.Warn("Could not get bot info", zap.Error(err))
	}

	return botWrapper, nil
}

// handleUpdate is the main update handler
func (b *Bot) handleUpdate(ctx context.Context, api *bot.Bot, update *models.Update) {
	// Handle message updates
	if update.Message != nil {
		b.handleMessage(ctx, update.Message)
		return
	}

	// Handle callback query updates
	if update.CallbackQuery != nil {
		b.handleCallbackQuery(ctx, update.CallbackQuery)
		return
	}
}

// GetAPI returns the bot API for testing
func (b *Bot) GetAPI() *bot.Bot {
	return b.api
}
