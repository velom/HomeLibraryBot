package bot

import (
	"sync"

	"github.com/go-telegram/bot"
	"go.uber.org/zap"
	"library/internal/llm"
	"library/internal/storage"
)

// Bot represents the Telegram bot wrapper
type Bot struct {
	api                  *bot.Bot
	db                   storage.Storage
	allowedUsers         map[int64]bool
	states               map[int64]*ConversationState
	statesMu             sync.RWMutex
	logger               *zap.Logger
	notificationChatID   int64 // Chat ID to send notifications when events are created via web-app (0 = disabled)
	notificationThreadID int   // Thread/topic ID for forum groups (0 = general/no topic)
	llmClient            *llm.Client
	pendingBooks         map[string]*pendingBook // books submitted via Mini App awaiting Telegram approval
	pendingBooksMu       sync.Mutex
	pendingBookSeq       uint64 // monotonic source for pending-book ids
}

// pendingBook is a book submitted via the Mini App awaiting Telegram approval.
type pendingBook struct {
	Name        string
	Labels      []string
	RequestedBy string
}

// ConversationState tracks the state of multi-step commands
type ConversationState struct {
	Command         string
	Step            int
	Data            map[string]interface{}
	MessageThreadID int // ID of the topic/thread in Telegram groups (forum mode)
}
