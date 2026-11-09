package traced

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"library/internal/storage"
	"library/internal/storage/stubs"
)

type failingLabels struct{ storage.Storage }

func (failingLabels) AddLabelToBook(context.Context, string, string) error {
	return errors.New("boom")
}

func TestWrapRecordsSpans(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)))
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	tracer = otel.Tracer("test")

	ctx := context.Background()
	s := Wrap(failingLabels{stubs.NewMockDB()}, "clickhouse")
	require.NoError(t, s.Initialize(ctx))
	_, err := s.ListParticipants(ctx)
	require.NoError(t, err)
	assert.Error(t, s.AddLabelToBook(ctx, "no such book", "x"))

	spans := rec.Ended()
	require.Len(t, spans, 3)
	assert.Equal(t, "storage.ListParticipants", spans[1].Name())
	assert.Equal(t, "storage.AddLabelToBook", spans[2].Name())
	assert.Equal(t, codes.Error, spans[2].Status().Code)
}
