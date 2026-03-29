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
	GetBooksByLabel(ctx context.Context, label string) ([]models.Book, error)
	GetAllLabels(ctx context.Context) ([]string, error)

	// Participant operations
	ListParticipants(ctx context.Context) ([]models.Participant, error)

	// Event operations
	CreateEvent(ctx context.Context, date time.Time, bookName, participantName string) error
	GetLastEvents(ctx context.Context, limit int) ([]models.Event, error)
	// GetLastEventsFiltered returns events with optional date range and participant filters.
	// Zero-value times mean no bound. Empty participant means no filter.
	GetLastEventsFiltered(ctx context.Context, limit int, since, until time.Time, participant string) ([]models.Event, error)

	// Statistics operations

	// GetTopBooks returns top N books by read count within the specified time period
	// If participantName is empty, returns statistics for all children (IsParent=false)
	// If participantName is provided, returns statistics only for that participant
	GetTopBooks(ctx context.Context, limit int, startDate, endDate time.Time, participantName string) ([]models.BookStat, error)

	// GetRarelyReadBooks returns books ordered by how long ago they were last read
	// If childrenOnly is true, only considers reads by children (IsParent=false)
	// If childrenOnly is false, considers reads by all participants
	// If label is not empty, only returns books with that label
	// If excludeLabels is not empty, excludes books that have any of the specified labels
	// Books never read are included with DaysSinceLastRead=-1
	GetRarelyReadBooks(ctx context.Context, limit int, childrenOnly bool, label string, excludeLabels []string) ([]models.RareBookStat, error)

	// GetDetailedBookStats returns per-participant reading statistics for books.
	// For each book × participant combination: read count and last read date.
	// Zero-value times mean no date bound. Empty strings mean no filter.
	// Results ordered by book_name ASC, read_count DESC, participant_name ASC.
	GetDetailedBookStats(ctx context.Context, startDate, endDate time.Time, bookName, participantName string) ([]models.DetailedBookStat, error)

	// GetParticipantStats returns per-book reading statistics for participants.
	// For each participant × book combination: read count.
	// Zero-value times mean no date bound. Empty strings mean no filter.
	// Results ordered by participant_name ASC, read_count DESC, book_name ASC.
	GetParticipantStats(ctx context.Context, startDate, endDate time.Time, bookName, participantName string) ([]models.ParticipantBookStat, error)

	// Lifecycle
	Initialize(ctx context.Context) error
	Close() error
}
