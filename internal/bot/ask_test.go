package bot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/go-telegram/bot/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/zap"

	"library/internal/llm"
)

const (
	llmToolCallReply = `{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[
		{"id":"call_1","type":"function","function":{"name":"get_books","arguments":"{}"}}]}}]}`
	llmTextReply = `{"choices":[{"message":{"role":"assistant","content":"В библиотеке 1 книга."}}]}`
)

func newAskTestBot(t *testing.T, replies func(call int) string) (*Bot, *tracetest.SpanRecorder) {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)))
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(replies(int(calls.Add(1)))))
	}))
	t.Cleanup(srv.Close)

	b, db := newTestBot(t)
	_, err := db.CreateBook(context.Background(), "Dune")
	require.NoError(t, err)
	b.llmClient = llm.NewClient(llm.Config{BaseURL: srv.URL, APIKey: "k", Model: "test-model", Environment: "test"}, zap.NewNop())
	return b, rec
}

func spanAttrs(s sdktrace.ReadOnlySpan) map[attribute.Key]attribute.Value {
	m := map[attribute.Key]attribute.Value{}
	for _, kv := range s.Attributes() {
		m[kv.Key] = kv.Value
	}
	return m
}

func TestRunAskWithToolsTrace(t *testing.T) {
	b, rec := newAskTestBot(t, func(call int) string {
		if call == 1 {
			return llmToolCallReply
		}
		return llmTextReply
	})

	history := []llm.Message{{Role: "system", Content: "sys"}, {Role: "user", Content: "Сколько книг?"}}
	answer, _, err := b.runAskWithTools(context.Background(), history, b.askTraceAttrs(42, "ask-s1"))
	require.NoError(t, err)
	assert.Equal(t, "В библиотеке 1 книга.", answer)

	byName := map[string][]sdktrace.ReadOnlySpan{}
	for _, s := range rec.Ended() {
		byName[s.Name()] = append(byName[s.Name()], s)
	}
	require.Len(t, byName["answer-question"], 1)
	require.Len(t, byName["get_books"], 1)
	require.Len(t, byName["generate-response"], 2)

	agent := byName["answer-question"][0]
	a := spanAttrs(agent)
	assert.Equal(t, "agent", a["langfuse.observation.type"].AsString())
	assert.Equal(t, "Сколько книг?", a["langfuse.observation.input"].AsString())
	assert.Equal(t, "В библиотеке 1 книга.", a["langfuse.observation.output"].AsString())
	assert.Equal(t, "1", a["langfuse.observation.metadata.tool_iterations"].AsString())
	assert.Equal(t, "false", a["langfuse.observation.metadata.hit_iteration_limit"].AsString())

	tool := byName["get_books"][0]
	ta := spanAttrs(tool)
	assert.Equal(t, "tool", ta["langfuse.observation.type"].AsString())
	assert.Contains(t, ta["langfuse.observation.output"].AsString(), "Dune")
	assert.Equal(t, agent.SpanContext().SpanID(), tool.Parent().SpanID(), "tool is a sibling of the generations under the agent")
	for _, g := range byName["generate-response"] {
		assert.Equal(t, agent.SpanContext().SpanID(), g.Parent().SpanID())
	}

	for _, s := range rec.Ended() {
		sa := spanAttrs(s)
		if _, ok := sa["langfuse.observation.type"]; !ok {
			continue // otelhttp spans
		}
		assert.Equal(t, "ask-s1", sa["langfuse.session.id"].AsString(), s.Name())
		assert.Equal(t, "42", sa["langfuse.user.id"].AsString(), s.Name())
		assert.Equal(t, "answer-question", sa["langfuse.trace.name"].AsString(), s.Name())
		assert.Equal(t, "test", sa["langfuse.environment"].AsString(), s.Name())
	}
}

func TestRunAskWithToolsIterationLimit(t *testing.T) {
	b, rec := newAskTestBot(t, func(int) string { return llmToolCallReply })

	answer, _, err := b.runAskWithTools(context.Background(),
		[]llm.Message{{Role: "user", Content: "loop"}}, b.askTraceAttrs(1, "ask-s2"))
	require.NoError(t, err)
	assert.Equal(t, iterationLimitAnswer, answer)

	for _, s := range rec.Ended() {
		if s.Name() == "answer-question" {
			a := spanAttrs(s)
			assert.Equal(t, "true", a["langfuse.observation.metadata.hit_iteration_limit"].AsString())
			assert.Equal(t, iterationLimitAnswer, a["langfuse.observation.output"].AsString())
			return
		}
	}
	t.Fatal("agent span not recorded")
}

func TestRunAskWithToolsLLMErrorFailsAgent(t *testing.T) {
	b, rec := newAskTestBot(t, func(int) string { return `not json` })

	_, _, err := b.runAskWithTools(context.Background(), []llm.Message{{Role: "user", Content: "q"}}, b.askTraceAttrs(1, "ask-s3"))
	require.Error(t, err)

	for _, s := range rec.Ended() {
		if s.Name() == "answer-question" {
			assert.Equal(t, "ERROR", spanAttrs(s)["langfuse.observation.level"].AsString())
			return
		}
	}
	t.Fatal("agent span not recorded")
}

func TestAskSessionPersistsAcrossTurns(t *testing.T) {
	b, rec := newAskTestBot(t, func(int) string { return llmTextReply })
	state := &ConversationState{Command: "ask", Step: 1, Data: map[string]interface{}{
		"history": []llm.Message{{Role: "system", Content: "sys"}}, "session_id": "ask-keep",
	}}
	msg := &models.Message{From: &models.User{ID: 7}, Chat: models.Chat{ID: 7}, Text: "ещё вопрос"}
	b.handleAskConversation(context.Background(), msg, state)
	b.handleAskConversation(context.Background(), msg, state)

	var sessions []string
	for _, s := range rec.Ended() {
		if s.Name() == "answer-question" {
			sessions = append(sessions, spanAttrs(s)["langfuse.session.id"].AsString())
		}
	}
	assert.Equal(t, []string{"ask-keep", "ask-keep"}, sessions)
}
