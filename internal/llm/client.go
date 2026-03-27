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

// Config holds configuration for the LLM client.
type Config struct {
	BaseURL string
	APIKey  string
	Model   string
}

// Client is a provider-agnostic LLM client using the OpenAI-compatible chat completions API.
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	model      string
	logger     *zap.Logger
}

// NewClient creates a new LLM client with the given configuration.
func NewClient(cfg Config, logger *zap.Logger) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: cfg.BaseURL,
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		logger:  logger,
	}
}

// Message represents a chat message in the OpenAI format.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
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
	Content   string
	ToolCalls []ToolCall
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
func (c *Client) ChatWithTools(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error) {
	reqBody := chatCompletionRequest{
		Model:    c.model,
		Messages: messages,
	}
	if len(tools) > 0 {
		reqBody.Tools = tools
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("llm: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("llm: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm: do request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("llm: read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm: API error status %d: %s", resp.StatusCode, string(respBytes))
	}

	var completion chatCompletionResponse
	if err := json.Unmarshal(respBytes, &completion); err != nil {
		return nil, fmt.Errorf("llm: unmarshal response: %w", err)
	}

	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("llm: no choices in response")
	}

	msg := completion.Choices[0].Message
	return &ChatResponse{
		Content:   msg.Content,
		ToolCalls: msg.ToolCalls,
	}, nil
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
	Message responseMessage `json:"message"`
}

type chatCompletionResponse struct {
	Choices []choice `json:"choices"`
}
