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
		state.Data["name"] = message.Text
		state.Step = 2
		msg := tgbotapi.NewMessage(message.Chat.ID, "Please enter the author name:")
		b.sendMessage(msg)

	case 2: // Waiting for author name
		state.Data["author"] = message.Text
		name := state.Data["name"].(string)
		author := state.Data["author"].(string)

		id, err := b.db.CreateBook(ctx, name, author)
		if err != nil {
			msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Error creating book: %v", err))
			b.sendMessage(msg)
		} else {
			text := fmt.Sprintf("Book created successfully!\nID: %s\nName: %s\nAuthor: %s", id, name, author)
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

		// Create inline keyboard for book selection
		msg := tgbotapi.NewMessage(message.Chat.ID, "📚 Select a book:")

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
