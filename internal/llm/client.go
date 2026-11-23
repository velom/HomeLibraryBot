package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/propagation"
	"go.uber.org/zap"

	"library/internal/telemetry"
)

// Config holds configuration for the LLM client.
type Config struct {
	BaseURL string
	APIKey  string
	Model   string
	// Gateway marks BaseURL as the Langfuse AI Gateway (OpenAI route). The gateway records
	// every call as a generation itself, so the client does not emit its own generation
	// span; it forwards traceparent and langfuse-* headers so the gateway nests the
	// generation inside the caller's trace.
	Gateway bool
	// Environment is the Langfuse environment of traces (e.g. production, development).
	Environment string
	// MaxAttempts bounds attempts per call on transient failures (429, 5xx, timeouts). Default 3.
	MaxAttempts int
	// RetryBaseDelay is the delay before the first retry; it doubles per retry. Default 1s.
	RetryBaseDelay time.Duration
	// RequestTimeout bounds a single HTTP attempt. Default 30s.
	RequestTimeout time.Duration
}

const (
	defaultMaxAttempts    = 3
	defaultRetryBaseDelay = time.Second
	defaultRequestTimeout = 30 * time.Second
)

// APIError is a non-200 response from the provider.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("llm: API error status %d: %s", e.StatusCode, e.Body)
}

// retryable reports whether a failed attempt is worth repeating: provider overload/rate limits
// and per-attempt timeouts. Caller cancellation and malformed responses are not retried.
func retryable(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	if apiErr, ok := errors.AsType[*APIError](err); ok {
		switch apiErr.StatusCode {
		case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway,
			http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		}
		return false
	}
	if netErr, ok := errors.AsType[net.Error](err); ok && netErr.Timeout() {
		return true
	}
	return false
}

// Client is a provider-agnostic LLM client using the OpenAI-compatible chat completions API.
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	model      string
	gateway    bool
	env        string
	logger     *zap.Logger

	maxAttempts    int
	retryBaseDelay time.Duration
}

// NewClient creates a new LLM client with the given configuration.
func NewClient(cfg Config, logger *zap.Logger) *Client {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = defaultMaxAttempts
	}
	if cfg.RetryBaseDelay <= 0 {
		cfg.RetryBaseDelay = defaultRetryBaseDelay
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = defaultRequestTimeout
	}
	return &Client{
		maxAttempts:    cfg.MaxAttempts,
		retryBaseDelay: cfg.RetryBaseDelay,
		httpClient: &http.Client{
			Timeout: cfg.RequestTimeout,
			// No propagation: trace context must not leak to the LLM provider.
			// Gateway mode injects traceparent explicitly (see injectTraceparent).
			Transport: otelhttp.NewTransport(http.DefaultTransport,
				otelhttp.WithPropagators(propagation.NewCompositeTextMapPropagator())),
		},
		baseURL: cfg.BaseURL,
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		gateway: cfg.Gateway,
		env:     cfg.Environment,
		logger:  logger,
	}
}

// Environment returns the configured Langfuse environment.
func (c *Client) Environment() string { return c.env }

// Message represents a chat message in the OpenAI format.
// For assistant messages with tool calls, RawJSON preserves the full original
// response (including provider-specific fields like Gemini's thought_signature).
type Message struct {
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	RawJSON    json.RawMessage `json:"-"` // not serialized by default; used via custom MarshalJSON
}

// MarshalJSON uses RawJSON verbatim if set (for replaying assistant tool-call messages),
// otherwise falls back to standard struct serialization.
func (m Message) MarshalJSON() ([]byte, error) {
	if m.RawJSON != nil {
		return m.RawJSON, nil
	}
	type plain Message
	return json.Marshal(plain(m))
}

// ToolCall represents a function call requested by the LLM.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall contains the function name and arguments.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool defines a tool available to the LLM.
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction describes a callable function.
type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ChatResponse represents the LLM's response, which may be text or tool calls.
type ChatResponse struct {
	Content          string
	ToolCalls        []ToolCall
	AssistantMessage Message // Full assistant message to append to history (preserves raw JSON for providers like Gemini)
}

// HasToolCalls returns true if the response requests tool calls.
func (r *ChatResponse) HasToolCalls() bool {
	return len(r.ToolCalls) > 0
}

// Ask sends a system prompt + user message and returns the LLM's text response.
func (c *Client) Ask(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	resp, err := c.ChatWithTools(ctx, []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}, nil)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// Chat sends a full message history and returns the LLM's text response.
func (c *Client) Chat(ctx context.Context, messages []Message) (string, error) {
	resp, err := c.ChatWithTools(ctx, messages, nil)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// ChatWithTools sends messages with optional tool definitions and returns a ChatResponse.
// Each call is recorded as a Langfuse generation (unless the gateway records it).
func (c *Client) ChatWithTools(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error) {
	var gen *Observation
	if !c.gateway {
		ctx, gen = startGeneration(ctx, c.model, messages, tools)
		defer gen.End()
	}
	resp, completion, attempts, err := c.chatWithRetry(ctx, messages, tools)
	if attempts > 1 {
		gen.SetMetadata("attempts", attempts)
	}
	if err != nil {
		gen.Fail(err)
		return nil, err
	}
	gen.SetOutput(string(completion.Choices[0].RawMessage))
	gen.setUsage(completion.Usage)
	if completion.Model != "" {
		gen.SetMetadata("response_model", completion.Model)
	}
	return resp, nil
}

// chatWithRetry repeats transient failures with exponential backoff. The whole loop shares one
// generation span, so a retried call is still one Langfuse generation.
func (c *Client) chatWithRetry(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, *chatCompletionResponse, int, error) {
	delay := c.retryBaseDelay
	for attempt := 1; ; attempt++ {
		resp, completion, err := c.chat(ctx, messages, tools)
		if err == nil || attempt >= c.maxAttempts || !retryable(ctx, err) {
			return resp, completion, attempt, err
		}
		telemetry.Logger(ctx, c.logger).Warn("LLM request failed, retrying",
			zap.Int("attempt", attempt), zap.Duration("delay", delay), zap.Error(err))
		select {
		case <-ctx.Done():
			return nil, nil, attempt, err
		case <-time.After(delay):
		}
		delay *= 2
	}
}

func (c *Client) chat(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, *chatCompletionResponse, error) {
	reqBody := chatCompletionRequest{
		Model:    c.model,
		Messages: messages,
	}
	if len(tools) > 0 {
		reqBody.Tools = tools
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, fmt.Errorf("llm: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, nil, fmt.Errorf("llm: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if c.gateway {
		injectTraceparent(ctx, req.Header)
		traceAttrsFrom(ctx).setGatewayHeaders(req.Header)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("llm: do request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("llm: read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, nil, &APIError{StatusCode: resp.StatusCode, Body: string(respBytes)}
	}

	var completion chatCompletionResponse
	if err := json.Unmarshal(respBytes, &completion); err != nil {
		return nil, nil, fmt.Errorf("llm: unmarshal response: %w", err)
	}

	if len(completion.Choices) == 0 {
		return nil, nil, fmt.Errorf("llm: no choices in response")
	}

	msg := completion.Choices[0].Message

	// Build an assistant Message that preserves the raw JSON (for Gemini thought_signature etc.)
	assistantMsg := Message{
		Role:      "assistant",
		Content:   msg.Content,
		ToolCalls: msg.ToolCalls,
		RawJSON:   completion.Choices[0].RawMessage,
	}

	return &ChatResponse{
		Content:          msg.Content,
		ToolCalls:        msg.ToolCalls,
		AssistantMessage: assistantMsg,
	}, &completion, nil
}

type chatCompletionRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Tools    []Tool    `json:"tools,omitempty"`
}

type responseMessage struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

type choice struct {
	Message    responseMessage `json:"message"`
	RawMessage json.RawMessage `json:"-"` // captured during unmarshal
}

// UnmarshalJSON captures the raw "message" JSON for verbatim replay.
func (c *choice) UnmarshalJSON(data []byte) error {
	// First unmarshal into a helper to get the raw message
	var raw struct {
		Message json.RawMessage `json:"message"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.RawMessage = raw.Message

	// Then unmarshal the message into the typed struct
	type plain choice
	return json.Unmarshal(data, (*plain)(c))
}

type chatCompletionResponse struct {
	Model   string   `json:"model"`
	Choices []choice `json:"choices"`
	Usage   *usage   `json:"usage,omitempty"`
}
