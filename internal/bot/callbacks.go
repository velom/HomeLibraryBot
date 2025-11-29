package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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
		msg := tgbotapi.NewMessage(query.Message.Chat.ID, fmt.Sprintf("Error: %v", err))
		b.sendMessage(msg)
		state.Step = -1
		return
	}

	// Create inline keyboard for book selection
	msg := tgbotapi.NewMessage(query.Message.Chat.ID, "📚 Select a book:")

	var rows [][]tgbotapi.InlineKeyboardButton
	for i, book := range books {
		button := tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("%s by %s", book.Name, book.Author),
			fmt.Sprintf("book:%d", i),
		)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(button))
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
