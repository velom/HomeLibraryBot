package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	libmodels "library/internal/models"

	"github.com/go-telegram/bot/models"
	"go.uber.org/zap"
)

// getChatIDFromQuery extracts chat ID from callback query message
func getChatIDFromQuery(query *models.CallbackQuery) int64 {
	if query.Message.Message != nil {
		return query.Message.Message.Chat.ID
	}
	return 0
}

// handleDateCallback processes date selection from inline keyboard
func (b *Bot) handleDateCallback(ctx context.Context, query *models.CallbackQuery, state *ConversationState) {
	data := strings.TrimPrefix(query.Data, "date:")

	// Handle custom date option
	if data == "custom" {
		state.Data["awaiting_custom_date"] = true
		b.sendMessageInThread(ctx, getChatIDFromQuery(query), "📝 Please enter the date in format YYYY-MM-DD\n\nExample: 2024-01-15", state.MessageThreadID)
		return
	}

	var date time.Time
	switch data {
	case "today":
		date = time.Now()
	case "yesterday":
		date = time.Now().AddDate(0, 0, -1)
	case "2daysago":
		date = time.Now().AddDate(0, 0, -2)
	case "3daysago":
		date = time.Now().AddDate(0, 0, -3)
	default:
		return
	}

	state.Data["date"] = date
	state.Step = 2

	// Get books and show selection
	books, err := b.db.ListReadableBooks(ctx)
	if err != nil {
		b.logger.Error("Failed to list readable books in date callback",
			zap.Error(err),
			zap.Int64("user_id", query.From.ID),
		)
		b.sendMessageInThread(ctx, getChatIDFromQuery(query), fmt.Sprintf("Error: %v", err), state.MessageThreadID)
		state.Step = -1
		return
	}

	// Create inline keyboard for book selection (2 columns)
	var rows [][]models.InlineKeyboardButton
	var currentRow []models.InlineKeyboardButton
	for i, book := range books {
		button := models.InlineKeyboardButton{
			Text:         book.Name,
			CallbackData: fmt.Sprintf("book:%d", i),
		}
		currentRow = append(currentRow, button)

		// Add row when we have 2 buttons or it's the last book
		if len(currentRow) == 2 || i == len(books)-1 {
			rows = append(rows, currentRow)
			currentRow = []models.InlineKeyboardButton{}
		}
	}

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: rows,
	}
	b.sendMessageInThreadWithMarkup(ctx, getChatIDFromQuery(query), "📚 Select a book:", state.MessageThreadID, keyboard)
}

// handleBookCallback processes book selection from inline keyboard
func (b *Bot) handleBookCallback(ctx context.Context, query *models.CallbackQuery, state *ConversationState) {
	indexStr := strings.TrimPrefix(query.Data, "book:")
	bookIdx, err := strconv.Atoi(indexStr)
	if err != nil {
		return
	}

	// Get books to validate selection
	books, err := b.db.ListReadableBooks(ctx)
	if err != nil || bookIdx < 0 || bookIdx >= len(books) {
		b.sendMessageInThread(ctx, getChatIDFromQuery(query), "Error: Invalid book selection", state.MessageThreadID)
		state.Step = -1
		return
	}

	selectedBook := books[bookIdx]
	state.Data["book"] = selectedBook.Name
	state.Step = 3

	// Get participants and show selection
	participants, err := b.db.ListParticipants(ctx)
	if err != nil {
		b.sendMessageInThread(ctx, getChatIDFromQuery(query), fmt.Sprintf("Error: %v", err), state.MessageThreadID)
		state.Step = -1
		return
	}

	// Create inline keyboard for participant selection
	var rows [][]models.InlineKeyboardButton
	for _, p := range participants {
		emoji := "👶"
		if p.IsParent {
			emoji = "👨"
		}
		button := models.InlineKeyboardButton{
			Text:         fmt.Sprintf("%s %s", emoji, p.Name),
			CallbackData: fmt.Sprintf("participant:%s", p.Name),
		}
		rows = append(rows, []models.InlineKeyboardButton{button})
	}

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: rows,
	}
	b.sendMessageInThreadWithMarkup(ctx, getChatIDFromQuery(query), "👤 Select a participant:", state.MessageThreadID, keyboard)
}

// handleParticipantCallback processes participant selection from inline keyboard
func (b *Bot) handleParticipantCallback(ctx context.Context, query *models.CallbackQuery, state *ConversationState) {
	participantName := strings.TrimPrefix(query.Data, "participant:")

	date := state.Data["date"].(time.Time)
	bookName := state.Data["book"].(string)

	// Create the event
	err := b.db.CreateEvent(ctx, date, bookName, participantName)
	if err != nil {
		b.sendMessageInThread(ctx, getChatIDFromQuery(query), fmt.Sprintf("Error creating event: %v", err), state.MessageThreadID)
	} else {
		text := fmt.Sprintf("✅ Reading event recorded!\n\n📅 Date: %s\n📚 Book: %s\n👤 Reader: %s",
			date.Format("2006-01-02"), bookName, participantName)
		b.sendMessageInThread(ctx, getChatIDFromQuery(query), text, state.MessageThreadID)
	}

	state.Step = -1 // Mark conversation as complete
}

// handleStatsPeriodCallback processes time period selection for statistics
func (b *Bot) handleStatsPeriodCallback(ctx context.Context, query *models.CallbackQuery, state *ConversationState) {
	periodType := strings.TrimPrefix(query.Data, "stats_period:")

	now := time.Now()
	var startDate, endDate time.Time

	switch periodType {
	case "month":
		// Show month selection menu
		state.Data["awaiting_month"] = true
		b.sendMessageInThread(ctx, getChatIDFromQuery(query), "📝 Please enter the month in format YYYY-MM\n\nExample: 2024-11", state.MessageThreadID)
		return
	case "year":
		// Show year selection menu
		state.Data["awaiting_year"] = true
		b.sendMessageInThread(ctx, getChatIDFromQuery(query), "📝 Please enter the year\n\nExample: 2024", state.MessageThreadID)
		return
	case "last2":
		startDate = now.AddDate(0, -2, 0)
		endDate = now
		state.Data["period_label"] = "Last 2 months"
	case "last3":
		startDate = now.AddDate(0, -3, 0)
		endDate = now
		state.Data["period_label"] = "Last 3 months"
	case "last6":
		startDate = now.AddDate(0, -6, 0)
		endDate = now
		state.Data["period_label"] = "Last 6 months"
	case "last12":
		startDate = now.AddDate(0, -12, 0)
		endDate = now
		state.Data["period_label"] = "Last 12 months"
	default:
		return
	}

	state.Data["start_date"] = startDate
	state.Data["end_date"] = endDate
	state.Step = 2

	// Show participant selection
	b.showParticipantSelectionForStats(ctx, getChatIDFromQuery(query), state.MessageThreadID)
}

// handleStatsParticipantCallback processes participant selection for statistics
func (b *Bot) handleStatsParticipantCallback(ctx context.Context, query *models.CallbackQuery, state *ConversationState) {
	participantName := strings.TrimPrefix(query.Data, "stats_participant:")
	state.Data["participant_name"] = participantName
	state.Step = 3

	// Show mode selection
	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "📊 Top 10", CallbackData: "stats_mode:top10"},
				{Text: "📋 Полный список", CallbackData: "stats_mode:full"},
			},
		},
	}
	b.sendMessageInThreadWithMarkup(ctx, getChatIDFromQuery(query), "📋 Select display mode:", state.MessageThreadID, keyboard)
}

// handleStatsModeCallback processes display mode selection for statistics
func (b *Bot) handleStatsModeCallback(ctx context.Context, query *models.CallbackQuery, state *ConversationState) {
	mode := strings.TrimPrefix(query.Data, "stats_mode:")

	startDate := state.Data["start_date"].(time.Time)
	endDate := state.Data["end_date"].(time.Time)
	periodLabel := state.Data["period_label"].(string)
	participantName := state.Data["participant_name"].(string)

	limit := 10
	if mode == "full" {
		limit = 0
	}

	b.generateAndSendStatsReport(ctx, getChatIDFromQuery(query), startDate, endDate, periodLabel, participantName, limit, state.MessageThreadID)
	state.Step = -1
}

// showParticipantSelectionForStats shows the participant selection menu for statistics
func (b *Bot) showParticipantSelectionForStats(ctx context.Context, chatID int64, messageThreadID int) {
	participants, err := b.db.ListParticipants(ctx)
	if err != nil {
		b.sendMessageInThread(ctx, chatID, fmt.Sprintf("Error: %v", err), messageThreadID)
		return
	}

	// Filter to show only children
	var children []string
	for _, p := range participants {
		if !p.IsParent {
			children = append(children, p.Name)
		}
	}

	var rows [][]models.InlineKeyboardButton

	// Add "All children" option
	rows = append(rows, []models.InlineKeyboardButton{
		{Text: "👶 All children", CallbackData: "stats_participant:"},
	})

	// Add individual children
	for _, childName := range children {
		button := models.InlineKeyboardButton{
			Text:         fmt.Sprintf("👶 %s", childName),
			CallbackData: fmt.Sprintf("stats_participant:%s", childName),
		}
		rows = append(rows, []models.InlineKeyboardButton{button})
	}

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: rows,
	}
	b.sendMessageInThreadWithMarkup(ctx, chatID, "👥 Select participant:", messageThreadID, keyboard)
}

// generateAndSendStatsReport generates and sends the statistics report
func (b *Bot) generateAndSendStatsReport(ctx context.Context, chatID int64, startDate, endDate time.Time, periodLabel, participantName string, limit int, messageThreadID int) {
	stats, err := b.db.GetTopBooks(ctx, limit, startDate, endDate, participantName)
	if err != nil {
		b.logger.Error("Failed to get top books for stats report",
			zap.Error(err),
			zap.Int64("chat_id", chatID),
			zap.String("participant", participantName),
			zap.Time("start_date", startDate),
			zap.Time("end_date", endDate),
		)
		b.sendMessageInThread(ctx, chatID, fmt.Sprintf("Error: %v", err), messageThreadID)
		return
	}

	if len(stats) == 0 {
		b.logger.Info("No reading events found for stats period",
			zap.Int64("chat_id", chatID),
			zap.String("participant", participantName),
			zap.Time("start_date", startDate),
			zap.Time("end_date", endDate),
		)
		b.sendMessageInThread(ctx, chatID, "No reading events found for the selected period.", messageThreadID)
		return
	}

	b.logger.Info("Generated stats report",
		zap.Int("book_count", len(stats)),
		zap.Int64("chat_id", chatID),
		zap.String("participant", participantName),
	)

	// Format the report
	var text strings.Builder
	text.WriteString("📊 Reading Statistics\n\n")
	text.WriteString(fmt.Sprintf("📅 Period: %s\n", periodLabel))
	text.WriteString(fmt.Sprintf("   %s - %s\n\n", startDate.Format("2006-01-02"), endDate.Format("2006-01-02")))

	if participantName == "" {
		text.WriteString("👥 Participant: All children\n\n")
	} else {
		text.WriteString(fmt.Sprintf("👥 Participant: %s\n\n", participantName))
	}

	if limit > 0 {
		text.WriteString(fmt.Sprintf("📚 Top %d Books:\n\n", limit))
	} else {
		text.WriteString("📚 All Books:\n\n")
	}
	for i, stat := range stats {
		text.WriteString(fmt.Sprintf("%d. %s - %d reads\n", i+1, stat.BookName, stat.ReadCount))
	}

	b.sendMessageInThread(ctx, chatID, text.String(), messageThreadID)
}

// handleRareLabelCallback processes label selection for rare books command
func (b *Bot) handleRareLabelCallback(ctx context.Context, query *models.CallbackQuery, state *ConversationState) {
	label := strings.TrimPrefix(query.Data, "rare_label:")
	const limit = 10

	// Exclude books with "Сами" label from rare books results
	excludeLabels := []string{"Сами"}

	// Get rarely read books by children
	childrenStats, err := b.db.GetRarelyReadBooks(ctx, limit, true, label, excludeLabels)
	if err != nil {
		b.logger.Error("Failed to get rarely read books by children",
			zap.Error(err),
			zap.Int64("user_id", query.From.ID),
		)
		b.sendMessageInThread(ctx, getChatIDFromQuery(query), fmt.Sprintf("Error: %v", err), state.MessageThreadID)
		state.Step = -1
		return
	}

	// Get rarely read books by all participants
	allStats, err := b.db.GetRarelyReadBooks(ctx, limit, false, label, excludeLabels)
	if err != nil {
		b.logger.Error("Failed to get rarely read books by all",
			zap.Error(err),
			zap.Int64("user_id", query.From.ID),
		)
		b.sendMessageInThread(ctx, getChatIDFromQuery(query), fmt.Sprintf("Error: %v", err), state.MessageThreadID)
		state.Step = -1
		return
	}

	var text strings.Builder
	text.WriteString("📚 Rarely read books")
	if label != "" {
		text.WriteString(fmt.Sprintf(" (label: %s)", label))
	}
	text.WriteString(":\n\n")

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

	b.sendMessageInThread(ctx, getChatIDFromQuery(query), text.String(), state.MessageThreadID)
	state.Step = -1 // Mark conversation as complete
}

// handleBookLabelsCallback processes book selection for the book_labels command
func (b *Bot) handleBookLabelsCallback(ctx context.Context, query *models.CallbackQuery, state *ConversationState) {
	indexStr := strings.TrimPrefix(query.Data, "booklabels:")
	bookIdx, err := strconv.Atoi(indexStr)
	if err != nil {
		return
	}

	books, err := b.db.ListReadableBooks(ctx)
	if err != nil || bookIdx < 0 || bookIdx >= len(books) {
		b.sendMessageInThread(ctx, getChatIDFromQuery(query), "Error: Invalid book selection", state.MessageThreadID)
		state.Step = -1
		return
	}

	selectedBook := books[bookIdx]

	var text strings.Builder
	text.WriteString(fmt.Sprintf("🏷 Labels for \"%s\":\n\n", selectedBook.Name))

	if len(selectedBook.Labels) == 0 {
		text.WriteString("No labels found for this book.")
	} else {
		for _, label := range selectedBook.Labels {
			text.WriteString(fmt.Sprintf("• %s\n", label))
		}
	}

	b.sendMessageInThread(ctx, getChatIDFromQuery(query), text.String(), state.MessageThreadID)
	state.Step = -1
}

// handleBooksByLabelCallback processes label selection for the books_by_label command
func (b *Bot) handleBooksByLabelCallback(ctx context.Context, query *models.CallbackQuery, state *ConversationState) {
	label := strings.TrimPrefix(query.Data, "booksbylabel:")

	books, err := b.db.GetBooksByLabel(ctx, label)
	if err != nil {
		b.logger.Error("Failed to get books by label",
			zap.Error(err),
			zap.Int64("user_id", query.From.ID),
			zap.String("label", label),
		)
		b.sendMessageInThread(ctx, getChatIDFromQuery(query), fmt.Sprintf("Error: %v", err), state.MessageThreadID)
		state.Step = -1
		return
	}

	var text strings.Builder
	text.WriteString(fmt.Sprintf("📚 Books with label \"%s\":\n\n", label))

	if len(books) == 0 {
		text.WriteString("No books found with this label.")
	} else {
		for i, book := range books {
			text.WriteString(fmt.Sprintf("%d. %s\n", i+1, book.Name))
		}
	}

	b.sendMessageInThread(ctx, getChatIDFromQuery(query), text.String(), state.MessageThreadID)
	state.Step = -1
}

// handleAddLabelBookCallback processes book selection for add label command
func (b *Bot) handleAddLabelBookCallback(ctx context.Context, query *models.CallbackQuery, state *ConversationState) {
	indexStr := strings.TrimPrefix(query.Data, "addlabel_book:")
	bookIdx, err := strconv.Atoi(indexStr)
	if err != nil {
		return
	}

	// Get books from state
	booksInterface, ok := state.Data["books"]
	if !ok {
		b.sendMessageInThread(ctx, getChatIDFromQuery(query), "Error: Book list not found", state.MessageThreadID)
		state.Step = -1
		return
	}

	books, ok := booksInterface.([]libmodels.Book)
	if !ok || bookIdx < 0 || bookIdx >= len(books) {
		b.sendMessageInThread(ctx, getChatIDFromQuery(query), "Error: Invalid book selection", state.MessageThreadID)
		state.Step = -1
		return
	}

	selectedBook := books[bookIdx]
	label := state.Data["label"].(string)

	// Add label to book
	err = b.db.AddLabelToBook(ctx, selectedBook.Name, label)
	if err != nil {
		b.logger.Error("Failed to add label to book",
			zap.Error(err),
			zap.Int64("user_id", query.From.ID),
			zap.String("book", selectedBook.Name),
			zap.String("label", label),
		)
		b.sendMessageInThread(ctx, getChatIDFromQuery(query), fmt.Sprintf("Error: %v", err), state.MessageThreadID)
		state.Step = -1
		return
	}

	b.sendMessageInThread(ctx, getChatIDFromQuery(query),
		fmt.Sprintf("✅ Label '%s' added to book '%s'", label, selectedBook.Name),
		state.MessageThreadID)
	state.Step = -1 // Mark conversation as complete
}
