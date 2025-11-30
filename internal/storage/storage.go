package storage

import (
	"context"
	"time"

	"library/internal/models"
)

// Storage defines the interface for data storage operations
type Storage interface {
	// Book operations
	CreateBook(ctx context.Context, name string) (string, error)
	ListReadableBooks(ctx context.Context) ([]models.Book, error)

	// Participant operations
	ListParticipants(ctx context.Context) ([]models.Participant, error)

	// Event operations
	CreateEvent(ctx context.Context, date time.Time, bookName, participantName string) error
	GetLastEvents(ctx context.Context, limit int) ([]models.Event, error)

	// Statistics operations

	// GetTopBooks returns top N books by read count within the specified time period
	// If participantName is empty, returns statistics for all children (IsParent=false)
	// If participantName is provided, returns statistics only for that participant
	GetTopBooks(ctx context.Context, limit int, startDate, endDate time.Time, participantName string) ([]models.BookStat, error)

	// Lifecycle
	Initialize(ctx context.Context) error
	Close() error
}
