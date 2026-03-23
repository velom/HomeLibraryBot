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
/rare - Show rarely read books
/add_label - Add a label to a book
/book_labels - Show labels for a book
/books_by_label - Show books by label`

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

// handleRareStart starts the rare books command with label selection
func (b *Bot) handleRareStart(ctx context.Context, message *models.Message) {
	userID := message.From.ID

	// Get all available labels
	labels, err := b.db.GetAllLabels(ctx)
	if err != nil {
		b.logger.Error("Failed to get labels",
			zap.Error(err),
			zap.Int64("user_id", userID),
		)
		b.sendMessageInThread(ctx, message.Chat.ID, fmt.Sprintf("Error: %v", err), message.MessageThreadID)
		return
	}

	// Set conversation state
	b.statesMu.Lock()
	b.states[userID] = &ConversationState{
		Command:         "rare",
		Step:            1,
		Data:            make(map[string]interface{}),
		MessageThreadID: message.MessageThreadID,
	}
	b.statesMu.Unlock()

	// Build keyboard with "All books" option and available labels
	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "📚 All books", CallbackData: "rare_label:"},
			},
		},
	}

	// Add labels in two columns
	for i := 0; i < len(labels); i += 2 {
		row := []models.InlineKeyboardButton{
			{Text: labels[i], CallbackData: fmt.Sprintf("rare_label:%s", labels[i])},
		}
		if i+1 < len(labels) {
			row = append(row, models.InlineKeyboardButton{Text: labels[i+1], CallbackData: fmt.Sprintf("rare_label:%s", labels[i+1])})
		}
		keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, row)
	}

	b.sendMessageInThreadWithMarkup(ctx, message.Chat.ID, "🏷 Filter by label:", message.MessageThreadID, keyboard)
}

// handleBookLabelsStart starts the book labels query command
func (b *Bot) handleBookLabelsStart(ctx context.Context, message *models.Message) {
	userID := message.From.ID

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
		b.sendMessageInThread(ctx, message.Chat.ID, "No readable books available.", message.MessageThreadID)
		return
	}

	b.statesMu.Lock()
	b.states[userID] = &ConversationState{
		Command:         "book_labels",
		Step:            1,
		Data:            make(map[string]interface{}),
		MessageThreadID: message.MessageThreadID,
	}
	b.statesMu.Unlock()

	// Build inline keyboard with books in 2 columns
	var rows [][]models.InlineKeyboardButton
	var currentRow []models.InlineKeyboardButton
	for i, book := range books {
		button := models.InlineKeyboardButton{
			Text:         book.Name,
			CallbackData: fmt.Sprintf("booklabels:%d", i),
		}
		currentRow = append(currentRow, button)

		if len(currentRow) == 2 || i == len(books)-1 {
			rows = append(rows, currentRow)
			currentRow = []models.InlineKeyboardButton{}
		}
	}

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: rows,
	}
	b.sendMessageInThreadWithMarkup(ctx, message.Chat.ID, "📚 Select a book to see its labels:", message.MessageThreadID, keyboard)
}

// handleBooksByLabelStart starts the books by label query command
func (b *Bot) handleBooksByLabelStart(ctx context.Context, message *models.Message) {
	userID := message.From.ID

	labels, err := b.db.GetAllLabels(ctx)
	if err != nil {
		b.logger.Error("Failed to get labels",
			zap.Error(err),
			zap.Int64("user_id", userID),
		)
		b.sendMessageInThread(ctx, message.Chat.ID, fmt.Sprintf("Error: %v", err), message.MessageThreadID)
		return
	}

	if len(labels) == 0 {
		b.sendMessageInThread(ctx, message.Chat.ID, "No labels found. Use /add_label to add labels to books.", message.MessageThreadID)
		return
	}

	b.statesMu.Lock()
	b.states[userID] = &ConversationState{
		Command:         "books_by_label",
		Step:            1,
		Data:            make(map[string]interface{}),
		MessageThreadID: message.MessageThreadID,
	}
	b.statesMu.Unlock()

	// Build inline keyboard with labels in 2 columns
	var rows [][]models.InlineKeyboardButton
	var currentRow []models.InlineKeyboardButton
	for i, label := range labels {
		button := models.InlineKeyboardButton{
			Text:         label,
			CallbackData: fmt.Sprintf("booksbylabel:%s", label),
		}
		currentRow = append(currentRow, button)

		if len(currentRow) == 2 || i == len(labels)-1 {
			rows = append(rows, currentRow)
			currentRow = []models.InlineKeyboardButton{}
		}
	}

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: rows,
	}
	b.sendMessageInThreadWithMarkup(ctx, message.Chat.ID, "🏷 Select a label to see its books:", message.MessageThreadID, keyboard)
}

// handleAddLabelStart starts the add label command
func (b *Bot) handleAddLabelStart(ctx context.Context, message *models.Message) {
	userID := message.From.ID

	// Set conversation state
	b.statesMu.Lock()
	b.states[userID] = &ConversationState{
		Command:         "add_label",
		Step:            1,
		Data:            make(map[string]interface{}),
		MessageThreadID: message.MessageThreadID,
	}
	b.statesMu.Unlock()

	b.sendMessageInThread(ctx, message.Chat.ID, "🏷 Which label?", message.MessageThreadID)
}
