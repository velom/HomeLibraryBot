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

	// Lifecycle
	Initialize(ctx context.Context) error
	Close() error
}
