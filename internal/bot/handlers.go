package bot

import (
	"context"
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// handleMessage processes a single message
func (b *Bot) handleMessage(message *tgbotapi.Message) {
	// Recover from panics to prevent bot crashes
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from panic in handleMessage: %v", r)
			msg := tgbotapi.NewMessage(message.Chat.ID, "An error occurred while processing your request. Please try again.")
			b.sendMessage(msg)
		}
	}()

	userID := message.From.ID
	ctx := context.Background()

	// Check if user is in a conversation
	if state, ok := b.states[userID]; ok {
		// If conversation is already complete (Step == -1), clean it up and process as new command
		if state.Step == -1 {
			delete(b.states, userID)
		} else if message.IsCommand() {
			// Allow any command to interrupt/cancel an ongoing conversation
			delete(b.states, userID)
			// Continue to process the new command below
		} else {
			// Not a command, continue the conversation
			b.handleConversation(ctx, message, state)
			return
		}
	}

	// Handle commands
	if message.IsCommand() {
		switch message.Command() {
		case "start":
			b.handleStart(message)
		case "new_book":
			b.handleNewBookStart(message)
		case "read":
			b.handleReadStart(ctx, message)
		case "who_is_next":
			b.handleWhoIsNext(ctx, message)
		case "last":
			b.handleLast(ctx, message)
		case "stats":
			b.handleStatsStart(ctx, message)
		default:
			msg := tgbotapi.NewMessage(message.Chat.ID, "Unknown command. Use /start to see available commands.")
			b.sendMessage(msg)
		}
	}
}

// handleCallbackQuery processes inline keyboard button clicks
func (b *Bot) handleCallbackQuery(query *tgbotapi.CallbackQuery) {
	// Recover from panics
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from panic in handleCallbackQuery: %v", r)
		}
	}()

	userID := query.From.ID
	ctx := context.Background()

	// Answer the callback query to remove loading state
	callback := tgbotapi.NewCallback(query.ID, "")
	if b.api != nil {
		b.api.Request(callback)
	}

	// Check if user is in a conversation
	state, ok := b.states[userID]
	if !ok {
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
	}

	// Clean up completed conversations
	if state.Step == -1 {
		delete(b.states, userID)
	}
}
