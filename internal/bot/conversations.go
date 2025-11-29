package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// handleConversation processes multi-step conversations
func (b *Bot) handleConversation(ctx context.Context, message *tgbotapi.Message, state *ConversationState) {
	userID := message.From.ID

	switch state.Command {
	case "new_book":
		b.handleNewBookConversation(ctx, message, state)
	case "read":
		b.handleReadConversation(ctx, message, state)
	case "stats":
		b.handleStatsConversation(ctx, message, state)
	}

	// Clean up completed conversations
	if state.Step == -1 {
		delete(b.states, userID)
	}
}

// handleNewBookConversation handles the new book multi-step process
func (b *Bot) handleNewBookConversation(ctx context.Context, message *tgbotapi.Message, state *ConversationState) {
	switch state.Step {
	case 1: // Waiting for book name
		name := message.Text

		id, err := b.db.CreateBook(ctx, name)
		if err != nil {
			msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Error creating book: %v", err))
			b.sendMessage(msg)
		} else {
			text := fmt.Sprintf("Book created successfully!\nName: %s", id)
			msg := tgbotapi.NewMessage(message.Chat.ID, text)
			b.sendMessage(msg)
		}

		state.Step = -1 // Mark conversation as complete
	}
}

// handleReadConversation handles the read event multi-step process
func (b *Bot) handleReadConversation(ctx context.Context, message *tgbotapi.Message, state *ConversationState) {
	switch state.Step {
	case 1: // Waiting for custom date input
		// Check if we're awaiting custom date
		if _, ok := state.Data["awaiting_custom_date"]; !ok {
			// Not awaiting custom date, ignore text input
			return
		}

		var date time.Time
		var err error

		if strings.ToLower(message.Text) == "today" {
			date = time.Now()
		} else {
			date, err = time.Parse("2006-01-02", message.Text)
			if err != nil {
				msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Invalid date format. Please use YYYY-MM-DD\n\nExample: 2024-01-15")
				b.sendMessage(msg)
				return
			}
		}

		// Clear the awaiting flag
		delete(state.Data, "awaiting_custom_date")
		state.Data["date"] = date
		state.Step = 2

		// Show book selection with inline keyboard
		books, err := b.db.ListReadableBooks(ctx)
		if err != nil {
			msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Error: %v", err))
			b.sendMessage(msg)
			state.Step = -1
			return
		}

		// Create inline keyboard for book selection (2 columns)
		msg := tgbotapi.NewMessage(message.Chat.ID, "📚 Select a book:")

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

	case 2: // Waiting for book selection
		bookIdx, err := strconv.Atoi(message.Text)
		if err != nil {
			msg := tgbotapi.NewMessage(message.Chat.ID, "Invalid selection. Please enter a valid number:")
			b.sendMessage(msg)
			return
		}

		// Get books list to validate selection
		bookList, err := b.db.ListReadableBooks(ctx)
		if err != nil {
			msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Error: %v", err))
			b.sendMessage(msg)
			state.Step = -1
			return
		}

		if bookIdx < 1 || bookIdx > len(bookList) {
			msg := tgbotapi.NewMessage(message.Chat.ID, "Invalid selection. Please enter a valid number:")
			b.sendMessage(msg)
			return
		}

		selectedBook := bookList[bookIdx-1]
		state.Data["book"] = selectedBook.Name
		state.Step = 3

		// Show participant selection
		participants, err := b.db.ListParticipants(ctx)
		if err != nil {
			msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Error: %v", err))
			b.sendMessage(msg)
			state.Step = -1
			return
		}

		var text strings.Builder
		text.WriteString("Please select a participant by number:\n\n")
		for i, p := range participants {
			text.WriteString(fmt.Sprintf("%d. %s\n", i+1, p.Name))
		}

		msg := tgbotapi.NewMessage(message.Chat.ID, text.String())
		b.sendMessage(msg)

	case 3: // Waiting for participant selection
		participantIdx, err := strconv.Atoi(message.Text)
		if err != nil {
			msg := tgbotapi.NewMessage(message.Chat.ID, "Invalid selection. Please enter a valid number:")
			b.sendMessage(msg)
			return
		}

		// Get participants list to validate selection
		participantList, err := b.db.ListParticipants(ctx)
		if err != nil {
			msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Error: %v", err))
			b.sendMessage(msg)
			state.Step = -1
			return
		}

		if participantIdx < 1 || participantIdx > len(participantList) {
			msg := tgbotapi.NewMessage(message.Chat.ID, "Invalid selection. Please enter a valid number:")
			b.sendMessage(msg)
			return
		}

		selectedParticipant := participantList[participantIdx-1]
		date := state.Data["date"].(time.Time)
		bookName := state.Data["book"].(string)

		// Create the event
		err = b.db.CreateEvent(ctx, date, bookName, selectedParticipant.Name)
		if err != nil {
			msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Error creating event: %v", err))
			b.sendMessage(msg)
		} else {
			text := fmt.Sprintf("Reading event recorded!\n\nDate: %s\nBook: %s\nReader: %s",
				date.Format("2006-01-02"), bookName, selectedParticipant.Name)
			msg := tgbotapi.NewMessage(message.Chat.ID, text)
			b.sendMessage(msg)
		}

		state.Step = -1 // Mark conversation as complete
	}
}

// handleStatsConversation handles the statistics multi-step process
func (b *Bot) handleStatsConversation(ctx context.Context, message *tgbotapi.Message, state *ConversationState) {
	switch state.Step {
	case 1: // Waiting for month or year input
		// Check if we're awaiting month
		if _, ok := state.Data["awaiting_month"]; ok {
			// Parse month in YYYY-MM format
			date, err := time.Parse("2006-01", message.Text)
			if err != nil {
				msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Invalid month format. Please use YYYY-MM\n\nExample: 2024-11")
				b.sendMessage(msg)
				return
			}

			// Calculate start and end dates for the month
			startDate := time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, time.UTC)
			endDate := startDate.AddDate(0, 1, 0).Add(-time.Second)

			delete(state.Data, "awaiting_month")
			state.Data["start_date"] = startDate
			state.Data["end_date"] = endDate
			state.Data["period_label"] = date.Format("January 2006")
			state.Step = 2

			// Show participant selection
			b.showParticipantSelectionForStats(ctx, message.Chat.ID)
			return
		}

		// Check if we're awaiting year
		if _, ok := state.Data["awaiting_year"]; ok {
			// Parse year
			year, err := strconv.Atoi(message.Text)
			if err != nil || year < 1900 || year > 2100 {
				msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Invalid year. Please enter a valid year\n\nExample: 2024")
				b.sendMessage(msg)
				return
			}

			// Calculate start and end dates for the year
			startDate := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
			endDate := time.Date(year, 12, 31, 23, 59, 59, 0, time.UTC)

			delete(state.Data, "awaiting_year")
			state.Data["start_date"] = startDate
			state.Data["end_date"] = endDate
			state.Data["period_label"] = fmt.Sprintf("Year %d", year)
			state.Step = 2

			// Show participant selection
			b.showParticipantSelectionForStats(ctx, message.Chat.ID)
			return
		}
	}
}
