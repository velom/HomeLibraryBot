package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"
)

// handleDateCallback processes date selection from inline keyboard
func (b *Bot) handleDateCallback(ctx context.Context, query *tgbotapi.CallbackQuery, state *ConversationState) {
	data := strings.TrimPrefix(query.Data, "date:")

	// Handle custom date option
	if data == "custom" {
		state.Data["awaiting_custom_date"] = true
		msg := tgbotapi.NewMessage(query.Message.Chat.ID, "📝 Please enter the date in format YYYY-MM-DD\n\nExample: 2024-01-15")
		b.sendMessage(msg)
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
		msg := tgbotapi.NewMessage(query.Message.Chat.ID, fmt.Sprintf("Error: %v", err))
		b.sendMessage(msg)
		state.Step = -1
		return
	}

	// Create inline keyboard for book selection (2 columns)
	msg := tgbotapi.NewMessage(query.Message.Chat.ID, "📚 Select a book:")

	var rows [][]tgbotapi.InlineKeyboardButton
	var currentRow []tgbotapi.InlineKeyboardButton
	for i, book := range books {
		button := tgbotapi.NewInlineKeyboardButtonData(
			book.Name,
			fmt.Sprintf("book:%d", i),
		)
		currentRow = append(currentRow, button)

		// Add row when we have 2 buttons or it's the last book
		if len(currentRow) == 2 || i == len(books)-1 {
			rows = append(rows, currentRow)
			currentRow = []tgbotapi.InlineKeyboardButton{}
		}
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	msg.ReplyMarkup = keyboard
	b.sendMessage(msg)
}

// handleBookCallback processes book selection from inline keyboard
func (b *Bot) handleBookCallback(ctx context.Context, query *tgbotapi.CallbackQuery, state *ConversationState) {
	indexStr := strings.TrimPrefix(query.Data, "book:")
	bookIdx, err := strconv.Atoi(indexStr)
	if err != nil {
		return
	}

	// Get books to validate selection
	books, err := b.db.ListReadableBooks(ctx)
	if err != nil || bookIdx < 0 || bookIdx >= len(books) {
		msg := tgbotapi.NewMessage(query.Message.Chat.ID, "Error: Invalid book selection")
		b.sendMessage(msg)
		state.Step = -1
		return
	}

	selectedBook := books[bookIdx]
	state.Data["book"] = selectedBook.Name
	state.Step = 3

	// Get participants and show selection
	participants, err := b.db.ListParticipants(ctx)
	if err != nil {
		msg := tgbotapi.NewMessage(query.Message.Chat.ID, fmt.Sprintf("Error: %v", err))
		b.sendMessage(msg)
		state.Step = -1
		return
	}

	// Create inline keyboard for participant selection
	msg := tgbotapi.NewMessage(query.Message.Chat.ID, "👤 Select a participant:")

	var rows [][]tgbotapi.InlineKeyboardButton
	for _, p := range participants {
		emoji := "👶"
		if p.IsParent {
			emoji = "👨"
		}
		button := tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("%s %s", emoji, p.Name),
			fmt.Sprintf("participant:%s", p.Name),
		)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(button))
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	msg.ReplyMarkup = keyboard
	b.sendMessage(msg)
}

// handleParticipantCallback processes participant selection from inline keyboard
func (b *Bot) handleParticipantCallback(ctx context.Context, query *tgbotapi.CallbackQuery, state *ConversationState) {
	participantName := strings.TrimPrefix(query.Data, "participant:")

	date := state.Data["date"].(time.Time)
	bookName := state.Data["book"].(string)

	// Create the event
	err := b.db.CreateEvent(ctx, date, bookName, participantName)
	if err != nil {
		msg := tgbotapi.NewMessage(query.Message.Chat.ID, fmt.Sprintf("Error creating event: %v", err))
		b.sendMessage(msg)
	} else {
		text := fmt.Sprintf("✅ Reading event recorded!\n\n📅 Date: %s\n📚 Book: %s\n👤 Reader: %s",
			date.Format("2006-01-02"), bookName, participantName)
		msg := tgbotapi.NewMessage(query.Message.Chat.ID, text)
		b.sendMessage(msg)
	}

	state.Step = -1 // Mark conversation as complete
}

// handleStatsPeriodCallback processes time period selection for statistics
func (b *Bot) handleStatsPeriodCallback(ctx context.Context, query *tgbotapi.CallbackQuery, state *ConversationState) {
	periodType := strings.TrimPrefix(query.Data, "stats_period:")

	now := time.Now()
	var startDate, endDate time.Time

	switch periodType {
	case "month":
		// Show month selection menu
		state.Data["awaiting_month"] = true
		msg := tgbotapi.NewMessage(query.Message.Chat.ID, "📝 Please enter the month in format YYYY-MM\n\nExample: 2024-11")
		b.sendMessage(msg)
		return
	case "year":
		// Show year selection menu
		state.Data["awaiting_year"] = true
		msg := tgbotapi.NewMessage(query.Message.Chat.ID, "📝 Please enter the year\n\nExample: 2024")
		b.sendMessage(msg)
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
	b.showParticipantSelectionForStats(ctx, query.Message.Chat.ID)
}

// handleStatsParticipantCallback processes participant selection for statistics
func (b *Bot) handleStatsParticipantCallback(ctx context.Context, query *tgbotapi.CallbackQuery, state *ConversationState) {
	participantName := strings.TrimPrefix(query.Data, "stats_participant:")

	startDate := state.Data["start_date"].(time.Time)
	endDate := state.Data["end_date"].(time.Time)
	periodLabel := state.Data["period_label"].(string)

	// Generate and send the report
	b.generateAndSendStatsReport(ctx, query.Message.Chat.ID, startDate, endDate, periodLabel, participantName)

	state.Step = -1 // Mark conversation as complete
}

// showParticipantSelectionForStats shows the participant selection menu for statistics
func (b *Bot) showParticipantSelectionForStats(ctx context.Context, chatID int64) {
	participants, err := b.db.ListParticipants(ctx)
	if err != nil {
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Error: %v", err))
		b.sendMessage(msg)
		return
	}

	// Filter to show only children
	var children []string
	for _, p := range participants {
		if !p.IsParent {
			children = append(children, p.Name)
		}
	}

	msg := tgbotapi.NewMessage(chatID, "👥 Select participant:")

	var rows [][]tgbotapi.InlineKeyboardButton

	// Add "All children" option
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("👶 All children", "stats_participant:"),
	))

	// Add individual children
	for _, childName := range children {
		button := tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("👶 %s", childName),
			fmt.Sprintf("stats_participant:%s", childName),
		)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(button))
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	msg.ReplyMarkup = keyboard
	b.sendMessage(msg)
}

// generateAndSendStatsReport generates and sends the statistics report
func (b *Bot) generateAndSendStatsReport(ctx context.Context, chatID int64, startDate, endDate time.Time, periodLabel, participantName string) {
	// Get top 10 books
	stats, err := b.db.GetTopBooks(ctx, 10, startDate, endDate, participantName)
	if err != nil {
		b.logger.Error("Failed to get top books for stats report",
			zap.Error(err),
			zap.Int64("chat_id", chatID),
			zap.String("participant", participantName),
			zap.Time("start_date", startDate),
			zap.Time("end_date", endDate),
		)
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Error: %v", err))
		b.sendMessage(msg)
		return
	}

	if len(stats) == 0 {
		b.logger.Info("No reading events found for stats period",
			zap.Int64("chat_id", chatID),
			zap.String("participant", participantName),
			zap.Time("start_date", startDate),
			zap.Time("end_date", endDate),
		)
		msg := tgbotapi.NewMessage(chatID, "No reading events found for the selected period.")
		b.sendMessage(msg)
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

	text.WriteString("📚 Top 10 Books:\n\n")
	for i, stat := range stats {
		text.WriteString(fmt.Sprintf("%d. %s - %d reads\n", i+1, stat.BookName, stat.ReadCount))
	}

	msg := tgbotapi.NewMessage(chatID, text.String())
	b.sendMessage(msg)
}
