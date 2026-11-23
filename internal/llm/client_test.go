package llm_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"library/internal/llm"
)

func newTestClient(t *testing.T, serverURL string) *llm.Client {
	t.Helper()
	cfg := llm.Config{
		BaseURL:        serverURL,
		APIKey:         "test-key",
		Model:          "test-model",
		RetryBaseDelay: time.Millisecond,
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

func newRetryClient(t *testing.T, serverURL string, attempts int) *llm.Client {
	t.Helper()
	return llm.NewClient(llm.Config{
		BaseURL: serverURL, APIKey: "k", Model: "m",
		MaxAttempts: attempts, RetryBaseDelay: time.Millisecond,
	}, zap.NewNop())
}

const okReply = `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`

func TestChatWithTools_RetriesTransientErrors(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout} {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if calls.Add(1) < 3 {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":{"message":"busy"}}`))
				return
			}
			_, _ = w.Write([]byte(okReply))
		}))
		resp, err := newRetryClient(t, server.URL, 3).ChatWithTools(context.Background(), []llm.Message{{Role: "user", Content: "q"}}, nil)
		server.Close()
		require.NoError(t, err, status)
		assert.Equal(t, "ok", resp.Content)
		assert.Equal(t, int32(3), calls.Load(), status)
	}
}

func TestChatWithTools_GivesUpAfterMaxAttempts(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"message":"high demand"}}`))
	}))
	defer server.Close()

	_, err := newRetryClient(t, server.URL, 3).ChatWithTools(context.Background(), []llm.Message{{Role: "user", Content: "q"}}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "503")
	assert.Equal(t, int32(3), calls.Load())
}

func TestChatWithTools_DoesNotRetryClientErrors(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound} {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.WriteHeader(status)
		}))
		_, err := newRetryClient(t, server.URL, 3).ChatWithTools(context.Background(), []llm.Message{{Role: "user", Content: "q"}}, nil)
		server.Close()
		require.Error(t, err)
		assert.Equal(t, int32(1), calls.Load(), status)
	}
}

func TestChatWithTools_DoesNotRetryMalformedResponse(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()
	_, err := newRetryClient(t, server.URL, 3).ChatWithTools(context.Background(), []llm.Message{{Role: "user", Content: "q"}}, nil)
	require.Error(t, err)
	assert.Equal(t, int32(1), calls.Load())
}

func TestChatWithTools_StopsRetryingWhenContextDone(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := llm.NewClient(llm.Config{BaseURL: server.URL, APIKey: "k", Model: "m", MaxAttempts: 5, RetryBaseDelay: time.Hour}, zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := client.ChatWithTools(ctx, []llm.Message{{Role: "user", Content: "q"}}, nil)
	require.Error(t, err)
	assert.Less(t, time.Since(start), 5*time.Second)
	assert.Equal(t, int32(1), calls.Load())
}

func TestChatWithTools_RetriesTimeouts(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			select {
			case <-r.Context().Done():
			case <-time.After(300 * time.Millisecond):
			}
			return
		}
		_, _ = w.Write([]byte(okReply))
	}))
	defer server.Close()

	client := llm.NewClient(llm.Config{BaseURL: server.URL, APIKey: "k", Model: "m",
		MaxAttempts: 2, RetryBaseDelay: time.Millisecond, RequestTimeout: 100 * time.Millisecond}, zap.NewNop())
	resp, err := client.ChatWithTools(context.Background(), []llm.Message{{Role: "user", Content: "q"}}, nil)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp.Content)
	assert.Equal(t, int32(2), calls.Load())
}
