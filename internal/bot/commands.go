package bot

import (
	"context"
	"fmt"
	"strings"

	tgbotapi "github.com/matterbridge/telegram-bot-api/v6"
	"go.uber.org/zap"
)

// handleStart shows welcome message and available commands
func (b *Bot) handleStart(message *tgbotapi.Message) {
	text := `Welcome to the Home Library Bot! 📚

Available commands:
/new_book - Register a new book
/read - Record a reading event
/who_is_next - See who should read next
/last - Show last 10 reading events
/stats - View reading statistics`

	b.sendMessageInThread(message.Chat.ID, text, message.MessageThreadID)
}

// handleNewBookStart initiates the new book conversation
func (b *Bot) handleNewBookStart(message *tgbotapi.Message) {
	userID := message.From.ID
	b.states[userID] = &ConversationState{
		Command:         "new_book",
		Step:            1,
		Data:            make(map[string]interface{}),
		MessageThreadID: message.MessageThreadID,
	}

	b.sendMessageInThread(message.Chat.ID, "Please enter the book name:", message.MessageThreadID)
}

// handleReadStart initiates the read event conversation
func (b *Bot) handleReadStart(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID

	// Get readable books
	books, err := b.db.ListReadableBooks(ctx)
	if err != nil {
		b.logger.Error("Failed to list readable books",
			zap.Error(err),
			zap.Int64("user_id", userID),
		)
		b.sendMessageInThread(message.Chat.ID, fmt.Sprintf("Error: %v", err), message.MessageThreadID)
		return
	}

	if len(books) == 0 {
		b.logger.Info("No readable books available", zap.Int64("user_id", userID))
		b.sendMessageInThread(message.Chat.ID, "No readable books available. Please add books first with /new_book", message.MessageThreadID)
		return
	}

	b.states[userID] = &ConversationState{
		Command:         "read",
		Step:            1,
		Data:            make(map[string]interface{}),
		MessageThreadID: message.MessageThreadID,
	}

	// Show date selection with inline keyboard
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📆 Today", "date:today"),
			tgbotapi.NewInlineKeyboardButtonData("⏮ Yesterday", "date:yesterday"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⏮⏮ 2 days ago", "date:2daysago"),
			tgbotapi.NewInlineKeyboardButtonData("⏮⏮⏮ 3 days ago", "date:3daysago"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📝 Custom date", "date:custom"),
		),
	)
	b.sendMessageInThreadWithMarkup(message.Chat.ID, "📅 Select reading date:", message.MessageThreadID, keyboard)
}

// handleWhoIsNext shows who should read next based on rotation logic
func (b *Bot) handleWhoIsNext(ctx context.Context, message *tgbotapi.Message) {
	// Get all participants
	participants, err := b.db.ListParticipants(ctx)
	if err != nil {
		b.logger.Error("Failed to list participants",
			zap.Error(err),
			zap.Int64("user_id", message.From.ID),
		)
		b.sendMessageInThread(message.Chat.ID, fmt.Sprintf("Error: %v", err), message.MessageThreadID)
		return
	}

	if len(participants) == 0 {
		b.logger.Warn("No participants found", zap.Int64("user_id", message.From.ID))
		b.sendMessageInThread(message.Chat.ID, "No participants found in database", message.MessageThreadID)
		return
	}

	// Get the last event to determine last reader
	events, err := b.db.GetLastEvents(ctx, 1)
	if err != nil {
		b.logger.Error("Failed to get last events",
			zap.Error(err),
			zap.Int64("user_id", message.From.ID),
		)
		b.sendMessageInThread(message.Chat.ID, fmt.Sprintf("Error: %v", err), message.MessageThreadID)
		return
	}

	// Determine last participant name
	var lastParticipant string
	if len(events) > 0 {
		lastParticipant = events[0].ParticipantName
	}

	// Compute next participant using rotation algorithm
	nextReader := ComputeNextParticipant(participants, lastParticipant)

	if nextReader == "" {
		b.sendMessageInThread(message.Chat.ID, "No child participants found in database", message.MessageThreadID)
		return
	}

	text := fmt.Sprintf("Next to read: %s", nextReader)
	b.sendMessageInThread(message.Chat.ID, text, message.MessageThreadID)
}

// handleLast shows the last 10 reading events
func (b *Bot) handleLast(ctx context.Context, message *tgbotapi.Message) {
	events, err := b.db.GetLastEvents(ctx, 10)
	if err != nil {
		b.logger.Error("Failed to get last events",
			zap.Error(err),
			zap.Int64("user_id", message.From.ID),
		)
		b.sendMessageInThread(message.Chat.ID, fmt.Sprintf("Error: %v", err), message.MessageThreadID)
		return
	}

	if len(events) == 0 {
		b.logger.Info("No reading events found", zap.Int64("user_id", message.From.ID))
		b.sendMessageInThread(message.Chat.ID, "No reading events recorded yet.", message.MessageThreadID)
		return
	}

	b.logger.Info("Retrieved last events",
		zap.Int("event_count", len(events)),
		zap.Int64("user_id", message.From.ID),
	)

	var text strings.Builder
	text.WriteString("Last reading events:\n\n")
	for i, event := range events {
		text.WriteString(fmt.Sprintf("%d. %s - %s (%s)\n",
			i+1,
			event.Date.Format("2006-01-02"),
			event.BookName,
			event.ParticipantName))
	}

	b.sendMessageInThread(message.Chat.ID, text.String(), message.MessageThreadID)
}

// handleStatsStart initiates the statistics conversation
func (b *Bot) handleStatsStart(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID
	b.states[userID] = &ConversationState{
		Command:         "stats",
		Step:            1,
		Data:            make(map[string]interface{}),
		MessageThreadID: message.MessageThreadID,
	}

	// Show time period selection
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📅 Specific month", "stats_period:month"),
			tgbotapi.NewInlineKeyboardButtonData("📅 Calendar year", "stats_period:year"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⏮ Last 2 months", "stats_period:last2"),
			tgbotapi.NewInlineKeyboardButtonData("⏮ Last 3 months", "stats_period:last3"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⏮ Last 6 months", "stats_period:last6"),
			tgbotapi.NewInlineKeyboardButtonData("⏮ Last 12 months", "stats_period:last12"),
		),
	)
	b.sendMessageInThreadWithMarkup(message.Chat.ID, "📊 Select time period for statistics:", message.MessageThreadID, keyboard)
}
