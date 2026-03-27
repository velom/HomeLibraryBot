# `/ask` Command — LLM Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `/ask` command that lets users ask natural language questions about their library data via an LLM.

**Architecture:** New `internal/llm/` package with a provider-agnostic client that speaks the OpenAI-compatible chat completions API (no SDK). The Bot gets an optional `*llm.Client` — if not configured, `/ask` replies with a "not configured" message. Data is pre-fetched via existing storage methods and injected into the system prompt.

**Tech Stack:** Go stdlib `net/http` for LLM API calls, `httptest` for testing, Google Gemini free tier (OpenAI-compatible endpoint) as default provider.

---

### Task 1: LLM Client — Tests

**Files:**
- Create: `internal/llm/client_test.go`

- [ ] **Step 1: Create test file with happy path test**

```go
package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestAsk_HappyPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var reqBody chatCompletionRequest
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		require.NoError(t, err)
		assert.Equal(t, "test-model", reqBody.Model)
		assert.Len(t, reqBody.Messages, 2)
		assert.Equal(t, "system", reqBody.Messages[0].Role)
		assert.Equal(t, "You are helpful", reqBody.Messages[0].Content)
		assert.Equal(t, "user", reqBody.Messages[1].Role)
		assert.Equal(t, "What books?", reqBody.Messages[1].Content)

		resp := chatCompletionResponse{
			Choices: []choice{
				{Message: message{Role: "assistant", Content: "Here are your books!"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "test-model",
	}, zap.NewNop())

	result, err := client.Ask(context.Background(), "You are helpful", "What books?")
	require.NoError(t, err)
	assert.Equal(t, "Here are your books!", result)
}
```

- [ ] **Step 2: Add API error test**

Append to the same file:

```go
func TestAsk_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"rate limit exceeded"}}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "test-model",
	}, zap.NewNop())

	_, err := client.Ask(context.Background(), "system", "user")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "429")
}
```

- [ ] **Step 3: Add malformed response test**

```go
func TestAsk_MalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`not json`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "test-model",
	}, zap.NewNop())

	_, err := client.Ask(context.Background(), "system", "user")
	require.Error(t, err)
}
```

- [ ] **Step 4: Add empty choices test**

```go
func TestAsk_EmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatCompletionResponse{Choices: []choice{}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "test-model",
	}, zap.NewNop())

	_, err := client.Ask(context.Background(), "system", "user")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no choices")
}
```

- [ ] **Step 5: Add context cancellation test**

```go
func TestAsk_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until context is cancelled — the client should give up
		<-r.Context().Done()
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "test-model",
	}, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := client.Ask(ctx, "system", "user")
	require.Error(t, err)
}
```

- [ ] **Step 6: Run tests to verify they fail (no implementation yet)**

Run: `go test -v ./internal/llm/...`
Expected: compilation failure — package doesn't exist yet.

---

### Task 2: LLM Client — Implementation

**Files:**
- Create: `internal/llm/client.go`

- [ ] **Step 1: Implement the LLM client**

```go
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
)

type Config struct {
	BaseURL string
	APIKey  string
	Model   string
}

type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	model      string
	logger     *zap.Logger
}

func NewClient(cfg Config, logger *zap.Logger) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    cfg.BaseURL,
		apiKey:     cfg.APIKey,
		model:      cfg.Model,
		logger:     logger,
	}
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
}

type choice struct {
	Message message `json:"message"`
}

type chatCompletionResponse struct {
	Choices []choice `json:"choices"`
}

func (c *Client) Ask(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	reqBody := chatCompletionRequest{
		Model: c.model,
		Messages: []message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMessage},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		c.logger.Error("LLM API error",
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(respBytes)),
		)
		return "", fmt.Errorf("LLM API returned status %d: %s", resp.StatusCode, string(respBytes))
	}

	var chatResp chatCompletionResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("LLM returned no choices in response")
	}

	return chatResp.Choices[0].Message.Content, nil
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test -v ./internal/llm/...`
Expected: all 5 tests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/llm/client.go internal/llm/client_test.go
git commit -m "feat: add LLM client with OpenAI-compatible API"
```

---

### Task 3: System Prompt Builder — Tests

**Files:**
- Create: `internal/bot/ask_test.go`

- [ ] **Step 1: Create test file**

```go
package bot

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"library/internal/models"
)

func TestBuildAskSystemPrompt_WithData(t *testing.T) {
	books := []models.Book{
		{Name: "Колобок", IsReadable: true, Labels: []string{"Сказки", "Детям"}},
		{Name: "Война и мир", IsReadable: false, Labels: nil},
	}
	participants := []models.Participant{
		{Name: "Миша", IsParent: false},
		{Name: "Папа", IsParent: true},
	}
	events := []models.Event{
		{Date: time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC), BookName: "Колобок", ParticipantName: "Миша"},
	}

	prompt := buildAskSystemPrompt(books, participants, events)

	assert.Contains(t, prompt, "Колобок")
	assert.Contains(t, prompt, "Сказки, Детям")
	assert.Contains(t, prompt, "Война и мир")
	assert.Contains(t, prompt, "Миша")
	assert.Contains(t, prompt, "ребёнок")
	assert.Contains(t, prompt, "Папа")
	assert.Contains(t, prompt, "родитель")
	assert.Contains(t, prompt, "2026-03-25")
	assert.Contains(t, prompt, "помощник семейной библиотеки")
}

func TestBuildAskSystemPrompt_EmptyData(t *testing.T) {
	prompt := buildAskSystemPrompt(nil, nil, nil)

	assert.Contains(t, prompt, "помощник семейной библиотеки")
	assert.Contains(t, prompt, "Книги")
	assert.Contains(t, prompt, "Участники")
	assert.Contains(t, prompt, "Последние события")
}

func TestBuildAskSystemPrompt_ContainsDate(t *testing.T) {
	prompt := buildAskSystemPrompt(nil, nil, nil)

	today := time.Now().Format("2006-01-02")
	assert.Contains(t, prompt, today)
}

func TestBuildAskSystemPrompt_BookWithNoLabels(t *testing.T) {
	books := []models.Book{
		{Name: "Книга без меток", IsReadable: true, Labels: nil},
	}
	prompt := buildAskSystemPrompt(books, nil, nil)

	assert.Contains(t, prompt, "Книга без меток")
	// Should not contain label formatting artifacts for books without labels
	assert.True(t, strings.Contains(prompt, "Книга без меток"))
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -v -run TestBuildAsk ./internal/bot/...`
Expected: compilation failure — `buildAskSystemPrompt` doesn't exist yet.

---

### Task 4: System Prompt Builder + `/ask` Handler — Implementation

**Files:**
- Modify: `internal/config/config.go` — Add LLM config fields
- Modify: `internal/bot/types.go` — Add `llmClient` field
- Modify: `internal/bot/constructor.go` — Accept `*llm.Client`
- Modify: `internal/bot/handlers.go` — Add `/ask` case
- Modify: `internal/bot/commands.go` — Add `handleAsk()` and `buildAskSystemPrompt()`
- Modify: `internal/app/app.go` — Initialize LLM client

- [ ] **Step 1: Add LLM config fields to `internal/config/config.go`**

Add these fields to the `Config` struct after the `UseMockDB` field:

```go
	// LLM configuration (optional)
	LLMBaseURL string
	LLMApiKey  string
	LLMModel   string
```

Add this block at the end of `LoadFromEnv()`, before `return config, nil`:

```go
	// LLM configuration (optional)
	config.LLMBaseURL = os.Getenv("LLM_BASE_URL")
	if config.LLMBaseURL == "" {
		config.LLMBaseURL = "https://generativelanguage.googleapis.com/v1beta/openai"
	}
	config.LLMApiKey = os.Getenv("LLM_API_KEY")
	config.LLMModel = os.Getenv("LLM_MODEL")
	if config.LLMModel == "" {
		config.LLMModel = "gemini-2.0-flash"
	}
```

- [ ] **Step 2: Add `llmClient` to Bot struct in `internal/bot/types.go`**

Add import for `"library/internal/llm"` and add this field to the `Bot` struct after `notificationThreadID`:

```go
	llmClient *llm.Client // Optional LLM client for /ask command (nil = disabled)
```

- [ ] **Step 3: Update `NewBot` in `internal/bot/constructor.go`**

Change the `NewBot` signature to accept `*llm.Client`:

```go
func NewBot(token string, db storage.Storage, allowedUserIDs []int64, notificationChatID int64, notificationThreadID int, llmClient *llm.Client, logger *zap.Logger) (*Bot, error) {
```

Add `llmClient` to the `botWrapper` initialization:

```go
	botWrapper := &Bot{
		db:                   db,
		allowedUsers:         allowedUsers,
		states:               make(map[int64]*ConversationState),
		logger:               logger,
		notificationChatID:   notificationChatID,
		notificationThreadID: notificationThreadID,
		llmClient:            llmClient,
	}
```

Add import for `"library/internal/llm"`.

- [ ] **Step 4: Add `/ask` case in `internal/bot/handlers.go`**

In the `handleMessage()` switch statement, add before the `default` case:

```go
		case "ask":
			b.handleAsk(ctx, message)
```

- [ ] **Step 5: Add `handleAsk()` and `buildAskSystemPrompt()` in `internal/bot/commands.go`**

Add to the imports: `"time"` and `"library/internal/models"` (models is already imported by the bot package — check if the import alias is needed; it's `"github.com/go-telegram/bot/models"` that's imported as `models`, so use the full path `libmodels "library/internal/models"` or use the types directly).

Note: The `models` import in `commands.go` refers to `github.com/go-telegram/bot/models`. The internal models are in `library/internal/models`. Since `commands.go` already imports `"github.com/go-telegram/bot/models"`, you need an alias. Use `botmodels "github.com/go-telegram/bot/models"` or add `appmodels "library/internal/models"`. Looking at the existing code, the pattern is to use `models` for the Telegram bot models. So import `appmodels "library/internal/models"`.

Add these functions at the end of `commands.go`:

```go
// handleAsk handles the /ask command for natural language queries
func (b *Bot) handleAsk(ctx context.Context, message *models.Message) {
	// Extract question text after "/ask"
	question := strings.TrimSpace(strings.TrimPrefix(message.Text, "/ask"))
	// Also handle "/ask@botname" format
	if strings.HasPrefix(question, "@") {
		if idx := strings.Index(question, " "); idx != -1 {
			question = strings.TrimSpace(question[idx:])
		} else {
			question = ""
		}
	}

	if question == "" {
		b.sendMessageInThread(ctx, message.Chat.ID,
			"Использование: /ask <вопрос>\nПример: /ask Какие книги читали на этой неделе?",
			message.MessageThreadID)
		return
	}

	if b.llmClient == nil {
		b.sendMessageInThread(ctx, message.Chat.ID,
			"Функция /ask не настроена.",
			message.MessageThreadID)
		return
	}

	// Fetch data context
	books, err := b.db.ListReadableBooks(ctx)
	if err != nil {
		b.logger.Error("Failed to list books for /ask", zap.Error(err))
		b.sendMessageInThread(ctx, message.Chat.ID, "Ошибка при получении данных.", message.MessageThreadID)
		return
	}

	participants, err := b.db.ListParticipants(ctx)
	if err != nil {
		b.logger.Error("Failed to list participants for /ask", zap.Error(err))
		b.sendMessageInThread(ctx, message.Chat.ID, "Ошибка при получении данных.", message.MessageThreadID)
		return
	}

	events, err := b.db.GetLastEvents(ctx, 50)
	if err != nil {
		b.logger.Error("Failed to get events for /ask", zap.Error(err))
		b.sendMessageInThread(ctx, message.Chat.ID, "Ошибка при получении данных.", message.MessageThreadID)
		return
	}

	systemPrompt := buildAskSystemPrompt(books, participants, events)

	b.logger.Info("Calling LLM for /ask",
		zap.Int64("user_id", message.From.ID),
		zap.String("question", question),
	)

	answer, err := b.llmClient.Ask(ctx, systemPrompt, question)
	if err != nil {
		b.logger.Error("LLM request failed", zap.Error(err))
		b.sendMessageInThread(ctx, message.Chat.ID, "Ошибка при обращении к ИИ. Попробуйте позже.", message.MessageThreadID)
		return
	}

	// Telegram message limit is 4096 characters
	if len(answer) > 4096 {
		answer = answer[:4093] + "..."
	}

	b.sendMessageInThread(ctx, message.Chat.ID, answer, message.MessageThreadID)
}

// buildAskSystemPrompt builds a Russian-language system prompt with library data context
func buildAskSystemPrompt(books []appmodels.Book, participants []appmodels.Participant, events []appmodels.Event) string {
	var sb strings.Builder

	sb.WriteString("Ты — помощник семейной библиотеки. Отвечай на русском языке. Будь кратким и полезным.\n")
	sb.WriteString("Используй только предоставленные данные. Если данных недостаточно для ответа, так и скажи.\n\n")
	sb.WriteString(fmt.Sprintf("Сегодняшняя дата: %s\n\n", time.Now().Format("2006-01-02")))

	sb.WriteString("== Книги ==\n")
	for _, book := range books {
		line := book.Name
		if len(book.Labels) > 0 {
			line += fmt.Sprintf(" [метки: %s]", strings.Join(book.Labels, ", "))
		}
		if book.IsReadable {
			line += " — доступна для чтения"
		} else {
			line += " — не доступна"
		}
		sb.WriteString(line + "\n")
	}
	if len(books) == 0 {
		sb.WriteString("(нет книг)\n")
	}

	sb.WriteString("\n== Участники ==\n")
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

	sb.WriteString("\n== Последние события (чтение) ==\n")
	for _, e := range events {
		sb.WriteString(fmt.Sprintf("%s — %s выбрал(а) \"%s\"\n", e.Date.Format("2006-01-02"), e.ParticipantName, e.BookName))
	}
	if len(events) == 0 {
		sb.WriteString("(нет событий)\n")
	}

	return sb.String()
}
```

- [ ] **Step 6: Update `internal/app/app.go` to initialize LLM client**

Add import: `"library/internal/llm"`

In `initBot()`, before the `bot.NewBot(...)` call, add:

```go
	// Initialize LLM client if configured
	var llmClient *llm.Client
	if a.config.LLMApiKey != "" {
		llmClient = llm.NewClient(llm.Config{
			BaseURL: a.config.LLMBaseURL,
			APIKey:  a.config.LLMApiKey,
			Model:   a.config.LLMModel,
		}, a.logger)
		a.logger.Info("LLM client initialized",
			zap.String("base_url", a.config.LLMBaseURL),
			zap.String("model", a.config.LLMModel),
		)
	} else {
		a.logger.Info("LLM client not configured (LLM_API_KEY not set)")
	}
```

Update the `bot.NewBot(...)` call to pass `llmClient`:

```go
	telegramBot, err := bot.NewBot(a.config.TelegramToken, a.db, a.config.AllowedUserIDs, a.config.NotificationChatID, a.config.NotificationThreadID, llmClient, a.logger)
```

- [ ] **Step 7: Add `/ask` to the `/start` help message in `internal/bot/commands.go`**

Add this line to the `handleStart` help text:

```
/ask - Ask a question about your library (AI)
```

- [ ] **Step 8: Update `.env.example`**

Add at the end:

```
# LLM Configuration (optional - enables /ask command)
# Supports any OpenAI-compatible API (Gemini, OpenAI, Ollama, etc.)
LLM_BASE_URL=https://generativelanguage.googleapis.com/v1beta/openai
LLM_API_KEY=
LLM_MODEL=gemini-2.0-flash
```

- [ ] **Step 9: Run prompt builder tests**

Run: `go test -v -run TestBuildAsk ./internal/bot/...`
Expected: all 4 tests PASS.

- [ ] **Step 10: Run all LLM client tests**

Run: `go test -v ./internal/llm/...`
Expected: all 5 tests PASS.

- [ ] **Step 11: Run full test suite**

Run: `go test -v ./...`
Expected: all tests PASS (including any existing tests — make sure `NewBot` signature change doesn't break anything).

Note: If existing tests call `bot.NewBot(...)`, they need the new `llmClient` parameter. Pass `nil` for tests that don't need LLM.

- [ ] **Step 12: Build**

Run: `make build`
Expected: all binaries build successfully.

- [ ] **Step 13: Commit**

```bash
git add internal/config/config.go internal/bot/types.go internal/bot/constructor.go internal/bot/handlers.go internal/bot/commands.go internal/bot/ask_test.go internal/app/app.go .env.example
git commit -m "feat: add /ask command with LLM integration for natural language queries"
```

---

### Task 5: Fix Any Compilation Issues From NewBot Signature Change

**Files:**
- Modify: any files that call `bot.NewBot(...)` (search the codebase)

- [ ] **Step 1: Search for all `NewBot` call sites**

Run: `grep -rn "NewBot(" internal/ cmd/`

If any callers besides `app.go` exist (e.g., test files or `cmd/library-dev/`), update them to include the new `llmClient` parameter (pass `nil` for tests/dev).

- [ ] **Step 2: Fix all call sites**

For each caller, add `nil` (or an initialized `*llm.Client`) as the new parameter before `logger`.

- [ ] **Step 3: Run full test suite again**

Run: `make test`
Expected: all tests PASS.

- [ ] **Step 4: Build again**

Run: `make build`
Expected: all binaries build successfully.

- [ ] **Step 5: Commit if there were fixes**

```bash
git add -A
git commit -m "fix: update NewBot callers with llmClient parameter"
```
