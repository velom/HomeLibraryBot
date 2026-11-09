package telemetry

import (
	"compress/gzip"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

func exportThroughFilter(t *testing.T, build func(tp *sdktrace.TracerProvider)) []tracetest.SpanStub {
	t.Helper()
	mem := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(langfuseFilter{next: mem}))
	build(tp)
	return mem.GetSpans() // read before Shutdown: InMemoryExporter.Shutdown resets its spans
}

func TestLangfuseFilterKeepsOnlyLLMScope(t *testing.T) {
	spans := exportThroughFilter(t, func(tp *sdktrace.TracerProvider) {
		_, s1 := tp.Tracer(LangfuseScope).Start(context.Background(), "answer-question")
		s1.End()
		_, s2 := tp.Tracer("library/internal/bot").Start(context.Background(), "telegram.message /ask")
		s2.End()
	})
	require.Len(t, spans, 1)
	assert.Equal(t, "answer-question", spans[0].Name)
}

func TestLangfuseFilterDetachesRoot(t *testing.T) {
	spans := exportThroughFilter(t, func(tp *sdktrace.TracerProvider) {
		ctx, parent := tp.Tracer("library/internal/bot").Start(context.Background(), "telegram.message /ask")
		ctx, root := tp.Tracer(LangfuseScope).Start(ctx, "answer-question", trace.WithAttributes(LangfuseRootAttr.Bool(true)))
		_, child := tp.Tracer(LangfuseScope).Start(ctx, "get_books")
		child.End()
		root.End()
		parent.End()
	})
	require.Len(t, spans, 2)
	byName := map[string]tracetest.SpanStub{}
	for _, s := range spans {
		byName[s.Name] = s
	}
	root := byName["answer-question"]
	assert.False(t, root.Parent.IsValid())
	for _, kv := range root.Attributes {
		assert.NotEqual(t, LangfuseRootAttr, kv.Key)
	}
	assert.Equal(t, root.SpanContext.SpanID(), byName["get_books"].Parent.SpanID())
}

type otlpCapture struct {
	mu       sync.Mutex
	requests []*http.Request
	bodies   []*collectortrace.ExportTraceServiceRequest
}

func (c *otlpCapture) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/public/otel/v1/traces" && r.URL.Path != "/v1/traces" {
			w.WriteHeader(http.StatusOK) // metrics/logs of the regular OTLP backend
			return
		}
		var body io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			require.NoError(t, err)
			body = gz
		}
		raw, err := io.ReadAll(body)
		require.NoError(t, err)
		req := &collectortrace.ExportTraceServiceRequest{}
		require.NoError(t, proto.Unmarshal(raw, req))
		c.mu.Lock()
		c.requests = append(c.requests, r)
		c.bodies = append(c.bodies, req)
		c.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}
}

func restoreGlobals(t *testing.T) {
	prevTP, prevProp := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
	})
}

func TestSetupLangfuseOnly(t *testing.T) {
	restoreGlobals(t)
	lf := &otlpCapture{}
	srv := httptest.NewServer(lf.handler(t))
	defer srv.Close()

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("LANGFUSE_PUBLIC_KEY", "pk-lf-test")
	t.Setenv("LANGFUSE_SECRET_KEY", "sk-lf-test")
	t.Setenv("LANGFUSE_BASE_URL", srv.URL+"/")

	shutdown, err := Setup(context.Background())
	require.NoError(t, err)
	_, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider)
	require.True(t, ok, "tracer provider must be installed when only Langfuse is configured")

	ctx, parent := otel.Tracer("library/internal/bot").Start(context.Background(), "telegram.message /ask")
	_, root := otel.Tracer(LangfuseScope).Start(ctx, "answer-question", trace.WithAttributes(LangfuseRootAttr.Bool(true), attribute.String("langfuse.observation.type", "agent")))
	root.End()
	parent.End()
	require.NoError(t, shutdown(context.Background()))

	require.Len(t, lf.requests, 1)
	r := lf.requests[0]
	assert.Equal(t, "/api/public/otel/v1/traces", r.URL.Path)
	assert.Equal(t, "Basic "+base64.StdEncoding.EncodeToString([]byte("pk-lf-test:sk-lf-test")), r.Header.Get("Authorization"))
	assert.Equal(t, "4", r.Header.Get("x-langfuse-ingestion-version"))

	spans := lf.bodies[0].ResourceSpans[0].ScopeSpans
	require.Len(t, spans, 1)
	require.Len(t, spans[0].Spans, 1)
	assert.Equal(t, "answer-question", spans[0].Spans[0].Name)
	assert.Empty(t, spans[0].Spans[0].ParentSpanId, "root observation must be detached from the filtered Telegram span")
}

func TestSetupBothBackendsDoNotShareSettings(t *testing.T) {
	restoreGlobals(t)
	lf, other := &otlpCapture{}, &otlpCapture{}
	lfSrv := httptest.NewServer(lf.handler(t))
	defer lfSrv.Close()
	otherSrv := httptest.NewServer(other.handler(t))
	defer otherSrv.Close()

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", otherSrv.URL)
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "authorization=Bearer clickstack-secret")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_HEADERS", "authorization=Bearer clickstack-secret")
	t.Setenv("OTEL_SDK_DISABLED", "")
	t.Setenv("LANGFUSE_PUBLIC_KEY", "pk-lf-test")
	t.Setenv("LANGFUSE_SECRET_KEY", "sk-lf-test")
	t.Setenv("LANGFUSE_BASE_URL", lfSrv.URL)

	shutdown, err := Setup(context.Background())
	require.NoError(t, err)
	_, s1 := otel.Tracer(LangfuseScope).Start(context.Background(), "answer-question")
	s1.End()
	_, s2 := otel.Tracer("library/internal/bot").Start(context.Background(), "telegram.message /start")
	s2.End()
	require.NoError(t, shutdown(context.Background()))

	for _, r := range lf.requests {
		assert.NotContains(t, r.Header.Get("Authorization"), "clickstack-secret")
	}
	var lfSpans int
	for _, b := range lf.bodies {
		for _, rs := range b.ResourceSpans {
			for _, ss := range rs.ScopeSpans {
				assert.Equal(t, LangfuseScope, ss.Scope.Name)
				lfSpans += len(ss.Spans)
			}
		}
	}
	assert.Equal(t, 1, lfSpans)
	for _, r := range other.requests {
		if r.URL.Path == "/v1/traces" {
			assert.Equal(t, "Bearer clickstack-secret", r.Header.Get("Authorization"))
		}
	}
}
