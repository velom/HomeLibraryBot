package stubs

import (
	"context"
	"library/internal/models"
	"sort"
	"sync"
	"time"
)

// MockDB is an in-memory implementation of the Database interface for testing
type MockDB struct {
	mu           sync.RWMutex
	books        map[string]models.Book
	participants map[string]models.Participant
	events       []models.Event
}

// NewMockDB creates a new mock database
func NewMockDB() *MockDB {
	return &MockDB{
		books:        make(map[string]models.Book),
		participants: make(map[string]models.Participant),
		events:       make([]models.Event, 0),
	}
}

// Initialize sets up default participants for testing
func (m *MockDB) Initialize(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Add default test participants
	m.participants["Alice"] = models.Participant{
		Name:     "Alice",
		IsParent: false,
	}
	m.participants["Bob"] = models.Participant{
		Name:     "Bob",
		IsParent: false,
	}
	m.participants["Mom"] = models.Participant{
		Name:     "Mom",
		IsParent: true,
	}
	m.participants["Dad"] = models.Participant{
		Name:     "Dad",
		IsParent: true,
	}

	return nil
}

// CreateBook creates a new book and returns the book name as identifier
func (m *MockDB) CreateBook(ctx context.Context, name string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.books[name] = models.Book{
		Name:       name,
		IsReadable: true,
	}
	return name, nil
}

// ListReadableBooks returns all readable books
func (m *MockDB) ListReadableBooks(ctx context.Context) ([]models.Book, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var books []models.Book
	for _, book := range m.books {
		if book.IsReadable {
			books = append(books, book)
		}
	}

	// Sort by name
	sort.Slice(books, func(i, j int) bool {
		return books[i].Name < books[j].Name
	})

	return books, nil
}

// ListParticipants returns all participants
func (m *MockDB) ListParticipants(ctx context.Context) ([]models.Participant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var participants []models.Participant
	for _, p := range m.participants {
		participants = append(participants, p)
	}

	// Sort by name
	sort.Slice(participants, func(i, j int) bool {
		return participants[i].Name < participants[j].Name
	})

	return participants, nil
}

// CreateEvent creates a new reading event
func (m *MockDB) CreateEvent(ctx context.Context, date time.Time, bookName, participantName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.events = append(m.events, models.Event{
		Date:            date,
		BookName:        bookName,
		ParticipantName: participantName,
	})

	return nil
}

// GetLastEvents returns the last N events
func (m *MockDB) GetLastEvents(ctx context.Context, limit int) ([]models.Event, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Sort events by date descending
	sortedEvents := make([]models.Event, len(m.events))
	copy(sortedEvents, m.events)
	sort.Slice(sortedEvents, func(i, j int) bool {
		return sortedEvents[i].Date.After(sortedEvents[j].Date)
	})

	if limit > len(sortedEvents) {
		limit = len(sortedEvents)
	}

	return sortedEvents[:limit], nil
}

// GetTopBooks returns top N books by read count within the specified time period
// If participantName is empty, returns statistics for all children (IsParent=false)
// If participantName is provided, returns statistics only for that participant
func (m *MockDB) GetTopBooks(ctx context.Context, limit int, startDate, endDate time.Time, participantName string) ([]models.BookStat, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Count books
	bookCounts := make(map[string]int)

	for _, event := range m.events {
		// Filter by date range
		if event.Date.Before(startDate) || event.Date.After(endDate) {
			continue
		}

		// Filter by participant
		if participantName != "" {
			// Specific participant
			if event.ParticipantName != participantName {
				continue
			}
		} else {
			// All children (not parents)
			participant, exists := m.participants[event.ParticipantName]
			if !exists || participant.IsParent {
				continue
			}
		}

		bookCounts[event.BookName]++
	}

	// Convert to slice
	var stats []models.BookStat
	for bookName, count := range bookCounts {
		stats = append(stats, models.BookStat{
			BookName:  bookName,
			ReadCount: count,
		})
	}

	// Sort by count descending, then by name
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].ReadCount != stats[j].ReadCount {
			return stats[i].ReadCount > stats[j].ReadCount
		}
		return stats[i].BookName < stats[j].BookName
	})

	// Limit results
	if limit > 0 && limit < len(stats) {
		stats = stats[:limit]
	}

	return stats, nil
}

// Close does nothing for mock DB
func (m *MockDB) Close() error {
	return nil
}
