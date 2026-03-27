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

		var reqBody map[string]interface{}
		err = json.Unmarshal(body, &reqBody)
		require.NoError(t, err)

		assert.Equal(t, "test-model", reqBody["model"])
		messages, ok := reqBody["messages"].([]interface{})
		require.True(t, ok)
		assert.Len(t, messages, 2)

		systemMsg := messages[0].(map[string]interface{})
		assert.Equal(t, "system", systemMsg["role"])

		userMsg := messages[1].(map[string]interface{})
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
		// Never responds — but context should cancel before this matters
		<-r.Context().Done()
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := client.Ask(ctx, "system", "user")
	require.Error(t, err)
}
