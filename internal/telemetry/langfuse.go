package telemetry

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// LangfuseScope is the instrumentation scope of spans exported to Langfuse.
// Spans from any other scope (Telegram, HTTP, storage) only go to the regular OTLP backend.
const LangfuseScope = "library/llm"

// LangfuseRootAttr marks the span that becomes the root observation in Langfuse.
// Its real parent (e.g. the Telegram update span) is not exported to Langfuse,
// so the exporter detaches it to avoid an orphaned observation.
const LangfuseRootAttr = attribute.Key("library.langfuse.root")

const defaultLangfuseBaseURL = "https://cloud.langfuse.com"

// LangfuseEnabled reports whether Langfuse export is configured via LANGFUSE_PUBLIC_KEY and LANGFUSE_SECRET_KEY.
func LangfuseEnabled() bool {
	return os.Getenv("LANGFUSE_PUBLIC_KEY") != "" && os.Getenv("LANGFUSE_SECRET_KEY") != ""
}

func langfuseBaseURL() string {
	if u := os.Getenv("LANGFUSE_BASE_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return defaultLangfuseBaseURL
}

// newLangfuseExporter builds an OTLP/HTTP exporter for the Langfuse OTel endpoint.
// Every option the OTEL_EXPORTER_OTLP_* env vars could set is pinned explicitly so
// the settings of the regular OTLP backend never leak into Langfuse traffic.
func newLangfuseExporter(ctx context.Context) (sdktrace.SpanExporter, error) {
	auth := base64.StdEncoding.EncodeToString([]byte(os.Getenv("LANGFUSE_PUBLIC_KEY") + ":" + os.Getenv("LANGFUSE_SECRET_KEY")))
	endpoint := langfuseBaseURL() + "/api/public/otel/v1/traces"
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpointURL(endpoint),
		otlptracehttp.WithHeaders(map[string]string{
			"Authorization":                "Basic " + auth,
			"x-langfuse-ingestion-version": "4",
		}),
		otlptracehttp.WithCompression(otlptracehttp.GzipCompression),
		otlptracehttp.WithTimeout(10 * time.Second),
	}
	if strings.HasPrefix(endpoint, "https://") {
		opts = append(opts, otlptracehttp.WithTLSClientConfig(&tls.Config{MinVersion: tls.VersionTLS12}))
	}
	exp, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return langfuseFilter{next: exp}, nil
}

// langfuseFilter forwards only LangfuseScope spans and detaches Langfuse root spans from their parent.
type langfuseFilter struct {
	next sdktrace.SpanExporter
}

func (f langfuseFilter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	out := make([]sdktrace.ReadOnlySpan, 0, len(spans))
	for _, s := range spans {
		if s.InstrumentationScope().Name != LangfuseScope {
			continue
		}
		if isLangfuseRoot(s) {
			s = detachedSpan{ReadOnlySpan: s}
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil
	}
	return f.next.ExportSpans(ctx, out)
}

func (f langfuseFilter) Shutdown(ctx context.Context) error { return f.next.Shutdown(ctx) }

func isLangfuseRoot(s sdktrace.ReadOnlySpan) bool {
	for _, kv := range s.Attributes() {
		if kv.Key == LangfuseRootAttr {
			return kv.Value.AsBool()
		}
	}
	return false
}

// detachedSpan presents a span without a parent and without the internal root marker.
// The underlying span is shared with other processors, so nothing is mutated.
type detachedSpan struct {
	sdktrace.ReadOnlySpan
}

func (d detachedSpan) Parent() trace.SpanContext { return trace.SpanContext{} }

func (d detachedSpan) Attributes() []attribute.KeyValue {
	attrs := d.ReadOnlySpan.Attributes()
	out := make([]attribute.KeyValue, 0, len(attrs))
	for _, kv := range attrs {
		if kv.Key != LangfuseRootAttr {
			out = append(out, kv)
		}
	}
	return out
}
