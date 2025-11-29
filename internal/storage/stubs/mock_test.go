package stubs

import (
	"context"
	"testing"
	"time"
)

func TestMockDB_CreateBook(t *testing.T) {
	db := NewMockDB()
	ctx := context.Background()

	// Initialize database
	if err := db.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	// Create a book
	id, err := db.CreateBook(ctx, "Test Book", "Test Author")
	if err != nil {
		t.Fatalf("Failed to create book: %v", err)
	}

	if id == "" {
		t.Fatal("Expected non-empty book ID")
	}

	// Verify book appears in readable books list
	books, err := db.ListReadableBooks(ctx)
	if err != nil {
		t.Fatalf("Failed to list readable books: %v", err)
	}

	if len(books) != 1 {
		t.Errorf("Expected 1 book, got %d", len(books))
	}

	if books[0].Name != "Test Book" || books[0].Author != "Test Author" {
		t.Errorf("Expected 'Test Book' by 'Test Author', got '%s' by '%s'", books[0].Name, books[0].Author)
	}

	if !books[0].IsReadable {
		t.Error("Expected book to be readable by default")
	}
}

func TestMockDB_ListReadableBooks(t *testing.T) {
	db := NewMockDB()
	ctx := context.Background()

	if err := db.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	// Create multiple books
	_, _ = db.CreateBook(ctx, "Book A", "Author 1")
	_, _ = db.CreateBook(ctx, "Book B", "Author 2")
	_, _ = db.CreateBook(ctx, "Book C", "Author 3")

	// List readable books
	books, err := db.ListReadableBooks(ctx)
	if err != nil {
		t.Fatalf("Failed to list readable books: %v", err)
	}

	if len(books) != 3 {
		t.Errorf("Expected 3 readable books, got %d", len(books))
	}

	// Books should be sorted by name
	expectedNames := []string{"Book A", "Book B", "Book C"}
	for i, book := range books {
		if book.Name != expectedNames[i] {
			t.Errorf("Expected book %d to be '%s', got '%s'", i, expectedNames[i], book.Name)
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
	_, err := db.CreateBook(ctx, "Test Book", "Test Author")
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
	_, err := db.CreateBook(ctx, "Test Book", "Test Author")
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
