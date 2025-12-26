package bot

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-telegram/bot/models"
	"go.uber.org/zap"
)

// handleStart shows welcome message and available commands
func (b *Bot) handleStart(ctx context.Context, message *models.Message) {
	text := `Welcome to the Home Library Bot! 📚

Available commands:
/new_book - Register a new book
/read - Record a reading event
/who_is_next - See who should read next
/last - Show last 10 reading events
/stats - View reading statistics
/rare - Show rarely read books`

	b.sendMessageInThread(ctx, message.Chat.ID, text, message.MessageThreadID)
}

// handleNewBookStart initiates the new book conversation
func (b *Bot) handleNewBookStart(ctx context.Context, message *models.Message) {
	userID := message.From.ID

	b.statesMu.Lock()
	b.states[userID] = &ConversationState{
		Command:         "new_book",
		Step:            1,
		Data:            make(map[string]interface{}),
		MessageThreadID: message.MessageThreadID,
	}
	b.statesMu.Unlock()

	b.sendMessageInThread(ctx, message.Chat.ID, "Please enter the book name:", message.MessageThreadID)
}

// handleReadStart initiates the read event conversation
func (b *Bot) handleReadStart(ctx context.Context, message *models.Message) {
	userID := message.From.ID

	// Get readable books
	books, err := b.db.ListReadableBooks(ctx)
	if err != nil {
		b.logger.Error("Failed to list readable books",
			zap.Error(err),
			zap.Int64("user_id", userID),
		)
		b.sendMessageInThread(ctx, message.Chat.ID, fmt.Sprintf("Error: %v", err), message.MessageThreadID)
		return
	}

	if len(books) == 0 {
		b.logger.Info("No readable books available", zap.Int64("user_id", userID))
		b.sendMessageInThread(ctx, message.Chat.ID, "No readable books available. Please add books first with /new_book", message.MessageThreadID)
		return
	}

	b.statesMu.Lock()
	b.states[userID] = &ConversationState{
		Command:         "read",
		Step:            1,
		Data:            make(map[string]interface{}),
		MessageThreadID: message.MessageThreadID,
	}
	b.statesMu.Unlock()

	// Show date selection with inline keyboard
	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "📆 Today", CallbackData: "date:today"},
				{Text: "⏮ Yesterday", CallbackData: "date:yesterday"},
			},
			{
				{Text: "⏮⏮ 2 days ago", CallbackData: "date:2daysago"},
				{Text: "⏮⏮⏮ 3 days ago", CallbackData: "date:3daysago"},
			},
			{
				{Text: "📝 Custom date", CallbackData: "date:custom"},
			},
		},
	}
	b.sendMessageInThreadWithMarkup(ctx, message.Chat.ID, "📅 Select reading date:", message.MessageThreadID, keyboard)
}

// handleWhoIsNext shows who should read next based on rotation logic
func (b *Bot) handleWhoIsNext(ctx context.Context, message *models.Message) {
	// Get all participants
	participants, err := b.db.ListParticipants(ctx)
	if err != nil {
		b.logger.Error("Failed to list participants",
			zap.Error(err),
			zap.Int64("user_id", message.From.ID),
		)
		b.sendMessageInThread(ctx, message.Chat.ID, fmt.Sprintf("Error: %v", err), message.MessageThreadID)
		return
	}

	if len(participants) == 0 {
		b.logger.Warn("No participants found", zap.Int64("user_id", message.From.ID))
		b.sendMessageInThread(ctx, message.Chat.ID, "No participants found in database", message.MessageThreadID)
		return
	}

	// Get the last event to determine last reader
	events, err := b.db.GetLastEvents(ctx, 1)
	if err != nil {
		b.logger.Error("Failed to get last events",
			zap.Error(err),
			zap.Int64("user_id", message.From.ID),
		)
		b.sendMessageInThread(ctx, message.Chat.ID, fmt.Sprintf("Error: %v", err), message.MessageThreadID)
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
		b.sendMessageInThread(ctx, message.Chat.ID, "No child participants found in database", message.MessageThreadID)
		return
	}

	text := fmt.Sprintf("Next to read: %s", nextReader)
	b.sendMessageInThread(ctx, message.Chat.ID, text, message.MessageThreadID)
}

// handleLast shows the last 10 reading events
func (b *Bot) handleLast(ctx context.Context, message *models.Message) {
	events, err := b.db.GetLastEvents(ctx, 10)
	if err != nil {
		b.logger.Error("Failed to get last events",
			zap.Error(err),
			zap.Int64("user_id", message.From.ID),
		)
		b.sendMessageInThread(ctx, message.Chat.ID, fmt.Sprintf("Error: %v", err), message.MessageThreadID)
		return
	}

	if len(events) == 0 {
		b.logger.Info("No reading events found", zap.Int64("user_id", message.From.ID))
		b.sendMessageInThread(ctx, message.Chat.ID, "No reading events recorded yet.", message.MessageThreadID)
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

	b.sendMessageInThread(ctx, message.Chat.ID, text.String(), message.MessageThreadID)
}

// handleStatsStart initiates the statistics conversation
func (b *Bot) handleStatsStart(ctx context.Context, message *models.Message) {
	userID := message.From.ID

	b.statesMu.Lock()
	b.states[userID] = &ConversationState{
		Command:         "stats",
		Step:            1,
		Data:            make(map[string]interface{}),
		MessageThreadID: message.MessageThreadID,
	}
	b.statesMu.Unlock()

	// Show time period selection
	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "📅 Specific month", CallbackData: "stats_period:month"},
				{Text: "📅 Calendar year", CallbackData: "stats_period:year"},
			},
			{
				{Text: "⏮ Last 2 months", CallbackData: "stats_period:last2"},
				{Text: "⏮ Last 3 months", CallbackData: "stats_period:last3"},
			},
			{
				{Text: "⏮ Last 6 months", CallbackData: "stats_period:last6"},
				{Text: "⏮ Last 12 months", CallbackData: "stats_period:last12"},
			},
		},
	}
	b.sendMessageInThreadWithMarkup(ctx, message.Chat.ID, "📊 Select time period for statistics:", message.MessageThreadID, keyboard)
}

// handleRare shows rarely read books in two categories: by children and by all participants
func (b *Bot) handleRare(ctx context.Context, message *models.Message) {
	const limit = 10

	// Get rarely read books by children
	childrenStats, err := b.db.GetRarelyReadBooks(ctx, limit, true)
	if err != nil {
		b.logger.Error("Failed to get rarely read books by children",
			zap.Error(err),
			zap.Int64("user_id", message.From.ID),
		)
		b.sendMessageInThread(ctx, message.Chat.ID, fmt.Sprintf("Error: %v", err), message.MessageThreadID)
		return
	}

	// Get rarely read books by all participants
	allStats, err := b.db.GetRarelyReadBooks(ctx, limit, false)
	if err != nil {
		b.logger.Error("Failed to get rarely read books by all",
			zap.Error(err),
			zap.Int64("user_id", message.From.ID),
		)
		b.sendMessageInThread(ctx, message.Chat.ID, fmt.Sprintf("Error: %v", err), message.MessageThreadID)
		return
	}

	var text strings.Builder
	text.WriteString("📚 Rarely read books:\n\n")

	// Children's perspective
	text.WriteString("👶 By children's choice:\n")
	if len(childrenStats) == 0 {
		text.WriteString("No data available\n")
	} else {
		for i, stat := range childrenStats {
			text.WriteString(fmt.Sprintf("%d. %s", i+1, stat.BookName))
			if stat.DaysSinceLastRead == -1 {
				text.WriteString(" (never read)")
			} else {
				lastReadStr := stat.LastReadDate.Format("2006-01-02")
				text.WriteString(fmt.Sprintf(" (%d days ago, last: %s)", stat.DaysSinceLastRead, lastReadStr))
			}
			text.WriteString("\n")
		}
	}

	text.WriteString("\n📖 Overall (all participants):\n")
	if len(allStats) == 0 {
		text.WriteString("No data available\n")
	} else {
		for i, stat := range allStats {
			text.WriteString(fmt.Sprintf("%d. %s", i+1, stat.BookName))
			if stat.DaysSinceLastRead == -1 {
				text.WriteString(" (never read)")
			} else {
				lastReadStr := stat.LastReadDate.Format("2006-01-02")
				text.WriteString(fmt.Sprintf(" (%d days ago, last: %s)", stat.DaysSinceLastRead, lastReadStr))
			}
			text.WriteString("\n")
		}
	}

	b.sendMessageInThread(ctx, message.Chat.ID, text.String(), message.MessageThreadID)
}
