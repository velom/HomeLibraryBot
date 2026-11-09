package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/zap"

	"library/internal/llm"
)

// recordSpans installs a recording global tracer provider and propagator for the test.
func recordSpans(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	prevTP, prevProp := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)))
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
	})
	return rec
}

func attrMap(s sdktrace.ReadOnlySpan) map[attribute.Key]attribute.Value {
	m := map[attribute.Key]attribute.Value{}
	for _, kv := range s.Attributes() {
		m[kv.Key] = kv.Value
	}
	return m
}

func spansNamed(rec *tracetest.SpanRecorder, name string) []sdktrace.ReadOnlySpan {
	var out []sdktrace.ReadOnlySpan
	for _, s := range rec.Ended() {
		if s.Name() == name {
			out = append(out, s)
		}
	}
	return out
}

var testAttrs = llm.TraceAttrs{
	SessionID:   "ask-abc",
	UserID:      "42",
	TraceName:   "answer-question",
	Environment: "test",
	Tags:        []string{"ask"},
}

const toolCallResponse = `{
	"model": "test-model-001",
	"choices": [{"message": {"role": "assistant", "content": "", "tool_calls": [
		{"id": "call_1", "type": "function", "function": {"name": "get_books", "arguments": "{}"}}
	]}}],
	"usage": {"prompt_tokens": 100, "completion_tokens": 20, "total_tokens": 120,
		"prompt_tokens_details": {"cached_tokens": 30},
		"completion_tokens_details": {"reasoning_tokens": 5}}
}`

func serve(t *testing.T, status int, body string, inspect func(*http.Request)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if inspect != nil {
			inspect(r)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestChatWithToolsRecordsGeneration(t *testing.T) {
	rec := recordSpans(t)
	srv := serve(t, http.StatusOK, toolCallResponse, nil)
	client := newTestClient(t, srv.URL)

	ctx, agent := llm.StartAgent(context.Background(), "answer-question", testAttrs, "how many books?")
	_, err := client.ChatWithTools(ctx, []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "how many books?"},
	}, []llm.Tool{{Type: "function", Function: llm.ToolFunction{Name: "get_books", Parameters: json.RawMessage(`{}`)}}})
	require.NoError(t, err)
	agent.End()

	gens := spansNamed(rec, "generate-response")
	require.Len(t, gens, 1)
	gen := gens[0]
	a := attrMap(gen)
	assert.Equal(t, "generation", a["langfuse.observation.type"].AsString())
	assert.Equal(t, "test-model", a["langfuse.observation.model.name"].AsString())
	assert.JSONEq(t, `{"input":70,"input_cached_tokens":30,"output":15,"output_reasoning_tokens":5,"total":120}`,
		a["langfuse.observation.usage_details"].AsString())
	assert.JSONEq(t, `[{"role":"system","content":"sys"},{"role":"user","content":"how many books?"}]`,
		a["langfuse.observation.input"].AsString())
	assert.Contains(t, a["langfuse.observation.output"].AsString(), `"tool_calls"`)
	assert.Equal(t, "get_books", a["langfuse.observation.metadata.tools"].AsString())
	assert.Equal(t, "test-model-001", a["langfuse.observation.metadata.response_model"].AsString())
	assert.Equal(t, "ask-abc", a["langfuse.session.id"].AsString())
	assert.Equal(t, "42", a["langfuse.user.id"].AsString())
	assert.Equal(t, []string{"ask"}, a["langfuse.trace.tags"].AsStringSlice())

	agents := spansNamed(rec, "answer-question")
	require.Len(t, agents, 1)
	assert.Equal(t, agents[0].SpanContext().SpanID(), gen.Parent().SpanID(), "generation nests under the agent")
	assert.Equal(t, "agent", attrMap(agents[0])["langfuse.observation.type"].AsString())
}

func TestChatWithToolsNoUsage(t *testing.T) {
	rec := recordSpans(t)
	srv := serve(t, http.StatusOK, `{"choices":[{"message":{"role":"assistant","content":"hi"}}]}`, nil)

	_, err := newTestClient(t, srv.URL).ChatWithTools(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}, nil)
	require.NoError(t, err)

	gens := spansNamed(rec, "generate-response")
	require.Len(t, gens, 1)
	_, ok := attrMap(gens[0])["langfuse.observation.usage_details"]
	assert.False(t, ok)
}

func TestChatWithToolsErrorMarksGeneration(t *testing.T) {
	rec := recordSpans(t)
	srv := serve(t, http.StatusInternalServerError, `{"error":"boom"}`, nil)

	_, err := newTestClient(t, srv.URL).ChatWithTools(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}, nil)
	require.Error(t, err)

	gens := spansNamed(rec, "generate-response")
	require.Len(t, gens, 1)
	assert.Equal(t, codes.Error, gens[0].Status().Code)
	assert.Equal(t, "ERROR", attrMap(gens[0])["langfuse.observation.level"].AsString())
}

func TestGatewayModeSkipsGenerationAndSendsContext(t *testing.T) {
	rec := recordSpans(t)
	var got http.Header
	srv := serve(t, http.StatusOK, toolCallResponse, func(r *http.Request) { got = r.Header.Clone() })
	client := llm.NewClient(llm.Config{BaseURL: srv.URL, APIKey: "gw-key", Model: "openai-prod:gpt-test", Gateway: true}, zap.NewNop())

	attrs := testAttrs
	attrs.Tags = []string{"ask", "a,b"}
	ctx, agent := llm.StartAgent(context.Background(), "answer-question", attrs, "q")
	_, err := client.ChatWithTools(ctx, []llm.Message{{Role: "user", Content: "q"}}, nil)
	require.NoError(t, err)
	agent.End()

	assert.Empty(t, spansNamed(rec, "generate-response"), "gateway records the generation; the client must not duplicate it")

	sc := spansNamed(rec, "answer-question")[0].SpanContext()
	assert.Equal(t, "00-"+sc.TraceID().String()+"-"+sc.SpanID().String()+"-01", got.Get("traceparent"),
		"gateway generation must nest under the agent span, not the HTTP client span")
	assert.Equal(t, "ask-abc", got.Get("langfuse-session-id"))
	assert.Equal(t, "42", got.Get("langfuse-user-id"))
	assert.Equal(t, "answer-question", got.Get("langfuse-trace-name"))
	assert.Equal(t, "test", got.Get("langfuse-environment"))
	assert.Equal(t, "ask,a%2Cb", got.Get("langfuse-tags"))
	assert.Equal(t, "Bearer gw-key", got.Get("Authorization"))
}

func TestNoTraceContextToProvider(t *testing.T) {
	recordSpans(t)
	var got http.Header
	srv := serve(t, http.StatusOK, toolCallResponse, func(r *http.Request) { got = r.Header.Clone() })

	ctx, agent := llm.StartAgent(context.Background(), "answer-question", testAttrs, "q")
	defer agent.End()
	_, err := newTestClient(t, srv.URL).ChatWithTools(ctx, []llm.Message{{Role: "user", Content: "q"}}, nil)
	require.NoError(t, err)

	assert.Empty(t, got.Get("traceparent"))
	assert.Empty(t, got.Get("baggage"))
	assert.Empty(t, got.Get("langfuse-session-id"))
}

func TestUsageClampsInconsistentDetails(t *testing.T) {
	rec := recordSpans(t)
	srv := serve(t, http.StatusOK, `{"choices":[{"message":{"role":"assistant","content":"x"}}],
		"usage":{"prompt_tokens":10,"completion_tokens":2,"prompt_tokens_details":{"cached_tokens":50}}}`, nil)

	_, err := newTestClient(t, srv.URL).ChatWithTools(context.Background(), []llm.Message{{Role: "user", Content: "q"}}, nil)
	require.NoError(t, err)

	assert.JSONEq(t, `{"input":0,"input_cached_tokens":10,"output":2,"total":12}`,
		attrMap(spansNamed(rec, "generate-response")[0])["langfuse.observation.usage_details"].AsString())
}
