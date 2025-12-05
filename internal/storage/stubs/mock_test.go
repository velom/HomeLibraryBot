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
