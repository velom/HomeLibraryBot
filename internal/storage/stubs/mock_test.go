package stubs

import (
	"context"
	"testing"
	"time"
)

func TestMockDB_CreateBook(t *testing.T) {
	db := NewMockDB()
	ctx := context.Background()

	// Initialize database (initializes with 10 default test books)
	if err := db.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	// Get initial count
	initialBooks, err := db.ListReadableBooks(ctx)
	if err != nil {
		t.Fatalf("Failed to list readable books: %v", err)
	}
	initialCount := len(initialBooks)

	// Create a book
	id, err := db.CreateBook(ctx, "Test Book")
	if err != nil {
		t.Fatalf("Failed to create book: %v", err)
	}

	if id == "" {
		t.Fatal("Expected non-empty book ID")
	}

	// Verify book count increased by 1
	books, err := db.ListReadableBooks(ctx)
	if err != nil {
		t.Fatalf("Failed to list readable books: %v", err)
	}

	if len(books) != initialCount+1 {
		t.Errorf("Expected %d books, got %d", initialCount+1, len(books))
	}

	// Find the created book
	found := false
	for _, book := range books {
		if book.Name == "Test Book" {
			found = true
			if !book.IsReadable {
				t.Error("Expected book to be readable by default")
			}
			break
		}
	}

	if !found {
		t.Error("Expected to find 'Test Book' in the list")
	}
}

func TestMockDB_ListReadableBooks(t *testing.T) {
	db := NewMockDB()
	ctx := context.Background()

	if err := db.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	// Get initial count (should have default test books)
	initialBooks, err := db.ListReadableBooks(ctx)
	if err != nil {
		t.Fatalf("Failed to list readable books: %v", err)
	}
	initialCount := len(initialBooks)

	// Create multiple books
	_, _ = db.CreateBook(ctx, "Book A")
	_, _ = db.CreateBook(ctx, "Book B")
	_, _ = db.CreateBook(ctx, "Book C")

	// List readable books
	books, err := db.ListReadableBooks(ctx)
	if err != nil {
		t.Fatalf("Failed to list readable books: %v", err)
	}

	expectedCount := initialCount + 3
	if len(books) != expectedCount {
		t.Errorf("Expected %d readable books, got %d", expectedCount, len(books))
	}

	// Books should be sorted by name - verify the newly created books are in order
	newBooks := make(map[string]bool)
	for _, book := range books {
		if book.Name == "Book A" || book.Name == "Book B" || book.Name == "Book C" {
			newBooks[book.Name] = true
		}
	}

	if len(newBooks) != 3 {
		t.Errorf("Expected to find 3 newly created books, found %d", len(newBooks))
	}

	// Verify all books are sorted by name
	for i := 0; i < len(books)-1; i++ {
		if books[i].Name > books[i+1].Name {
			t.Error("Expected books to be sorted by name")
			break
		}
	}
}

func TestMockDB_Participants(t *testing.T) {
	db := NewMockDB()
	ctx := context.Background()

	if err := db.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	// Create participants via Initialize (uses default participants in mock)
	// List all participants
	participants, err := db.ListParticipants(ctx)
	if err != nil {
		t.Fatalf("Failed to list participants: %v", err)
	}

	// Mock DB initializes with default participants
	if len(participants) == 0 {
		t.Error("Expected default participants to be initialized")
	}

	// Verify participants are sorted by name
	for i := 0; i < len(participants)-1; i++ {
		if participants[i].Name > participants[i+1].Name {
			t.Error("Expected participants to be sorted by name")
			break
		}
	}
}

func TestMockDB_Events(t *testing.T) {
	db := NewMockDB()
	ctx := context.Background()

	if err := db.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	// Create a book
	_, err := db.CreateBook(ctx, "Test Book")
	if err != nil {
		t.Fatalf("Failed to create book: %v", err)
	}

	// Create events
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)

	if err := db.CreateEvent(ctx, yesterday, "Test Book", "Alice"); err != nil {
		t.Fatalf("Failed to create event: %v", err)
	}

	if err := db.CreateEvent(ctx, now, "Test Book", "Bob"); err != nil {
		t.Fatalf("Failed to create event: %v", err)
	}

	// Get last events
	events, err := db.GetLastEvents(ctx, 10)
	if err != nil {
		t.Fatalf("Failed to get last events: %v", err)
	}

	if len(events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(events))
	}

	// Events should be in reverse chronological order
	if events[0].ParticipantName != "Bob" {
		t.Errorf("Expected first event to be Bob, got %s", events[0].ParticipantName)
	}

	if events[1].ParticipantName != "Alice" {
		t.Errorf("Expected second event to be Alice, got %s", events[1].ParticipantName)
	}
}

func TestMockDB_GetBooksByLabel(t *testing.T) {
	db := NewMockDB()
	ctx := context.Background()

	if err := db.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	// Add labels to some books
	_ = db.AddLabelToBook(ctx, "The Hobbit", "Fantasy")
	_ = db.AddLabelToBook(ctx, "Harry Potter and the Philosopher's Stone", "Fantasy")
	_ = db.AddLabelToBook(ctx, "The Cat in the Hat", "Rhymes")

	// Get books by label "Fantasy"
	books, err := db.GetBooksByLabel(ctx, "Fantasy")
	if err != nil {
		t.Fatalf("Failed to get books by label: %v", err)
	}

	if len(books) != 2 {
		t.Errorf("Expected 2 books with label 'Fantasy', got %d", len(books))
	}

	// Verify sorted by name
	if len(books) == 2 {
		if books[0].Name != "Harry Potter and the Philosopher's Stone" {
			t.Errorf("Expected first book to be Harry Potter, got %s", books[0].Name)
		}
		if books[1].Name != "The Hobbit" {
			t.Errorf("Expected second book to be The Hobbit, got %s", books[1].Name)
		}
	}

	// Get books by label that doesn't exist
	books, err = db.GetBooksByLabel(ctx, "NonExistent")
	if err != nil {
		t.Fatalf("Failed to get books by label: %v", err)
	}

	if len(books) != 0 {
		t.Errorf("Expected 0 books with label 'NonExistent', got %d", len(books))
	}
}

func TestMockDB_GetDetailedBookStats(t *testing.T) {
	db := NewMockDB()
	ctx := context.Background()
	if err := db.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	now := time.Now()
	_ = db.CreateEvent(ctx, now.AddDate(0, 0, -5), "The Hobbit", "Alice")
	_ = db.CreateEvent(ctx, now.AddDate(0, 0, -2), "The Hobbit", "Alice")
	_ = db.CreateEvent(ctx, now.AddDate(0, 0, -1), "The Hobbit", "Bob")

	// All stats (no filters) — should have rows for every book × participant
	stats, err := db.GetDetailedBookStats(ctx, time.Time{}, time.Time{}, "", "")
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}
	participants, _ := db.ListParticipants(ctx)
	books, _ := db.ListReadableBooks(ctx)
	expectedRows := len(books) * len(participants)
	if len(stats) != expectedRows {
		t.Errorf("Expected %d rows (books×participants), got %d", expectedRows, len(stats))
	}

	// Alice + The Hobbit = 2 reads
	for _, s := range stats {
		if s.BookName == "The Hobbit" && s.ParticipantName == "Alice" {
			if s.ReadCount != 2 {
				t.Errorf("Expected Alice read The Hobbit 2 times, got %d", s.ReadCount)
			}
			if s.LastReadDate == nil {
				t.Error("Expected non-nil LastReadDate")
			}
			break
		}
	}

	// Filter by book
	stats, err = db.GetDetailedBookStats(ctx, time.Time{}, time.Time{}, "The Hobbit", "")
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}
	if len(stats) != len(participants) {
		t.Errorf("Expected %d rows for one book, got %d", len(participants), len(stats))
	}

	// Filter by participant
	stats, err = db.GetDetailedBookStats(ctx, time.Time{}, time.Time{}, "", "Alice")
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}
	if len(stats) != len(books) {
		t.Errorf("Expected %d rows for one participant, got %d", len(books), len(stats))
	}

	// Filter by date range (last 3 days only)
	since := now.AddDate(0, 0, -3)
	stats, err = db.GetDetailedBookStats(ctx, since, now, "The Hobbit", "Alice")
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("Expected 1 row, got %d", len(stats))
	}
	if stats[0].ReadCount != 1 {
		t.Errorf("Expected 1 read in last 3 days, got %d", stats[0].ReadCount)
	}

	// Zero-read entries exist
	stats, err = db.GetDetailedBookStats(ctx, time.Time{}, time.Time{}, "The Hobbit", "")
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}
	hasZero := false
	for _, s := range stats {
		if s.ReadCount == 0 {
			hasZero = true
			if s.LastReadDate != nil {
				t.Error("Expected nil LastReadDate for zero-read entry")
			}
			break
		}
	}
	if !hasZero {
		t.Error("Expected at least one participant with 0 reads")
	}
}

func TestMockDB_GetParticipantStats(t *testing.T) {
	db := NewMockDB()
	ctx := context.Background()
	if err := db.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	now := time.Now()
	_ = db.CreateEvent(ctx, now.AddDate(0, 0, -5), "The Hobbit", "Alice")
	_ = db.CreateEvent(ctx, now.AddDate(0, 0, -2), "The Hobbit", "Alice")
	_ = db.CreateEvent(ctx, now.AddDate(0, 0, -1), "Goodnight Moon", "Alice")

	// All stats
	stats, err := db.GetParticipantStats(ctx, time.Time{}, time.Time{}, "", "")
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}
	participants, _ := db.ListParticipants(ctx)
	books, _ := db.ListReadableBooks(ctx)
	if len(stats) != len(books)*len(participants) {
		t.Errorf("Expected %d rows, got %d", len(books)*len(participants), len(stats))
	}

	// Alice + The Hobbit = 2
	for _, s := range stats {
		if s.ParticipantName == "Alice" && s.BookName == "The Hobbit" {
			if s.ReadCount != 2 {
				t.Errorf("Expected 2 reads, got %d", s.ReadCount)
			}
			break
		}
	}

	// Filter by participant — ordered by read_count DESC within participant
	stats, err = db.GetParticipantStats(ctx, time.Time{}, time.Time{}, "", "Alice")
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}
	if len(stats) != len(books) {
		t.Errorf("Expected %d rows for Alice, got %d", len(books), len(stats))
	}
	if len(stats) > 1 && stats[0].ReadCount < stats[1].ReadCount {
		t.Error("Expected stats ordered by read_count DESC within participant")
	}

	// Date range filter
	since := now.AddDate(0, 0, -3)
	stats, err = db.GetParticipantStats(ctx, since, now, "", "Alice")
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}
	for _, s := range stats {
		if s.BookName == "The Hobbit" && s.ReadCount != 1 {
			t.Errorf("Expected 1 read in last 3 days for The Hobbit, got %d", s.ReadCount)
		}
	}

	// Zero reads present
	hasZero := false
	for _, s := range stats {
		if s.ReadCount == 0 {
			hasZero = true
			break
		}
	}
	if !hasZero {
		t.Error("Expected zero-read entries")
	}
}

func TestMockDB_GetLastEvents_Limit(t *testing.T) {
	db := NewMockDB()
	ctx := context.Background()

	if err := db.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	// Create a book
	_, err := db.CreateBook(ctx, "Test Book")
	if err != nil {
		t.Fatalf("Failed to create book: %v", err)
	}

	// Create 5 events
	for i := 0; i < 5; i++ {
		date := time.Now().AddDate(0, 0, -i)
		if err := db.CreateEvent(ctx, date, "Test Book", "Alice"); err != nil {
			t.Fatalf("Failed to create event: %v", err)
		}
	}

	// Get last 3 events
	events, err := db.GetLastEvents(ctx, 3)
	if err != nil {
		t.Fatalf("Failed to get last events: %v", err)
	}

	if len(events) != 3 {
		t.Errorf("Expected 3 events, got %d", len(events))
	}
}
