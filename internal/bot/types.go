package bot

import (
	tgbotapi "github.com/matterbridge/telegram-bot-api/v6"
	"go.uber.org/zap"
	"library/internal/storage"
)

// Bot represents the Telegram bot
type Bot struct {
	api          *tgbotapi.BotAPI
	db           storage.Storage
	allowedUsers map[int64]bool
	states       map[int64]*ConversationState
	logger       *zap.Logger
}

// ConversationState tracks the state of multi-step commands
type ConversationState struct {
	Command         string
	Step            int
	Data            map[string]interface{}
	MessageThreadID int // ID of the topic/thread in Telegram groups (forum mode)
}
