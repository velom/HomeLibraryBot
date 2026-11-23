package bot

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/zap"

	"library/internal/llm"
	"library/internal/storage/stubs"
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

func TestStatsToolsOmitUnreadBooks(t *testing.T) {
	b, db := newTestBot(t)
	ctx := context.Background()
	require.NoError(t, db.CreateEvent(ctx, time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), "The Hobbit", "Dad"))
	require.NoError(t, db.CreateEvent(ctx, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), "Matilda", "Dad"))

	out := b.toolGetParticipantStats(ctx, "2024-05-01", "", "", "Dad")
	assert.Equal(t, "👤 Dad:\n  The Hobbit — 1 раз\n", out)

	out = b.toolGetDetailedBookStats(ctx, "2024-05-01", "", "", "Dad")
	assert.Equal(t, "📚 The Hobbit:\n  Dad — 1 раз, последний: 2024-06-01\n", out)

	assert.Equal(t, "(нет данных)\n", b.toolGetParticipantStats(ctx, "2025-01-01", "", "", "Dad"))
	assert.Equal(t, "(нет данных)\n", b.toolGetDetailedBookStats(ctx, "2025-01-01", "", "", "Dad"))
}

// seedUnreadScenario reproduces the production failure: some books read only by parents, some never read.
// Mock participants: Alice, Bob (children), Dad, Mom (parents).
func seedUnreadScenario(t *testing.T, db *stubs.MockDB) {
	t.Helper()
	ctx := context.Background()
	d := func(m time.Month, day int) time.Time { return time.Date(2024, m, day, 0, 0, 0, 0, time.UTC) }
	// Read by a child.
	require.NoError(t, db.CreateEvent(ctx, d(3, 1), "The Hobbit", "Alice"))
	require.NoError(t, db.CreateEvent(ctx, d(6, 1), "Matilda", "Bob"))
	// Read only by parents.
	require.NoError(t, db.CreateEvent(ctx, d(5, 10), "Goodnight Moon", "Mom"))
	require.NoError(t, db.CreateEvent(ctx, d(4, 1), "Charlotte's Web", "Dad"))
	// Remaining 6 of the 10 mock books are never read.
	require.NoError(t, db.AddLabelToBook(ctx, "Green Eggs and Ham", "Сами"))
	require.NoError(t, db.AddLabelToBook(ctx, "Goodnight Moon", "Сами"))
}

var neverReadBooks = []string{
	"Charlie and the Chocolate Factory",
	"Green Eggs and Ham",
	"Harry Potter and the Philosopher's Stone",
	"The Cat in the Hat",
	"The Very Hungry Caterpillar",
	"Where the Wild Things Are",
}

func unreadList(names ...string) string {
	var sb strings.Builder
	for i, n := range names {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, n)
	}
	return sb.String()
}

func TestUnreadBooks_AllReaders(t *testing.T) {
	b, db := newTestBot(t)
	seedUnreadScenario(t, db)

	out := b.executeTool(context.Background(), "get_unread_books", `{}`)
	assert.Equal(t, "Ни разу не читали (кто: все; период: всё время): 6 из 10\n"+unreadList(neverReadBooks...), out)
}

func TestUnreadBooks_ChildrenOnlyIncludesParentOnlyBooks(t *testing.T) {
	b, db := newTestBot(t)
	seedUnreadScenario(t, db)

	out := b.executeTool(context.Background(), "get_unread_books", `{"readers":"children"}`)
	assert.Equal(t, "Ни разу не читали (кто: дети; период: всё время): 8 из 10\n"+unreadList(
		"Charlie and the Chocolate Factory", "Charlotte's Web", "Goodnight Moon", "Green Eggs and Ham",
		"Harry Potter and the Philosopher's Stone", "The Cat in the Hat", "The Very Hungry Caterpillar",
		"Where the Wild Things Are"), out)

	out = b.executeTool(context.Background(), "get_unread_books", `{"readers":"parents"}`)
	assert.Contains(t, out, "(кто: родители; период: всё время): 8 из 10\n")
	assert.Contains(t, out, "The Hobbit")
	assert.NotContains(t, out, "Goodnight Moon")
}

func TestUnreadBooks_Period(t *testing.T) {
	b, db := newTestBot(t)
	seedUnreadScenario(t, db)

	// until is inclusive: Goodnight Moon read on 2024-05-10 counts.
	out := b.executeTool(context.Background(), "get_unread_books", `{"since":"2024-05-10","until":"2024-05-10"}`)
	assert.Contains(t, out, "(кто: все; период: 2024-05-10 — 2024-05-10): 9 из 10\n")
	assert.NotContains(t, out, "Goodnight Moon")
	assert.Contains(t, out, "The Hobbit")

	out = b.executeTool(context.Background(), "get_unread_books", `{"since":"2024-05-01"}`)
	assert.Contains(t, out, "(кто: все; период: с 2024-05-01): 8 из 10\n")
}

func TestUnreadBooks_LabelAndParticipant(t *testing.T) {
	b, db := newTestBot(t)
	seedUnreadScenario(t, db)
	ctx := context.Background()

	out := b.executeTool(ctx, "get_unread_books", `{"label":"Сами"}`)
	assert.Equal(t, "Ни разу не читали (кто: все; период: всё время; метка: Сами): 1 из 2\n1. Green Eggs and Ham\n", out)

	// Lower-case name resolves; readers is ignored when participant is set.
	out = b.executeTool(ctx, "get_unread_books", `{"participant":"mom","readers":"children"}`)
	assert.Contains(t, out, "(кто: Mom (readers проигнорирован); период: всё время): 9 из 10\n")
	assert.NotContains(t, out, "Goodnight Moon")
}

func TestUnreadBooks_AllRead(t *testing.T) {
	b, db := newTestBot(t)
	seedUnreadScenario(t, db)

	out := b.executeTool(context.Background(), "get_unread_books", `{"participant":"Mom","label":"Сами","since":"2024-05-01"}`)
	assert.Equal(t, "Ни разу не читали (кто: Mom; период: с 2024-05-01; метка: Сами): 1 из 2\n1. Green Eggs and Ham\n", out)

	out = b.executeTool(context.Background(), "get_unread_books", `{"label":"нет такой"}`)
	assert.Equal(t, "Нет книг с меткой «нет такой»\n", out)

	b2, db2 := newTestBot(t)
	for _, name := range []string{"Harry Potter and the Philosopher's Stone", "The Hobbit", "Charlotte's Web",
		"The Cat in the Hat", "Where the Wild Things Are", "Green Eggs and Ham", "The Very Hungry Caterpillar",
		"Goodnight Moon", "Matilda", "Charlie and the Chocolate Factory"} {
		require.NoError(t, db2.CreateEvent(context.Background(), time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), name, "Dad"))
	}
	assert.Equal(t, "Все 10 книг читались хотя бы раз (кто: все; период: всё время)\n",
		b2.executeTool(context.Background(), "get_unread_books", `{}`))
}

func TestUnreadBooks_Errors(t *testing.T) {
	b, _ := newTestBot(t)
	ctx := context.Background()

	assert.Equal(t, "error: readers must be one of: all, children, parents", b.executeTool(ctx, "get_unread_books", `{"readers":"дети"}`))
	assert.Equal(t, `error: unknown participant "Маша"; known: Alice, Bob, Dad, Mom`, b.executeTool(ctx, "get_unread_books", `{"participant":"Маша"}`))

	empty := &Bot{db: stubs.NewMockDB(), logger: zap.NewNop()}
	_, err := empty.db.CreateBook(ctx, "Dune")
	require.NoError(t, err)
	assert.Equal(t, "error: нет участников для readers=children", empty.executeTool(ctx, "get_unread_books", `{"readers":"children"}`))
	assert.Equal(t, "error: нет участников для readers=all", empty.executeTool(ctx, "get_unread_books", `{}`))
}

func TestRarelyReadBooks_CountsParentsAndPointsToUnread(t *testing.T) {
	b, db := newTestBot(t)
	seedUnreadScenario(t, db)
	ctx := context.Background()

	out := b.executeTool(ctx, "get_rarely_read_books", `{"limit":100}`)
	assert.Contains(t, out, "1. The Hobbit — последнее чтение: 2024-03-01")
	assert.Contains(t, out, "Charlotte's Web — последнее чтение: 2024-04-01")
	assert.Contains(t, out, "Goodnight Moon — последнее чтение: 2024-05-10")
	assert.NotContains(t, out, "никогда")
	assert.True(t, strings.HasSuffix(out, "Есть книги, которые ни разу не читали, — см. get_unread_books.\n"), out)

	out = b.executeTool(ctx, "get_rarely_read_books", `{"limit":100,"readers":"children"}`)
	assert.NotContains(t, out, "Goodnight Moon")
	assert.Contains(t, out, "Matilda")

	assert.Equal(t, "error: readers must be one of: all, children", b.executeTool(ctx, "get_rarely_read_books", `{"readers":"parents"}`))

	// No pointer when every book has been read.
	out = b.executeTool(ctx, "get_rarely_read_books", `{"label":"Сами","limit":1}`)
	assert.Equal(t, "1. Goodnight Moon — последнее чтение: 2024-05-10 ("+strconv.Itoa(int(time.Since(time.Date(2024, 5, 10, 0, 0, 0, 0, time.UTC)).Hours()/24))+" дн. назад)\n", out)
}

func TestParticipantResolutionInExecuteTool(t *testing.T) {
	b, db := newTestBot(t)
	ctx := context.Background()
	require.NoError(t, db.CreateEvent(ctx, time.Now().AddDate(0, 0, -1), "The Hobbit", "Dad"))

	assert.Contains(t, b.executeTool(ctx, "get_detailed_book_stats", `{"participant":" dad "}`), "Dad — 1 раз")
	assert.Contains(t, b.executeTool(ctx, "get_participant_stats", `{"participant":"DAD"}`), "👤 Dad:")
	assert.Contains(t, b.executeTool(ctx, "get_top_books", `{"participant":"dad"}`), "The Hobbit — 1 раз")
	assert.Contains(t, b.executeTool(ctx, "get_last_events", `{"participant":"dad"}`), "Dad | The Hobbit")

	for _, tool := range []string{"get_detailed_book_stats", "get_participant_stats", "get_top_books", "get_last_events"} {
		assert.Equal(t, `error: unknown participant "Маша"; known: Alice, Bob, Dad, Mom`,
			b.executeTool(ctx, tool, `{"participant":"Маша"}`), tool)
	}
}
