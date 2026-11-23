package bot

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
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
- Вопросы «что не читали / ни разу не читали / 0 прочтений» — только через get_unread_books.
- Участники перечислены ниже — get_participants для этого не вызывай. В параметр participant передавай имя из этого списка (в именительном падеже).

ФОРМАТИРОВАНИЕ:
- Используй Telegram HTML: <b>жирный</b>, <i>курсив</i>, <code>код</code>
- НЕ используй Markdown (**, *, #). Только HTML-теги.
- Для списков используй простые переносы строк с тире или номерами

Сегодняшняя дата: %s
Участники: %s`

// askSystemMessage builds the system prompt. Participants are inlined so the model doesn't spend an
// LLM round trip on get_participants before every per-person question.
func (b *Bot) askSystemMessage(ctx context.Context) llm.Message {
	participants := "(см. get_participants)"
	if list, err := b.db.ListParticipants(ctx); err != nil {
		b.log(ctx).Warn("Failed to list participants for /ask prompt", zap.Error(err))
	} else if len(list) > 0 {
		parts := make([]string, 0, len(list))
		for _, p := range list {
			role := "ребёнок"
			if p.IsParent {
				role = "родитель"
			}
			parts = append(parts, fmt.Sprintf("%s (%s)", p.Name, role))
		}
		participants = strings.Join(parts, ", ")
	}
	return llm.Message{Role: "system", Content: fmt.Sprintf(askSystemPrompt, time.Now().Format("2006-01-02"), participants)}
}

const maxToolIterations = 5

// askTraceName names each /ask turn in Langfuse. Keep it stable: evaluators and dashboards reference it.
const askTraceName = "answer-question"

const iterationLimitAnswer = "Превышен лимит обращений к данным. Попробуйте упростить вопрос."

// askTraceAttrs builds the Langfuse trace attributes for an /ask turn: one trace per turn,
// one session per /ask conversation.
func (b *Bot) askTraceAttrs(userID int64, sessionID string) llm.TraceAttrs {
	return llm.TraceAttrs{
		SessionID:   sessionID,
		UserID:      strconv.FormatInt(userID, 10),
		TraceName:   askTraceName,
		Environment: b.llmClient.Environment(),
		Tags:        []string{"ask"},
	}
}

func newAskSessionID() string {
	return "ask-" + rand.Text()
}

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
				"participant":{"type":"string","description":"Имя участника для фильтрации (пусто = все). Регистр не важен"}
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
				"participant":{"type":"string","description":"Имя участника для фильтрации (пусто = все дети). Регистр не важен"},
				"limit":{"type":"integer","description":"Сколько книг вернуть (по умолчанию 10)"}
			}}`),
		},
	},
	{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "get_rarely_read_books",
			Description: "Получить книги, которые читали давно, отсортированные по дате последнего прочтения (самые давние первыми). По умолчанию учитываются все участники. Книги, которые не читали ни разу, здесь не выводятся — для них используй get_unread_books",
			Parameters: json.RawMessage(`{"type":"object","properties":{
				"label":{"type":"string","description":"Фильтр по метке (пусто = все книги)"},
				"limit":{"type":"integer","description":"Сколько книг вернуть (по умолчанию 10)"},
				"readers":{"type":"string","enum":["all","children"],"description":"Чьи чтения учитывать: all — все участники, children — только дети (по умолчанию all)"}
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
	{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "get_detailed_book_stats",
			Description: "Получить детальную статистику по книгам: кто сколько раз прочитал каждую книгу и когда был последний раз. Возвращает только пары книга×участник с ненулевым числом прочтений за период; если ничего не прочитано — «(нет данных)». Для общей картины вызывай без фильтров, для конкретной книги — с параметром book. Для непрочитанных книг используй get_unread_books",
			Parameters: json.RawMessage(`{"type":"object","properties":{
				"since":{"type":"string","description":"Дата начала периода YYYY-MM-DD (по умолчанию: всё время)"},
				"until":{"type":"string","description":"Дата конца периода YYYY-MM-DD (по умолчанию: всё время)"},
				"book":{"type":"string","description":"Название книги для фильтрации (по умолчанию: все книги)"},
				"participant":{"type":"string","description":"Имя участника для фильтрации (по умолчанию: все участники). Регистр не важен"}
			}}`),
		},
	},
	{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "get_participant_stats",
			Description: "Получить статистику в разрезе читающих: кто сколько каких книг прочитал. Возвращает только прочитанные за период книги (без нулевых); если ничего не прочитано — «(нет данных)». Для конкретного участника — с параметром participant. Для непрочитанных книг используй get_unread_books",
			Parameters: json.RawMessage(`{"type":"object","properties":{
				"since":{"type":"string","description":"Дата начала периода YYYY-MM-DD (по умолчанию: всё время)"},
				"until":{"type":"string","description":"Дата конца периода YYYY-MM-DD (по умолчанию: всё время)"},
				"book":{"type":"string","description":"Название книги для фильтрации (по умолчанию: все книги)"},
				"participant":{"type":"string","description":"Имя участника для фильтрации (по умолчанию: все участники). Регистр не важен"}
			}}`),
		},
	},
	{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "get_unread_books",
			Description: "Книги, которые НЕ ЧИТАЛИ НИ РАЗУ — за всё время (без дат) или в указанном периоде (since/until); можно ограничить участником и меткой. Используй для вопросов «не читали», «ни разу», «0 прочтений», «что ещё не читали». Возвращает точный список — сверять списки самому не нужно",
			Parameters: json.RawMessage(`{"type":"object","properties":{
				"since":{"type":"string","description":"Дата начала периода YYYY-MM-DD (по умолчанию: всё время)"},
				"until":{"type":"string","description":"Дата конца периода YYYY-MM-DD включительно (по умолчанию: всё время)"},
				"participant":{"type":"string","description":"Имя участника: книги, которые не читал именно он (по умолчанию: никто). Регистр не важен"},
				"label":{"type":"string","description":"Только книги с этой меткой (по умолчанию: все книги)"},
				"readers":{"type":"string","enum":["all","children","parents"],"description":"Кого учитывать: all — все, children — только дети, parents — только родители (по умолчанию all)"}
			}}`),
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
		b.askSystemMessage(ctx),
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
	sessionID := newAskSessionID()

	if question != "" {
		history = append(history, llm.Message{Role: "user", Content: question})
		stopTyping := b.keepTyping(ctx, message.Chat.ID, message.MessageThreadID)
		answer, newHistory, err := b.runAskWithTools(ctx, history, b.askTraceAttrs(userID, sessionID))
		stopTyping()
		if err != nil {
			b.log(ctx).Error("LLM request failed", zap.Error(err))
			b.sendMessageInThread(ctx, message.Chat.ID, fmt.Sprintf("Ошибка при обращении к ИИ: %v", err), message.MessageThreadID)
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
		Data:            map[string]interface{}{"history": history, "session_id": sessionID},
		MessageThreadID: message.MessageThreadID,
	}
	b.statesMu.Unlock()
}

// handleAskConversation handles follow-up messages in an /ask conversation
func (b *Bot) handleAskConversation(ctx context.Context, message *models.Message, state *ConversationState) {
	history, ok := state.Data["history"].([]llm.Message)
	if !ok {
		b.log(ctx).Error("Invalid ask conversation state")
		state.Step = -1
		return
	}

	history = append(history, llm.Message{Role: "user", Content: message.Text})

	sessionID, _ := state.Data["session_id"].(string)
	if sessionID == "" {
		sessionID = newAskSessionID()
		state.Data["session_id"] = sessionID
	}
	stopTyping := b.keepTyping(ctx, message.Chat.ID, state.MessageThreadID)
	answer, newHistory, err := b.runAskWithTools(ctx, history, b.askTraceAttrs(message.From.ID, sessionID))
	stopTyping()
	if err != nil {
		b.log(ctx).Error("LLM request failed", zap.Error(err))
		b.sendMessageInThread(ctx, message.Chat.ID, fmt.Sprintf("Ошибка при обращении к ИИ: %v", err), state.MessageThreadID)
		return
	}

	state.Data["history"] = newHistory
	b.sendAskResponse(ctx, message.Chat.ID, answer, state.MessageThreadID)
}

// runAskWithTools executes the tool-calling loop: LLM requests tools, bot executes them, repeats.
// Each call is one Langfuse trace: an agent observation with a generation per LLM call and a
// tool observation per executed tool (siblings under the agent).
func (b *Bot) runAskWithTools(ctx context.Context, history []llm.Message, attrs llm.TraceAttrs) (answer string, _ []llm.Message, err error) {
	ctx, agent := llm.StartAgent(ctx, askTraceName, attrs, lastUserMessage(history))
	iterations, hitLimit := 0, false
	defer func() {
		agent.SetMetadata("tool_iterations", iterations)
		agent.SetMetadata("hit_iteration_limit", hitLimit)
		if err != nil {
			agent.Fail(err)
		} else {
			agent.SetOutput(answer)
		}
		agent.End()
	}()

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

		iterations++
		// LLM wants to call tools — execute them
		b.log(ctx).Debug("LLM requested tool calls",
			zap.Int("count", len(resp.ToolCalls)),
			zap.Int("iteration", i+1),
		)

		// Append the raw assistant message (preserves thought_signature for Gemini)
		history = append(history, resp.AssistantMessage)

		// Execute each tool and append results
		for _, tc := range resp.ToolCalls {
			result := b.runTracedTool(ctx, tc)
			history = append(history, llm.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})
		}
	}

	hitLimit = true
	return iterationLimitAnswer, history, nil
}

// runTracedTool executes a tool call inside a Langfuse tool observation.
func (b *Bot) runTracedTool(ctx context.Context, tc llm.ToolCall) string {
	ctx, obs := llm.StartTool(ctx, tc.Function.Name, tc.Function.Arguments)
	defer obs.End()
	obs.SetMetadata("tool_call_id", tc.ID)
	result := b.executeTool(ctx, tc.Function.Name, tc.Function.Arguments)
	obs.SetOutput(result)
	// Tools report failures as "error: ..." strings so the LLM can react to them.
	if msg, ok := strings.CutPrefix(result, "error: "); ok {
		obs.Fail(fmt.Errorf("%s", msg))
	}
	return result
}

func lastUserMessage(history []llm.Message) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "user" {
			return history[i].Content
		}
	}
	return ""
}

// executeTool runs a tool by name and returns the result as a string.
func (b *Bot) executeTool(ctx context.Context, name, argsJSON string) string {
	b.log(ctx).Debug("Executing tool", zap.String("name", name), zap.String("args", argsJSON))

	var args map[string]any
	if argsJSON != "" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return fmt.Sprintf("error: invalid arguments: %v", err)
		}
	}

	participant := ""
	if raw := stringArg(args, "participant", ""); raw != "" {
		resolved, errStr := b.resolveParticipant(ctx, raw)
		if errStr != "" {
			return errStr
		}
		participant = resolved
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
		return b.toolGetLastEvents(ctx, limit, since, until, participant)
	case "get_top_books":
		days := intArg(args, "days", 30)
		limit := intArg(args, "limit", 10)
		return b.toolGetTopBooks(ctx, days, participant, limit)
	case "get_rarely_read_books":
		label := stringArg(args, "label", "")
		limit := intArg(args, "limit", 10)
		var childrenOnly bool
		switch stringArg(args, "readers", readersAll) {
		case readersAll:
		case readersChildren:
			childrenOnly = true
		default:
			return "error: readers must be one of: all, children"
		}
		return b.toolGetRarelyReadBooks(ctx, label, limit, childrenOnly)
	case "get_labels":
		return b.toolGetLabels(ctx)
	case "get_detailed_book_stats":
		since := stringArg(args, "since", "")
		until := stringArg(args, "until", "")
		book := stringArg(args, "book", "")
		return b.toolGetDetailedBookStats(ctx, since, until, book, participant)
	case "get_participant_stats":
		since := stringArg(args, "since", "")
		until := stringArg(args, "until", "")
		book := stringArg(args, "book", "")
		return b.toolGetParticipantStats(ctx, since, until, book, participant)
	case "get_unread_books":
		return b.toolGetUnreadBooks(ctx,
			stringArg(args, "since", ""),
			stringArg(args, "until", ""),
			participant,
			strings.TrimSpace(stringArg(args, "label", "")),
			stringArg(args, "readers", readersAll))
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
	sinceDate, untilDate, errStr := parseDateRange(since, until)
	if errStr != "" {
		return errStr
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

// toolGetRarelyReadBooks lists read books by staleness. Never-read books are left to get_unread_books:
// storage sorts them last, so they would only show up with a large limit.
func (b *Bot) toolGetRarelyReadBooks(ctx context.Context, label string, limit int, childrenOnly bool) string {
	stats, err := b.db.GetRarelyReadBooks(ctx, limit, childrenOnly, label, nil)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	var sb strings.Builder
	n, hasUnread := 0, false
	for _, s := range stats {
		if s.LastReadDate == nil {
			hasUnread = true
			continue
		}
		n++
		sb.WriteString(fmt.Sprintf("%d. %s — последнее чтение: %s (%d дн. назад)\n",
			n, s.BookName, s.LastReadDate.Format("2006-01-02"), s.DaysSinceLastRead))
	}
	if n == 0 && !hasUnread {
		sb.WriteString("(нет данных)\n")
	}
	if hasUnread {
		sb.WriteString("Есть книги, которые ни разу не читали, — см. get_unread_books.\n")
	}
	return sb.String()
}

const (
	readersAll      = "all"
	readersChildren = "children"
	readersParents  = "parents"
)

// resolveParticipant maps a user-supplied name to the stored one, case-insensitively, so a wrong-case name
// doesn't silently look like "nobody read it".
func (b *Bot) resolveParticipant(ctx context.Context, name string) (string, string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ""
	}
	participants, err := b.db.ListParticipants(ctx)
	if err != nil {
		return "", fmt.Sprintf("error: %v", err)
	}
	known := make([]string, 0, len(participants))
	for _, p := range participants {
		if strings.EqualFold(p.Name, name) {
			return p.Name, ""
		}
		known = append(known, p.Name)
	}
	sort.Strings(known)
	return "", fmt.Sprintf("error: unknown participant %q; known: %s", name, strings.Join(known, ", "))
}

func (b *Bot) toolGetUnreadBooks(ctx context.Context, since, until, participant, label, readers string) string {
	var who string
	switch readers {
	case readersAll:
		who = "все"
	case readersChildren:
		who = "дети"
	case readersParents:
		who = "родители"
	default:
		return "error: readers must be one of: all, children, parents"
	}
	sinceDate, untilDate, errStr := parseDateRange(since, until)
	if errStr != "" {
		return errStr
	}

	participants, err := b.db.ListParticipants(ctx)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	counted := map[string]bool{}
	if participant != "" {
		counted[participant] = true
		who = participant
		if readers != readersAll {
			who += " (readers проигнорирован)"
		}
	} else {
		for _, p := range participants {
			if readers == readersAll || (readers == readersParents) == p.IsParent {
				counted[p.Name] = true
			}
		}
	}
	// With nobody to count, every book would look unread (or none, given the CROSS JOIN) — refuse instead.
	if len(counted) == 0 {
		return fmt.Sprintf("error: нет участников для readers=%s", readers)
	}

	books, err := b.db.ListReadableBooks(ctx)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	stats, err := b.db.GetDetailedBookStats(ctx, sinceDate, untilDate, "", participant)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	reads := map[string]int{}
	for _, s := range stats {
		if counted[s.ParticipantName] {
			reads[s.BookName] += s.ReadCount
		}
	}

	total := 0
	var unread []string
	for _, book := range books {
		if label != "" && !slices.Contains(book.Labels, label) {
			continue
		}
		total++
		if reads[book.Name] == 0 {
			unread = append(unread, book.Name)
		}
	}
	if total == 0 {
		if label != "" {
			return fmt.Sprintf("Нет книг с меткой «%s»\n", label)
		}
		return "(нет книг)\n"
	}
	sort.Strings(unread)

	scope := fmt.Sprintf("кто: %s; период: %s", who, describePeriod(since, until))
	if label != "" {
		scope += "; метка: " + label
	}
	if len(unread) == 0 {
		return fmt.Sprintf("Все %d книг читались хотя бы раз (%s)\n", total, scope)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Ни разу не читали (%s): %d из %d\n", scope, len(unread), total)
	for i, name := range unread {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, name)
	}
	return sb.String()
}

func describePeriod(since, until string) string {
	switch {
	case since == "" && until == "":
		return "всё время"
	case until == "":
		return "с " + since
	case since == "":
		return "по " + until
	default:
		return since + " — " + until
	}
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

	if strings.TrimSpace(answer) == "" {
		answer = "ИИ не вернул ответ. Попробуйте переформулировать вопрос."
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
		b.log(ctx).Warn("Failed to send HTML message, retrying as plain text", zap.Error(err))
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

func parseDateRange(since, until string) (sinceDate, untilDate time.Time, errStr string) {
	if since != "" {
		var err error
		sinceDate, err = time.Parse("2006-01-02", since)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Sprintf("error: invalid 'since' date format %q, expected YYYY-MM-DD", since)
		}
	}
	if until != "" {
		var err error
		untilDate, err = time.Parse("2006-01-02", until)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Sprintf("error: invalid 'until' date format %q, expected YYYY-MM-DD", until)
		}
		untilDate = untilDate.Add(24*time.Hour - time.Second)
	}
	return sinceDate, untilDate, ""
}

func (b *Bot) toolGetDetailedBookStats(ctx context.Context, since, until, book, participant string) string {
	sinceDate, untilDate, errStr := parseDateRange(since, until)
	if errStr != "" {
		return errStr
	}

	stats, err := b.db.GetDetailedBookStats(ctx, sinceDate, untilDate, book, participant)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	var sb strings.Builder
	currentBook := ""
	for _, s := range stats {
		if s.ReadCount == 0 || s.LastReadDate == nil {
			continue
		}
		if s.BookName != currentBook {
			if currentBook != "" {
				sb.WriteString("\n")
			}
			sb.WriteString(fmt.Sprintf("📚 %s:\n", s.BookName))
			currentBook = s.BookName
		}
		sb.WriteString(fmt.Sprintf("  %s — %d раз, последний: %s\n",
			s.ParticipantName, s.ReadCount, s.LastReadDate.Format("2006-01-02")))
	}
	if sb.Len() == 0 {
		return "(нет данных)\n"
	}
	return sb.String()
}

func (b *Bot) toolGetParticipantStats(ctx context.Context, since, until, book, participant string) string {
	sinceDate, untilDate, errStr := parseDateRange(since, until)
	if errStr != "" {
		return errStr
	}

	stats, err := b.db.GetParticipantStats(ctx, sinceDate, untilDate, book, participant)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	var sb strings.Builder
	currentParticipant := ""
	for _, s := range stats {
		if s.ReadCount == 0 {
			continue
		}
		if s.ParticipantName != currentParticipant {
			if currentParticipant != "" {
				sb.WriteString("\n")
			}
			sb.WriteString(fmt.Sprintf("👤 %s:\n", s.ParticipantName))
			currentParticipant = s.ParticipantName
		}
		sb.WriteString(fmt.Sprintf("  %s — %d раз\n", s.BookName, s.ReadCount))
	}
	if sb.Len() == 0 {
		return "(нет данных)\n"
	}
	return sb.String()
}
