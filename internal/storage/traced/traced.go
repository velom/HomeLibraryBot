// Package traced provides a storage.Storage decorator that records a client span per call.
package traced

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"library/internal/models"
	"library/internal/storage"
)

var tracer = otel.Tracer("library/internal/storage")

type Storage struct {
	next   storage.Storage
	system string
}

// Wrap returns s instrumented with spans; system is reported as db.system.name.
func Wrap(s storage.Storage, system string) *Storage {
	return &Storage{next: s, system: system}
}

func (s *Storage) start(ctx context.Context, op string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	attrs = append(attrs, attribute.String("db.system.name", s.system), attribute.String("db.operation.name", op))
	return tracer.Start(ctx, "storage."+op, trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attrs...))
}

func end(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

func observe[T any](s *Storage, ctx context.Context, op string, fn func(context.Context) ([]T, error), attrs ...attribute.KeyValue) ([]T, error) {
	ctx, span := s.start(ctx, op, attrs...)
	res, err := fn(ctx)
	if err == nil {
		span.SetAttributes(attribute.Int("db.response.returned_rows", len(res)))
	}
	end(span, err)
	return res, err
}

func (s *Storage) CreateBook(ctx context.Context, name string) (string, error) {
	ctx, span := s.start(ctx, "CreateBook")
	res, err := s.next.CreateBook(ctx, name)
	end(span, err)
	return res, err
}

func (s *Storage) ListReadableBooks(ctx context.Context) ([]models.Book, error) {
	return observe(s, ctx, "ListReadableBooks", s.next.ListReadableBooks)
}

func (s *Storage) AddLabelToBook(ctx context.Context, bookName, label string) error {
	ctx, span := s.start(ctx, "AddLabelToBook", attribute.String("library.label", label))
	err := s.next.AddLabelToBook(ctx, bookName, label)
	end(span, err)
	return err
}

func (s *Storage) GetBooksWithoutLabel(ctx context.Context, label string) ([]models.Book, error) {
	return observe(s, ctx, "GetBooksWithoutLabel", func(ctx context.Context) ([]models.Book, error) {
		return s.next.GetBooksWithoutLabel(ctx, label)
	}, attribute.String("library.label", label))
}

func (s *Storage) GetBooksByLabel(ctx context.Context, label string) ([]models.Book, error) {
	return observe(s, ctx, "GetBooksByLabel", func(ctx context.Context) ([]models.Book, error) {
		return s.next.GetBooksByLabel(ctx, label)
	}, attribute.String("library.label", label))
}

func (s *Storage) GetAllLabels(ctx context.Context) ([]string, error) {
	return observe(s, ctx, "GetAllLabels", s.next.GetAllLabels)
}

func (s *Storage) ListParticipants(ctx context.Context) ([]models.Participant, error) {
	return observe(s, ctx, "ListParticipants", s.next.ListParticipants)
}

func (s *Storage) CreateEvent(ctx context.Context, date time.Time, bookName, participantName string) error {
	ctx, span := s.start(ctx, "CreateEvent")
	err := s.next.CreateEvent(ctx, date, bookName, participantName)
	end(span, err)
	return err
}

func (s *Storage) GetLastEvents(ctx context.Context, limit int) ([]models.Event, error) {
	return observe(s, ctx, "GetLastEvents", func(ctx context.Context) ([]models.Event, error) {
		return s.next.GetLastEvents(ctx, limit)
	}, attribute.Int("library.limit", limit))
}

func (s *Storage) GetLastEventsFiltered(ctx context.Context, limit int, since, until time.Time, participant string) ([]models.Event, error) {
	return observe(s, ctx, "GetLastEventsFiltered", func(ctx context.Context) ([]models.Event, error) {
		return s.next.GetLastEventsFiltered(ctx, limit, since, until, participant)
	}, attribute.Int("library.limit", limit))
}

func (s *Storage) GetTopBooks(ctx context.Context, limit int, startDate, endDate time.Time, participantName string) ([]models.BookStat, error) {
	return observe(s, ctx, "GetTopBooks", func(ctx context.Context) ([]models.BookStat, error) {
		return s.next.GetTopBooks(ctx, limit, startDate, endDate, participantName)
	}, attribute.Int("library.limit", limit))
}

func (s *Storage) GetRarelyReadBooks(ctx context.Context, limit int, childrenOnly bool, label string, excludeLabels []string) ([]models.RareBookStat, error) {
	return observe(s, ctx, "GetRarelyReadBooks", func(ctx context.Context) ([]models.RareBookStat, error) {
		return s.next.GetRarelyReadBooks(ctx, limit, childrenOnly, label, excludeLabels)
	}, attribute.Int("library.limit", limit), attribute.Bool("library.children_only", childrenOnly), attribute.String("library.label", label))
}

func (s *Storage) GetDetailedBookStats(ctx context.Context, startDate, endDate time.Time, bookName, participantName string) ([]models.DetailedBookStat, error) {
	return observe(s, ctx, "GetDetailedBookStats", func(ctx context.Context) ([]models.DetailedBookStat, error) {
		return s.next.GetDetailedBookStats(ctx, startDate, endDate, bookName, participantName)
	})
}

func (s *Storage) GetParticipantStats(ctx context.Context, startDate, endDate time.Time, bookName, participantName string) ([]models.ParticipantBookStat, error) {
	return observe(s, ctx, "GetParticipantStats", func(ctx context.Context) ([]models.ParticipantBookStat, error) {
		return s.next.GetParticipantStats(ctx, startDate, endDate, bookName, participantName)
	})
}

func (s *Storage) Initialize(ctx context.Context) error {
	ctx, span := s.start(ctx, "Initialize")
	err := s.next.Initialize(ctx)
	end(span, err)
	return err
}

func (s *Storage) Close() error { return s.next.Close() }

var _ storage.Storage = (*Storage)(nil)
