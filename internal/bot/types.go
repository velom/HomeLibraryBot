package bot

import (
	"sync"

	"github.com/go-telegram/bot"
	"go.uber.org/zap"
	"library/internal/storage"
)

// Bot represents the Telegram bot wrapper
type Bot struct {
	api          *bot.Bot
	db           storage.Storage
	allowedUsers map[int64]bool
	states       map[int64]*ConversationState
	statesMu     sync.RWMutex
	logger       *zap.Logger
	httpServer   *HTTPServer
}

// ConversationState tracks the state of multi-step commands
type ConversationState struct {
	Command         string
	Step            int
	Data            map[string]interface{}
	MessageThreadID int // ID of the topic/thread in Telegram groups (forum mode)
}
