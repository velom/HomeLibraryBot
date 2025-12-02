package bot

import (
	"context"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"go.uber.org/zap"
)

// handleMessage processes a single message
func (b *Bot) handleMessage(ctx context.Context, message *models.Message) {
	// Recover from panics to prevent bot crashes
	defer func() {
		if r := recover(); r != nil {
			b.logger.Error("Recovered from panic in handleMessage",
				zap.Any("panic", r),
				zap.Int64("user_id", message.From.ID),
				zap.Int64("chat_id", message.Chat.ID),
				zap.String("text", message.Text),
			)
			b.sendMessageInThread(ctx, message.Chat.ID, "An error occurred while processing your request. Please try again.", message.MessageThreadID)
		}
	}()

	userID := message.From.ID

	isCommand := len(message.Entities) > 0 && message.Entities[0].Type == models.MessageEntityTypeBotCommand && message.Entities[0].Offset == 0

	b.logger.Debug("Received message",
		zap.Int64("user_id", userID),
		zap.Int64("chat_id", message.Chat.ID),
		zap.String("text", message.Text),
		zap.Bool("is_command", isCommand),
	)

	// Check if user is in a conversation
	b.statesMu.RLock()
	state, hasState := b.states[userID]
	b.statesMu.RUnlock()

	if hasState {
		// If conversation is already complete (Step == -1), clean it up and process as new command
		if state.Step == -1 {
			b.statesMu.Lock()
			delete(b.states, userID)
			b.statesMu.Unlock()
		} else if isCommand {
			// Allow any command to interrupt/cancel an ongoing conversation
			b.statesMu.Lock()
			delete(b.states, userID)
			b.statesMu.Unlock()
			// Continue to process the new command below
		} else {
			// Not a command, continue the conversation
			b.handleConversation(ctx, message, state)
			return
		}
	}

	// Handle commands
	if isCommand {
		// Extract command from text
		cmdText := message.Text
		if len(cmdText) > 0 && cmdText[0] == '/' {
			cmdText = cmdText[1:] // Remove the leading '/'
			// Remove bot username if present
			if idx := strings.Index(cmdText, "@"); idx != -1 {
				cmdText = cmdText[:idx]
			}
			// Remove arguments if present
			if idx := strings.Index(cmdText, " "); idx != -1 {
				cmdText = cmdText[:idx]
			}
		}

		b.logger.Info("Processing command",
			zap.String("command", cmdText),
			zap.Int64("user_id", userID),
			zap.Int64("chat_id", message.Chat.ID),
		)

		switch cmdText {
		case "start":
			b.handleStart(ctx, message)
		case "new_book":
			b.handleNewBookStart(ctx, message)
		case "read":
			b.handleReadStart(ctx, message)
		case "who_is_next":
			b.handleWhoIsNext(ctx, message)
		case "last":
			b.handleLast(ctx, message)
		case "stats":
			b.handleStatsStart(ctx, message)
		default:
			b.logger.Warn("Unknown command",
				zap.String("command", cmdText),
				zap.Int64("user_id", userID),
			)
			b.sendMessageInThread(ctx, message.Chat.ID, "Unknown command. Use /start to see available commands.", message.MessageThreadID)
		}
	}
}

// handleCallbackQuery processes inline keyboard button clicks
func (b *Bot) handleCallbackQuery(ctx context.Context, query *models.CallbackQuery) {
	// Recover from panics
	defer func() {
		if r := recover(); r != nil {
			b.logger.Error("Recovered from panic in handleCallbackQuery",
				zap.Any("panic", r),
				zap.Int64("user_id", query.From.ID),
				zap.String("callback_data", query.Data),
			)
		}
	}()

	userID := query.From.ID

	b.logger.Debug("Received callback query",
		zap.Int64("user_id", userID),
		zap.String("callback_data", query.Data),
	)

	// Answer the callback query to remove loading state
	b.api.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{
		CallbackQueryID: query.ID,
	})

	// Check if user is in a conversation
	b.statesMu.RLock()
	state, ok := b.states[userID]
	b.statesMu.RUnlock()

	if !ok {
		b.logger.Debug("No conversation state for callback",
			zap.Int64("user_id", userID),
			zap.String("callback_data", query.Data),
		)
		return
	}

	// Handle callback based on prefix
	data := query.Data
	if strings.HasPrefix(data, "date:") {
		b.handleDateCallback(ctx, query, state)
	} else if strings.HasPrefix(data, "book:") {
		b.handleBookCallback(ctx, query, state)
	} else if strings.HasPrefix(data, "participant:") {
		b.handleParticipantCallback(ctx, query, state)
	} else if strings.HasPrefix(data, "stats_period:") {
		b.handleStatsPeriodCallback(ctx, query, state)
	} else if strings.HasPrefix(data, "stats_participant:") {
		b.handleStatsParticipantCallback(ctx, query, state)
	} else {
		b.logger.Warn("Unknown callback prefix",
			zap.String("callback_data", data),
			zap.Int64("user_id", userID),
		)
	}

	// Clean up completed conversations
	if state.Step == -1 {
		b.statesMu.Lock()
		delete(b.states, userID)
		b.statesMu.Unlock()
		b.logger.Debug("Conversation completed", zap.Int64("user_id", userID))
	}
}
