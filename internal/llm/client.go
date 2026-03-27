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

// Ask sends a system prompt + user message and returns the LLM's text response.
func (c *Client) Ask(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	return c.Chat(ctx, []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	})
}

// Chat sends a full message history and returns the LLM's text response.
func (c *Client) Chat(ctx context.Context, messages []Message) (string, error) {
	reqBody := chatCompletionRequest{
		Model:    c.model,
		Messages: messages,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("llm: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("llm: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm: do request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("llm: read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm: API error status %d: %s", resp.StatusCode, string(respBytes))
	}

	var completion chatCompletionResponse
	if err := json.Unmarshal(respBytes, &completion); err != nil {
		return "", fmt.Errorf("llm: unmarshal response: %w", err)
	}

	if len(completion.Choices) == 0 {
		return "", fmt.Errorf("llm: no choices in response")
	}

	return completion.Choices[0].Message.Content, nil
}

// Message represents a chat Message with a role and content.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type choice struct {
	Message Message `json:"message"`
}

type chatCompletionResponse struct {
	Choices []choice `json:"choices"`
}
