package llm_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"library/internal/llm"
)

func newTestClient(t *testing.T, serverURL string) *llm.Client {
	t.Helper()
	cfg := llm.Config{
		BaseURL: serverURL,
		APIKey:  "test-key",
		Model:   "test-model",
	}
	return llm.NewClient(cfg, zap.NewNop())
}

func TestAsk_HappyPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var reqBody map[string]any
		err = json.Unmarshal(body, &reqBody)
		require.NoError(t, err)

		assert.Equal(t, "test-model", reqBody["model"])
		messages, ok := reqBody["messages"].([]any)
		require.True(t, ok)
		assert.Len(t, messages, 2)

		systemMsg := messages[0].(map[string]any)
		assert.Equal(t, "system", systemMsg["role"])

		userMsg := messages[1].(map[string]any)
		assert.Equal(t, "user", userMsg["role"])

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"choices": [
				{
					"message": {
						"role": "assistant",
						"content": "Hello from LLM!"
					}
				}
			]
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.Ask(context.Background(), "You are a helpful assistant.", "Hello!")
	require.NoError(t, err)
	assert.Equal(t, "Hello from LLM!", result)
}

func TestAsk_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error": {"message": "rate limit exceeded", "type": "requests"}}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.Ask(context.Background(), "system", "user")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "429")
}

func TestAsk_MalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.Ask(context.Background(), "system", "user")
	require.Error(t, err)
}

func TestAsk_EmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices": []}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.Ask(context.Background(), "system", "user")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no choices")
}

func TestAsk_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.Ask(ctx, "system", "user")
	require.Error(t, err)
}

func TestChatWithTools_ToolCallResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var reqBody map[string]any
		err = json.Unmarshal(body, &reqBody)
		require.NoError(t, err)

		// Verify tools are sent
		tools, ok := reqBody["tools"].([]any)
		require.True(t, ok)
		assert.Len(t, tools, 1)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{
				"message": {
					"role": "assistant",
					"content": "",
					"tool_calls": [{
						"id": "call_123",
						"type": "function",
						"function": {
							"name": "get_events",
							"arguments": "{\"limit\":10}"
						}
					}]
				}
			}]
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	tools := []llm.Tool{{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "get_events",
			Description: "Get recent events",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"limit":{"type":"integer"}}}`),
		},
	}}

	resp, err := client.ChatWithTools(context.Background(), []llm.Message{
		{Role: "user", Content: "Show events"},
	}, tools)
	require.NoError(t, err)
	assert.True(t, resp.HasToolCalls())
	assert.Len(t, resp.ToolCalls, 1)
	assert.Equal(t, "call_123", resp.ToolCalls[0].ID)
	assert.Equal(t, "get_events", resp.ToolCalls[0].Function.Name)
	assert.Equal(t, `{"limit":10}`, resp.ToolCalls[0].Function.Arguments)
}

func TestChatWithTools_TextResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{
				"message": {
					"role": "assistant",
					"content": "Here is the answer"
				}
			}]
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	resp, err := client.ChatWithTools(context.Background(), []llm.Message{
		{Role: "user", Content: "Hello"},
	}, nil)
	require.NoError(t, err)
	assert.False(t, resp.HasToolCalls())
	assert.Equal(t, "Here is the answer", resp.Content)
}
