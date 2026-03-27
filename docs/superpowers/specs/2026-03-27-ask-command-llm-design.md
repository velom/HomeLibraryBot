# `/ask` Command — LLM Integration Design

## Summary

Add a `/ask` command that lets users ask natural language questions about their library data in Telegram. The bot fetches all relevant data, sends it as context to an LLM, and returns the LLM's answer. Uses the OpenAI-compatible chat completions API for provider flexibility (Gemini free tier by default, swappable to OpenAI/Anthropic/Ollama).

## User Flow

```
User: /ask Какие книги читали на этой неделе?
Bot: [fetches books, participants, last 50 events, top stats]
Bot: [builds system prompt with data context + user question]
Bot: [calls LLM via OpenAI-compatible API]
Bot: На этой неделе читали: "Колобок" (выбрал Миша, 25 марта)...
```

- Single-message command, no conversation state.
- If question is empty: reply with usage hint.
- If LLM is not configured: reply "Функция /ask не настроена."
- If LLM returns an error: reply with a generic error message, log details.

## Architecture

### New Package: `internal/llm/`

**`client.go`** — Provider-agnostic LLM client using OpenAI-compatible chat completions API.

```go
type Client struct {
    httpClient *http.Client
    baseURL    string // e.g. "https://generativelanguage.googleapis.com/v1beta/openai"
    apiKey     string
    model      string // e.g. "gemini-2.0-flash"
    logger     *zap.Logger
}

type Config struct {
    BaseURL string
    APIKey  string
    Model   string
}

func NewClient(cfg Config, logger *zap.Logger) *Client

// Ask sends a system prompt + user message and returns the LLM's text response.
func (c *Client) Ask(ctx context.Context, systemPrompt, userMessage string) (string, error)
```

Implementation details:
- POST to `{baseURL}/chat/completions` with `Authorization: Bearer {apiKey}`
- Request body: `{"model": "...", "messages": [{"role":"system","content":"..."},{"role":"user","content":"..."}]}`
- Parse response: extract `choices[0].message.content`
- Timeout: 30 seconds (LLM responses can be slow)
- No streaming — wait for full response.

### Bot Integration

**`internal/bot/types.go`** — Add `llmClient *llm.Client` field to Bot struct.

**`internal/bot/constructor.go`** — Accept optional `*llm.Client` parameter in `NewBot`.

**`internal/bot/handlers.go`** — Add `/ask` case in `handleMessage()` switch.

**`internal/bot/commands.go`** — New `handleAsk()` method:

1. Extract question text (everything after `/ask `).
2. If empty, reply with usage: "Использование: /ask <вопрос>\nПример: /ask Какие книги читали на этой неделе?"
3. If `b.llmClient == nil`, reply "Функция /ask не настроена."
4. Fetch data context via storage:
   - `b.db.ListReadableBooks(ctx)` — all books with labels
   - `b.db.ListParticipants(ctx)` — all participants
   - `b.db.GetLastEvents(ctx, 50)` — last 50 events
5. Build system prompt with `buildAskSystemPrompt(books, participants, events)`.
6. Call `b.llmClient.Ask(ctx, systemPrompt, question)`.
7. If response exceeds 4096 chars, truncate with "..." suffix.
8. Send response via `b.sendMessageInThread()`.

**`buildAskSystemPrompt()`** — Pure function, returns a Russian-language system prompt:

```
Ты — помощник семейной библиотеки. Отвечай на русском языке. Будь кратким и полезным.
Используй только предоставленные данные. Если данных недостаточно для ответа, так и скажи.

Сегодняшняя дата: {today}

== Книги ==
{name} [метки: label1, label2] — readable/not readable
...

== Участники ==
{name} (ребёнок/родитель)
...

== Последние события (чтение) ==
{date} — {participant} выбрал(а) "{book}"
...
```

### Configuration

**`internal/config/config.go`** — Add fields:

```go
LLMBaseURL string // env: LLM_BASE_URL, default: "https://generativelanguage.googleapis.com/v1beta/openai"
LLMApiKey  string // env: LLM_API_KEY, default: "" (disabled)
LLMModel   string // env: LLM_MODEL, default: "gemini-2.0-flash"
```

**`internal/app/app.go`** — Initialize `*llm.Client` if `LLMApiKey` is non-empty, pass to `NewBot`.

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `LLM_BASE_URL` | No | Gemini OpenAI-compat endpoint | OpenAI-compatible API base URL |
| `LLM_API_KEY` | No | (empty = disabled) | API key for the LLM provider |
| `LLM_MODEL` | No | `gemini-2.0-flash` | Model identifier |

## Testing

### `internal/llm/client_test.go`

Tests using `httptest.NewServer` to mock the OpenAI-compatible endpoint:

1. **Happy path** — valid request produces correct response, verify Authorization header, request body structure, model field.
2. **API error** — server returns 4xx/5xx, client returns descriptive error.
3. **Malformed response** — server returns invalid JSON, client handles gracefully.
4. **Empty content** — server returns valid JSON with empty content string.
5. **Timeout** — server hangs, client respects context timeout.

### `internal/bot/commands_test.go` (or `ask_test.go`)

Test `buildAskSystemPrompt()` as a pure function:

1. Verify all books, participants, and events appear in the prompt.
2. Verify today's date is included.
3. Verify labels are formatted correctly.
4. Verify empty data sets produce sensible prompt sections.

## Files Changed

| File | Action | Description |
|------|--------|-------------|
| `internal/llm/client.go` | Create | LLM client with OpenAI-compatible API |
| `internal/llm/client_test.go` | Create | Client tests with httptest |
| `internal/config/config.go` | Modify | Add LLM config fields |
| `internal/bot/types.go` | Modify | Add `llmClient` to Bot struct |
| `internal/bot/constructor.go` | Modify | Accept `*llm.Client` in NewBot |
| `internal/bot/handlers.go` | Modify | Add `/ask` command case |
| `internal/bot/commands.go` | Modify | Add `handleAsk()` and `buildAskSystemPrompt()` |
| `internal/bot/commands_test.go` | Create/Modify | Test `buildAskSystemPrompt()` |
| `internal/app/app.go` | Modify | Initialize LLM client from config |
| `.env.example` | Modify | Add LLM env vars |

## Out of Scope

- Streaming responses
- Conversation memory / follow-up questions
- LLM-generated SQL queries
- Tool-use / function-calling
- Rate limiting (Gemini free tier is 15 RPM, more than enough for a family bot)
- Mini App integration (only Telegram bot command)
