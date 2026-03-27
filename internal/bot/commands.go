package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"go.uber.org/zap"
	"library/internal/llm"
	appmodels "library/internal/models"
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
/books_by_label - Show books by label
/ask - Ask a question about your library (AI)`

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

// handleAsk handles the /ask command — starts a conversational LLM session
func (b *Bot) handleAsk(ctx context.Context, message *models.Message) {
	if b.llmClient == nil {
		b.sendMessageInThread(ctx, message.Chat.ID,
			"Функция /ask не настроена.",
			message.MessageThreadID)
		return
	}

	systemPrompt, err := b.buildAskContext(ctx)
	if err != nil {
		b.logger.Error("Failed to fetch data for /ask", zap.Error(err))
		b.sendMessageInThread(ctx, message.Chat.ID, "Ошибка при получении данных.", message.MessageThreadID)
		return
	}

	history := []llm.Message{
		{Role: "system", Content: systemPrompt},
	}

	// Check if there's a question inline with the command
	question := strings.TrimSpace(strings.TrimPrefix(message.Text, "/ask"))
	if strings.HasPrefix(question, "@") {
		if idx := strings.Index(question, " "); idx != -1 {
			question = strings.TrimSpace(question[idx:])
		} else {
			question = ""
		}
	}

	userID := message.From.ID

	if question != "" {
		// Inline question — answer immediately
		history = append(history, llm.Message{Role: "user", Content: question})

		answer, err := b.llmClient.Chat(ctx, history)
		if err != nil {
			b.logger.Error("LLM request failed", zap.Error(err))
			b.sendMessageInThread(ctx, message.Chat.ID, "Ошибка при обращении к ИИ. Попробуйте позже.", message.MessageThreadID)
			return
		}

		history = append(history, llm.Message{Role: "assistant", Content: answer})
		b.sendAskResponse(ctx, message.Chat.ID, answer, message.MessageThreadID)
	} else {
		// No question — just enter interactive mode
		b.sendMessageInThread(ctx, message.Chat.ID,
			"🤖 Режим ИИ-ассистента. Задавайте вопросы о библиотеке. Любая /команда завершит сессию.",
			message.MessageThreadID)
	}

	b.statesMu.Lock()
	b.states[userID] = &ConversationState{
		Command:         "ask",
		Step:            1,
		Data:            map[string]interface{}{"history": history},
		MessageThreadID: message.MessageThreadID,
	}
	b.statesMu.Unlock()
}

// buildAskContext fetches library data and builds the system prompt.
func (b *Bot) buildAskContext(ctx context.Context) (string, error) {
	books, err := b.db.ListReadableBooks(ctx)
	if err != nil {
		return "", err
	}

	participants, err := b.db.ListParticipants(ctx)
	if err != nil {
		return "", err
	}

	events, err := b.db.GetLastEvents(ctx, 50)
	if err != nil {
		return "", err
	}

	return buildAskSystemPrompt(books, participants, events), nil
}

// sendAskResponse sends an LLM answer, truncating if necessary for Telegram's limit.
func (b *Bot) sendAskResponse(ctx context.Context, chatID int64, answer string, threadID int) {
	if len([]rune(answer)) > 4096 {
		answer = string([]rune(answer)[:4093]) + "..."
	}
	b.sendMessageInThread(ctx, chatID, answer, threadID)
}

func buildAskSystemPrompt(books []appmodels.Book, participants []appmodels.Participant, events []appmodels.Event) string {
	var sb strings.Builder

	sb.WriteString("Ты — помощник семейной библиотеки. Отвечай на русском языке. Будь кратким и точным.\n")
	sb.WriteString("Используй ТОЛЬКО предоставленные данные. Считай внимательно, проверяй даты.\n")
	sb.WriteString("Если данных недостаточно для ответа, так и скажи.\n\n")
	sb.WriteString(fmt.Sprintf("Сегодняшняя дата: %s\n\n", time.Now().Format("2006-01-02")))

	sb.WriteString(fmt.Sprintf("== Книги (всего: %d) ==\n", len(books)))
	for i, book := range books {
		line := fmt.Sprintf("%d. %s", i+1, book.Name)
		if len(book.Labels) > 0 {
			line += fmt.Sprintf(" [метки: %s]", strings.Join(book.Labels, ", "))
		}
		sb.WriteString(line + "\n")
	}
	if len(books) == 0 {
		sb.WriteString("(нет книг)\n")
	}

	sb.WriteString(fmt.Sprintf("\n== Участники (всего: %d) ==\n", len(participants)))
	for i, p := range participants {
		role := "ребёнок"
		if p.IsParent {
			role = "родитель"
		}
		sb.WriteString(fmt.Sprintf("%d. %s (%s)\n", i+1, p.Name, role))
	}
	if len(participants) == 0 {
		sb.WriteString("(нет участников)\n")
	}

	sb.WriteString(fmt.Sprintf("\n== Последние события чтения (всего: %d) ==\n", len(events)))
	sb.WriteString("Формат: №. ДАТА | КТО | КНИГА\n")
	for i, e := range events {
		sb.WriteString(fmt.Sprintf("%d. %s | %s | %s\n", i+1, e.Date.Format("2006-01-02"), e.ParticipantName, e.BookName))
	}
	if len(events) == 0 {
		sb.WriteString("(нет событий)\n")
	}

	return sb.String()
}
