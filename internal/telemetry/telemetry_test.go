package telemetry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestLoggerAddsTraceFields(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	ctx, span := sdktrace.NewTracerProvider().Tracer("test").Start(context.Background(), "op")
	defer span.End()

	Logger(ctx, logger).Info("hello")

	require.Equal(t, 1, logs.Len())
	fields := logs.All()[0].ContextMap()
	assert.Equal(t, span.SpanContext().TraceID().String(), fields["trace_id"])
	assert.Equal(t, span.SpanContext().SpanID().String(), fields["span_id"])
	assert.NotContains(t, fields, "ctx")
}

func TestLoggerWithoutSpanIsUnchanged(t *testing.T) {
	logger := zap.NewNop()
	assert.Same(t, logger, Logger(context.Background(), logger))
}

func TestSetupDisabledWithoutEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("LANGFUSE_PUBLIC_KEY", "")
	assert.False(t, Enabled())
	assert.False(t, LangfuseEnabled())
	shutdown, err := Setup(context.Background())
	require.NoError(t, err)
	require.NoError(t, shutdown(context.Background()))
}
