package ch

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clickhouseTC "github.com/testcontainers/testcontainers-go/modules/clickhouse"
)

// runMigrations manually runs ClickHouse migrations
func runMigrations(ctx context.Context, db *ClickHouseDB) error {
	// Drop existing tables
	_ = db.conn.Exec(ctx, "DROP TABLE IF EXISTS events")
	_ = db.conn.Exec(ctx, "DROP TABLE IF EXISTS participants")
	_ = db.conn.Exec(ctx, "DROP TABLE IF EXISTS books")

	// Create books table
	err := db.conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS books (
			name String,
			is_readable Bool
		) ENGINE = MergeTree()
		ORDER BY name
	`)
	if err != nil {
		return err
	}

	// Create participants table
	err = db.conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS participants (
			name String,
			is_parent Bool
		) ENGINE = MergeTree()
		ORDER BY name
	`)
	if err != nil {
		return err
	}

	// Create events table
	err = db.conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS events (
			date DateTime,
			book_name String,
			participant_name String
		) ENGINE = MergeTree()
		ORDER BY date
	`)
	return err
}

// setupTestDB creates a test ClickHouse instance using testcontainers
func setupTestDB(t *testing.T) (*ClickHouseDB, func()) {
	ctx := context.Background()

	// Start ClickHouse container
	clickhouseContainer, err := clickhouseTC.Run(ctx,
		"clickhouse/clickhouse-server:24.3.3.102-alpine",
		clickhouseTC.WithUsername("default"),
		clickhouseTC.WithPassword(""),
		clickhouseTC.WithDatabase("default"),
	)
	require.NoError(t, err, "Failed to start ClickHouse container")

	// Get connection details
	host, err := clickhouseContainer.Host(ctx)
	require.NoError(t, err)

	port, err := clickhouseContainer.MappedPort(ctx, "9000/tcp")
	require.NoError(t, err)

	// Create database connection
	db, err := NewClickHouseDB(host, port.Int(), "default", "default", "", false)
	require.NoError(t, err, "Failed to connect to ClickHouse")

	// Run migrations manually (goose doesn't work well with ClickHouse)
	err = runMigrations(ctx, db)
	require.NoError(t, err, "Failed to run migrations")

	// Cleanup function
	cleanup := func() {
		db.Close()
		clickhouseContainer.Terminate(ctx)
	}

	return db, cleanup
}

// TestClickHouseDB_CreateBook tests book creation
func TestClickHouseDB_CreateBook(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Test creating a book
	bookName, err := db.CreateBook(ctx, "Harry Potter")
	require.NoError(t, err)
	assert.Equal(t, "Harry Potter", bookName)

	// Verify the book exists
	books, err := db.ListReadableBooks(ctx)
	require.NoError(t, err)
	assert.Len(t, books, 1)
	assert.Equal(t, "Harry Potter", books[0].Name)
	assert.True(t, books[0].IsReadable)
}

// TestClickHouseDB_ListReadableBooks tests listing readable books
func TestClickHouseDB_ListReadableBooks(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Initially should be empty
	books, err := db.ListReadableBooks(ctx)
	require.NoError(t, err)
	assert.Empty(t, books)

	// Add multiple books
	_, err = db.CreateBook(ctx, "Book C")
	require.NoError(t, err)
	_, err = db.CreateBook(ctx, "Book A")
	require.NoError(t, err)
	_, err = db.CreateBook(ctx, "Book B")
	require.NoError(t, err)

	// Should return books sorted by name
	books, err = db.ListReadableBooks(ctx)
	require.NoError(t, err)
	assert.Len(t, books, 3)
	assert.Equal(t, "Book A", books[0].Name)
	assert.Equal(t, "Book B", books[1].Name)
	assert.Equal(t, "Book C", books[2].Name)
}

// TestClickHouseDB_ListParticipants tests listing participants
func TestClickHouseDB_ListParticipants(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Add test participants directly to database
	err := db.conn.Exec(ctx, `INSERT INTO participants (name, is_parent) VALUES (?, ?)`, "Alice", false)
	require.NoError(t, err)
	err = db.conn.Exec(ctx, `INSERT INTO participants (name, is_parent) VALUES (?, ?)`, "Bob", false)
	require.NoError(t, err)
	err = db.conn.Exec(ctx, `INSERT INTO participants (name, is_parent) VALUES (?, ?)`, "Mom", true)
	require.NoError(t, err)

	// List participants
	participants, err := db.ListParticipants(ctx)
	require.NoError(t, err)
	assert.Len(t, participants, 3)

	// Verify order (sorted by name)
	assert.Equal(t, "Alice", participants[0].Name)
	assert.False(t, participants[0].IsParent)
	assert.Equal(t, "Bob", participants[1].Name)
	assert.False(t, participants[1].IsParent)
	assert.Equal(t, "Mom", participants[2].Name)
	assert.True(t, participants[2].IsParent)
}

// TestClickHouseDB_CreateEvent tests event creation
func TestClickHouseDB_CreateEvent(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create test data
	_, err := db.CreateBook(ctx, "Test Book")
	require.NoError(t, err)

	err = db.conn.Exec(ctx, `INSERT INTO participants (name, is_parent) VALUES (?, ?)`, "Alice", false)
	require.NoError(t, err)

	// Create event
	eventDate := time.Now().UTC().Truncate(time.Second)
	err = db.CreateEvent(ctx, eventDate, "Test Book", "Alice")
	require.NoError(t, err)

	// Verify event was created
	events, err := db.GetLastEvents(ctx, 10)
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, "Test Book", events[0].BookName)
	assert.Equal(t, "Alice", events[0].ParticipantName)
	assert.WithinDuration(t, eventDate, events[0].Date, time.Second)
}

// TestClickHouseDB_GetLastEvents tests retrieving last events
func TestClickHouseDB_GetLastEvents(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create test data
	_, err := db.CreateBook(ctx, "Book 1")
	require.NoError(t, err)

	err = db.conn.Exec(ctx, `INSERT INTO participants (name, is_parent) VALUES (?, ?)`, "Alice", false)
	require.NoError(t, err)

	// Create multiple events with different dates
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		eventDate := baseTime.Add(time.Duration(i) * 24 * time.Hour)
		err = db.CreateEvent(ctx, eventDate, "Book 1", "Alice")
		require.NoError(t, err)
	}

	// Test limit
	events, err := db.GetLastEvents(ctx, 3)
	require.NoError(t, err)
	assert.Len(t, events, 3)

	// Verify order (most recent first)
	for i := 0; i < len(events)-1; i++ {
		assert.True(t, events[i].Date.After(events[i+1].Date) || events[i].Date.Equal(events[i+1].Date))
	}

	// Test getting all events
	events, err = db.GetLastEvents(ctx, 10)
	require.NoError(t, err)
	assert.Len(t, events, 5)
}

// TestClickHouseDB_GetTopBooks tests statistics queries
func TestClickHouseDB_GetTopBooks(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Setup test data
	books := []string{"Book A", "Book B", "Book C", "Book D"}
	for _, book := range books {
		_, err := db.CreateBook(ctx, book)
		require.NoError(t, err)
	}

	// Add participants
	err := db.conn.Exec(ctx, `INSERT INTO participants (name, is_parent) VALUES (?, ?)`, "Alice", false)
	require.NoError(t, err)
	err = db.conn.Exec(ctx, `INSERT INTO participants (name, is_parent) VALUES (?, ?)`, "Bob", false)
	require.NoError(t, err)
	err = db.conn.Exec(ctx, `INSERT INTO participants (name, is_parent) VALUES (?, ?)`, "Mom", true)
	require.NoError(t, err)

	// Create events
	baseDate := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	events := []struct {
		date        time.Time
		book        string
		participant string
	}{
		// June 2024
		{baseDate, "Book A", "Alice"},
		{baseDate.Add(1 * time.Hour), "Book A", "Alice"},
		{baseDate.Add(2 * time.Hour), "Book A", "Bob"},
		{baseDate.Add(3 * time.Hour), "Book B", "Alice"},
		{baseDate.Add(4 * time.Hour), "Book B", "Bob"},
		{baseDate.Add(5 * time.Hour), "Book C", "Alice"},
		{baseDate.Add(6 * time.Hour), "Book A", "Mom"}, // Parent event
		// July 2024
		{baseDate.AddDate(0, 1, 0), "Book B", "Alice"},
		{baseDate.AddDate(0, 1, 1), "Book C", "Bob"},
		{baseDate.AddDate(0, 1, 2), "Book C", "Alice"},
	}

	for _, e := range events {
		err = db.CreateEvent(ctx, e.date, e.book, e.participant)
		require.NoError(t, err)
	}

	t.Run("All children for specific month", func(t *testing.T) {
		startDate := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		endDate := time.Date(2024, 6, 30, 23, 59, 59, 0, time.UTC)

		stats, err := db.GetTopBooks(ctx, 10, startDate, endDate, "")
		require.NoError(t, err)

		// Should exclude Mom's event
		// Book A: 3 (Alice: 2, Bob: 1, Mom excluded)
		// Book B: 2 (Alice: 1, Bob: 1)
		// Book C: 1 (Alice: 1)
		require.Len(t, stats, 3)
		assert.Equal(t, "Book A", stats[0].BookName)
		assert.Equal(t, 3, stats[0].ReadCount) // Alice: 2 + Bob: 1 (Mom excluded)
		assert.Equal(t, "Book B", stats[1].BookName)
		assert.Equal(t, 2, stats[1].ReadCount)
		assert.Equal(t, "Book C", stats[2].BookName)
		assert.Equal(t, 1, stats[2].ReadCount)
	})

	t.Run("Specific child for specific month", func(t *testing.T) {
		startDate := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		endDate := time.Date(2024, 6, 30, 23, 59, 59, 0, time.UTC)

		stats, err := db.GetTopBooks(ctx, 10, startDate, endDate, "Alice")
		require.NoError(t, err)

		// Book A: 2, Book B: 1, Book C: 1
		require.Len(t, stats, 3)
		assert.Equal(t, "Book A", stats[0].BookName)
		assert.Equal(t, 2, stats[0].ReadCount)
		assert.Equal(t, "Book B", stats[1].BookName)
		assert.Equal(t, 1, stats[1].ReadCount)
		assert.Equal(t, "Book C", stats[2].BookName)
		assert.Equal(t, 1, stats[2].ReadCount)
	})

	t.Run("Multiple months", func(t *testing.T) {
		startDate := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		endDate := time.Date(2024, 7, 31, 23, 59, 59, 0, time.UTC)

		stats, err := db.GetTopBooks(ctx, 10, startDate, endDate, "")
		require.NoError(t, err)

		// Book A: 3 (June: Alice: 2, Bob: 1), Book B: 3 (June: 2, July: 1), Book C: 3 (June: 1, July: 2)
		require.Len(t, stats, 3)
		// All books have 3 reads, ordered by name
		assert.Equal(t, "Book A", stats[0].BookName)
		assert.Equal(t, 3, stats[0].ReadCount)
		assert.Equal(t, "Book B", stats[1].BookName)
		assert.Equal(t, 3, stats[1].ReadCount)
		assert.Equal(t, "Book C", stats[2].BookName)
		assert.Equal(t, 3, stats[2].ReadCount)
	})

	t.Run("Limit results", func(t *testing.T) {
		startDate := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		endDate := time.Date(2024, 7, 31, 23, 59, 59, 0, time.UTC)

		stats, err := db.GetTopBooks(ctx, 2, startDate, endDate, "")
		require.NoError(t, err)

		// Should return only top 2
		assert.Len(t, stats, 2)
	})

	t.Run("No events in period", func(t *testing.T) {
		startDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		endDate := time.Date(2025, 1, 31, 23, 59, 59, 0, time.UTC)

		stats, err := db.GetTopBooks(ctx, 10, startDate, endDate, "")
		require.NoError(t, err)
		assert.Empty(t, stats)
	})
}

// TestClickHouseDB_ConcurrentOperations tests concurrent access
func TestClickHouseDB_ConcurrentOperations(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Setup test data
	_, err := db.CreateBook(ctx, "Concurrent Book")
	require.NoError(t, err)

	err = db.conn.Exec(ctx, `INSERT INTO participants (name, is_parent) VALUES (?, ?)`, "Alice", false)
	require.NoError(t, err)

	// Create events concurrently
	numGoroutines := 10
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			eventDate := time.Now().Add(time.Duration(idx) * time.Minute)
			err := db.CreateEvent(ctx, eventDate, "Concurrent Book", "Alice")
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify all events were created
	events, err := db.GetLastEvents(ctx, 100)
	require.NoError(t, err)
	assert.Len(t, events, numGoroutines)
}

// TestClickHouseDB_Close tests connection closing
func TestClickHouseDB_Close(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	err := db.Close()
	assert.NoError(t, err)

	// Second close should not panic
	err = db.Close()
	assert.NoError(t, err)
}
