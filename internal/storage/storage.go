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
	AddLabelToBook(ctx context.Context, bookName string, label string) error
	GetBooksWithoutLabel(ctx context.Context, label string) ([]models.Book, error)
	GetAllLabels(ctx context.Context) ([]string, error)

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

	// GetRarelyReadBooks returns books ordered by how long ago they were last read
	// If childrenOnly is true, only considers reads by children (IsParent=false)
	// If childrenOnly is false, considers reads by all participants
	// If label is not empty, only returns books with that label
	// Books never read are included with DaysSinceLastRead=-1
	GetRarelyReadBooks(ctx context.Context, limit int, childrenOnly bool, label string) ([]models.RareBookStat, error)

	// Lifecycle
	Initialize(ctx context.Context) error
	Close() error
}
