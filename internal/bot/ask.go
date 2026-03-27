package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"go.uber.org/zap"
	"library/internal/llm"
)

const askSystemPrompt = `Ты — помощник семейной библиотеки. Отвечай на русском языке. Будь кратким и точным.

ПРАВИЛА:
- НИКОГДА не задавай уточняющих вопросов. Принимай решения сам.
- Сразу вызывай инструменты и отвечай на основе полученных данных.
- "Прошлая неделя" = последние 7 дней. "Этот месяц" = с 1-го числа текущего месяца. Решай сам.
- Не придумывай данные — всегда запрашивай через инструменты.
- Считай внимательно, проверяй даты.

ФОРМАТИРОВАНИЕ:
- Используй Telegram HTML: <b>жирный</b>, <i>курсив</i>, <code>код</code>
- НЕ используй Markdown (**, *, #). Только HTML-теги.
- Для списков используй простые переносы строк с тире или номерами

Сегодняшняя дата: %s`

const maxToolIterations = 5

var askTools = []llm.Tool{
	{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "get_books",
			Description: "Получить список всех книг в библиотеке с их метками",
			Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
		},
	},
	{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "get_participants",
			Description: "Получить список всех участников (детей и родителей)",
			Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
		},
	},
	{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "get_last_events",
			Description: "Получить события чтения. Можно фильтровать по дате и участнику. Возвращает: дата, кто выбрал, какую книгу. Для полного списка за период используй limit=100 с since/until",
			Parameters: json.RawMessage(`{"type":"object","properties":{
				"limit":{"type":"integer","description":"Количество событий (по умолчанию 50, максимум 500)"},
				"since":{"type":"string","description":"Дата начала в формате YYYY-MM-DD (только события с этой даты)"},
				"until":{"type":"string","description":"Дата конца в формате YYYY-MM-DD (только события до этой даты включительно)"},
				"participant":{"type":"string","description":"Имя участника для фильтрации (пусто = все)"}
			}}`),
		},
	},
	{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "get_top_books",
			Description: "Получить топ книг по количеству прочтений за указанный период",
			Parameters: json.RawMessage(`{"type":"object","properties":{
				"days":{"type":"integer","description":"За сколько последних дней считать (по умолчанию 30)"},
				"participant":{"type":"string","description":"Имя участника для фильтрации (пусто = все дети)"},
				"limit":{"type":"integer","description":"Сколько книг вернуть (по умолчанию 10)"}
			}}`),
		},
	},
	{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "get_rarely_read_books",
			Description: "Получить книги, которые давно не читали, отсортированные по дате последнего прочтения",
			Parameters: json.RawMessage(`{"type":"object","properties":{
				"label":{"type":"string","description":"Фильтр по метке (пусто = все книги)"},
				"limit":{"type":"integer","description":"Сколько книг вернуть (по умолчанию 10)"}
			}}`),
		},
	},
	{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "get_labels",
			Description: "Получить список всех меток (категорий) книг",
			Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
		},
	},
}

// handleAsk handles the /ask command — starts a conversational LLM session with tool use
func (b *Bot) handleAsk(ctx context.Context, message *models.Message) {
	if b.llmClient == nil {
		b.sendMessageInThread(ctx, message.Chat.ID, "Функция /ask не настроена.", message.MessageThreadID)
		return
	}

	history := []llm.Message{
		{Role: "system", Content: fmt.Sprintf(askSystemPrompt, time.Now().Format("2006-01-02"))},
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
		history = append(history, llm.Message{Role: "user", Content: question})
		answer, newHistory, err := b.runAskWithTools(ctx, history)
		if err != nil {
			b.logger.Error("LLM request failed", zap.Error(err))
			b.sendMessageInThread(ctx, message.Chat.ID, "Ошибка при обращении к ИИ. Попробуйте позже.", message.MessageThreadID)
			return
		}
		history = newHistory
		b.sendAskResponse(ctx, message.Chat.ID, answer, message.MessageThreadID)
	} else {
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

// handleAskConversation handles follow-up messages in an /ask conversation
func (b *Bot) handleAskConversation(ctx context.Context, message *models.Message, state *ConversationState) {
	history, ok := state.Data["history"].([]llm.Message)
	if !ok {
		b.logger.Error("Invalid ask conversation state")
		state.Step = -1
		return
	}

	history = append(history, llm.Message{Role: "user", Content: message.Text})

	answer, newHistory, err := b.runAskWithTools(ctx, history)
	if err != nil {
		b.logger.Error("LLM request failed", zap.Error(err))
		b.sendMessageInThread(ctx, message.Chat.ID, "Ошибка при обращении к ИИ. Попробуйте позже.", state.MessageThreadID)
		return
	}

	state.Data["history"] = newHistory
	b.sendAskResponse(ctx, message.Chat.ID, answer, state.MessageThreadID)
}

// runAskWithTools executes the tool-calling loop: LLM requests tools, bot executes them, repeats.
func (b *Bot) runAskWithTools(ctx context.Context, history []llm.Message) (string, []llm.Message, error) {
	for i := 0; i < maxToolIterations; i++ {
		resp, err := b.llmClient.ChatWithTools(ctx, history, askTools)
		if err != nil {
			return "", history, err
		}

		if !resp.HasToolCalls() {
			// LLM returned a text answer
			history = append(history, resp.AssistantMessage)
			return resp.Content, history, nil
		}

		// LLM wants to call tools — execute them
		b.logger.Debug("LLM requested tool calls",
			zap.Int("count", len(resp.ToolCalls)),
			zap.Int("iteration", i+1),
		)

		// Append the raw assistant message (preserves thought_signature for Gemini)
		history = append(history, resp.AssistantMessage)

		// Execute each tool and append results
		for _, tc := range resp.ToolCalls {
			result := b.executeTool(ctx, tc.Function.Name, tc.Function.Arguments)
			history = append(history, llm.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})
		}
	}

	return "Превышен лимит обращений к данным. Попробуйте упростить вопрос.", history, nil
}

// executeTool runs a tool by name and returns the result as a string.
func (b *Bot) executeTool(ctx context.Context, name, argsJSON string) string {
	b.logger.Debug("Executing tool", zap.String("name", name), zap.String("args", argsJSON))

	var args map[string]any
	if argsJSON != "" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return fmt.Sprintf("error: invalid arguments: %v", err)
		}
	}

	switch name {
	case "get_books":
		return b.toolGetBooks(ctx)
	case "get_participants":
		return b.toolGetParticipants(ctx)
	case "get_last_events":
		limit := intArg(args, "limit", 50)
		if limit > 500 {
			limit = 500
		}
		since := stringArg(args, "since", "")
		until := stringArg(args, "until", "")
		participant := stringArg(args, "participant", "")
		return b.toolGetLastEvents(ctx, limit, since, until, participant)
	case "get_top_books":
		days := intArg(args, "days", 30)
		participant := stringArg(args, "participant", "")
		limit := intArg(args, "limit", 10)
		return b.toolGetTopBooks(ctx, days, participant, limit)
	case "get_rarely_read_books":
		label := stringArg(args, "label", "")
		limit := intArg(args, "limit", 10)
		return b.toolGetRarelyReadBooks(ctx, label, limit)
	case "get_labels":
		return b.toolGetLabels(ctx)
	default:
		return fmt.Sprintf("error: unknown tool %q", name)
	}
}

func (b *Bot) toolGetBooks(ctx context.Context) string {
	books, err := b.db.ListReadableBooks(ctx)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	var sb strings.Builder
	for i, book := range books {
		sb.WriteString(fmt.Sprintf("%d. %s", i+1, book.Name))
		if len(book.Labels) > 0 {
			sb.WriteString(fmt.Sprintf(" [%s]", strings.Join(book.Labels, ", ")))
		}
		sb.WriteString("\n")
	}
	if len(books) == 0 {
		sb.WriteString("(нет книг)\n")
	}
	return sb.String()
}

func (b *Bot) toolGetParticipants(ctx context.Context) string {
	participants, err := b.db.ListParticipants(ctx)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	var sb strings.Builder
	for _, p := range participants {
		role := "ребёнок"
		if p.IsParent {
			role = "родитель"
		}
		sb.WriteString(fmt.Sprintf("%s (%s)\n", p.Name, role))
	}
	if len(participants) == 0 {
		sb.WriteString("(нет участников)\n")
	}
	return sb.String()
}

func (b *Bot) toolGetLastEvents(ctx context.Context, limit int, since, until, participant string) string {
	var sinceDate, untilDate time.Time
	if since != "" {
		var err error
		sinceDate, err = time.Parse("2006-01-02", since)
		if err != nil {
			return fmt.Sprintf("error: invalid 'since' date format %q, expected YYYY-MM-DD", since)
		}
	}
	if until != "" {
		var err error
		untilDate, err = time.Parse("2006-01-02", until)
		if err != nil {
			return fmt.Sprintf("error: invalid 'until' date format %q, expected YYYY-MM-DD", until)
		}
		// Include the entire "until" day
		untilDate = untilDate.Add(24*time.Hour - time.Second)
	}

	events, err := b.db.GetLastEventsFiltered(ctx, limit, sinceDate, untilDate, strings.TrimSpace(participant))
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	var sb strings.Builder
	for i, e := range events {
		sb.WriteString(fmt.Sprintf("%d. %s | %s | %s\n", i+1, e.Date.Format("2006-01-02"), e.ParticipantName, e.BookName))
	}
	if len(events) == 0 {
		sb.WriteString("(нет событий за указанный период)\n")
	}
	return sb.String()
}

func (b *Bot) toolGetTopBooks(ctx context.Context, days int, participant string, limit int) string {
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -days)
	stats, err := b.db.GetTopBooks(ctx, limit, startDate, endDate, participant)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	var sb strings.Builder
	for i, s := range stats {
		sb.WriteString(fmt.Sprintf("%d. %s — %d раз\n", i+1, s.BookName, s.ReadCount))
	}
	if len(stats) == 0 {
		sb.WriteString("(нет данных за этот период)\n")
	}
	return sb.String()
}

func (b *Bot) toolGetRarelyReadBooks(ctx context.Context, label string, limit int) string {
	stats, err := b.db.GetRarelyReadBooks(ctx, limit, true, label, nil)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	var sb strings.Builder
	for i, s := range stats {
		if s.LastReadDate != nil {
			sb.WriteString(fmt.Sprintf("%d. %s — последнее чтение: %s (%d дн. назад)\n",
				i+1, s.BookName, s.LastReadDate.Format("2006-01-02"), s.DaysSinceLastRead))
		} else {
			sb.WriteString(fmt.Sprintf("%d. %s — никогда не читали\n", i+1, s.BookName))
		}
	}
	if len(stats) == 0 {
		sb.WriteString("(нет данных)\n")
	}
	return sb.String()
}

func (b *Bot) toolGetLabels(ctx context.Context) string {
	labels, err := b.db.GetAllLabels(ctx)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	if len(labels) == 0 {
		return "(нет меток)\n"
	}
	return strings.Join(labels, ", ") + "\n"
}

// sendAskResponse sends an LLM answer with HTML formatting, truncating if necessary.
func (b *Bot) sendAskResponse(ctx context.Context, chatID int64, answer string, threadID int) {
	if b.api == nil {
		return
	}

	if len([]rune(answer)) > 4096 {
		answer = string([]rune(answer)[:4093]) + "..."
	}

	params := &tgbot.SendMessageParams{
		ChatID:    chatID,
		Text:      answer,
		ParseMode: models.ParseModeHTML,
	}
	if threadID != 0 {
		params.MessageThreadID = threadID
	}

	_, err := b.api.SendMessage(ctx, params)
	if err != nil {
		// If HTML parsing fails (malformed tags from LLM), retry without parse mode
		b.logger.Warn("Failed to send HTML message, retrying as plain text", zap.Error(err))
		params.ParseMode = ""
		b.api.SendMessage(ctx, params)
	}
}

// Helper functions for extracting typed args from JSON
func intArg(args map[string]any, key string, defaultVal int) int {
	if v, ok := args[key]; ok {
		if f, ok := v.(float64); ok {
			return int(f)
		}
	}
	return defaultVal
}

func stringArg(args map[string]any, key string, defaultVal string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return defaultVal
}
