package bot

import (
	"context"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	b.sendMessage(msg)
}

// handleNewBookStart initiates the new book conversation
func (b *Bot) handleNewBookStart(message *tgbotapi.Message) {
	userID := message.From.ID
	b.states[userID] = &ConversationState{
		Command: "new_book",
		Step:    1,
		Data:    make(map[string]interface{}),
	}

	msg := tgbotapi.NewMessage(message.Chat.ID, "Please enter the book name:")
	b.sendMessage(msg)
}

// handleReadStart initiates the read event conversation
func (b *Bot) handleReadStart(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID

	// Get readable books
	books, err := b.db.ListReadableBooks(ctx)
	if err != nil {
		msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Error: %v", err))
		b.sendMessage(msg)
		return
	}

	if len(books) == 0 {
		msg := tgbotapi.NewMessage(message.Chat.ID, "No readable books available. Please add books first with /new_book")
		b.sendMessage(msg)
		return
	}

	b.states[userID] = &ConversationState{
		Command: "read",
		Step:    1,
		Data:    make(map[string]interface{}),
	}

	// Show date selection with inline keyboard
	msg := tgbotapi.NewMessage(message.Chat.ID, "📅 Select reading date:")

	// Create inline keyboard with date options
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
	msg.ReplyMarkup = keyboard
	b.sendMessage(msg)
}

// handleWhoIsNext shows who should read next based on rotation logic
func (b *Bot) handleWhoIsNext(ctx context.Context, message *tgbotapi.Message) {
	// Get all participants
	participants, err := b.db.ListParticipants(ctx)
	if err != nil {
		msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Error: %v", err))
		b.sendMessage(msg)
		return
	}

	if len(participants) == 0 {
		msg := tgbotapi.NewMessage(message.Chat.ID, "No participants found in database")
		b.sendMessage(msg)
		return
	}

	// Get the last event to determine last reader
	events, err := b.db.GetLastEvents(ctx, 1)
	if err != nil {
		msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Error: %v", err))
		b.sendMessage(msg)
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
		msg := tgbotapi.NewMessage(message.Chat.ID, "No child participants found in database")
		b.sendMessage(msg)
		return
	}

	text := fmt.Sprintf("Next to read: %s", nextReader)
	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	b.sendMessage(msg)
}

// handleLast shows the last 10 reading events
func (b *Bot) handleLast(ctx context.Context, message *tgbotapi.Message) {
	events, err := b.db.GetLastEvents(ctx, 10)
	if err != nil {
		msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Error: %v", err))
		b.sendMessage(msg)
		return
	}

	if len(events) == 0 {
		msg := tgbotapi.NewMessage(message.Chat.ID, "No reading events recorded yet.")
		b.sendMessage(msg)
		return
	}

	var text strings.Builder
	text.WriteString("Last reading events:\n\n")
	for i, event := range events {
		text.WriteString(fmt.Sprintf("%d. %s - %s (%s)\n",
			i+1,
			event.Date.Format("2006-01-02"),
			event.BookName,
			event.ParticipantName))
	}

	msg := tgbotapi.NewMessage(message.Chat.ID, text.String())
	b.sendMessage(msg)
}

// handleStatsStart initiates the statistics conversation
func (b *Bot) handleStatsStart(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID
	b.states[userID] = &ConversationState{
		Command: "stats",
		Step:    1,
		Data:    make(map[string]interface{}),
	}

	// Show time period selection
	msg := tgbotapi.NewMessage(message.Chat.ID, "📊 Select time period for statistics:")

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
	msg.ReplyMarkup = keyboard
	b.sendMessage(msg)
}
