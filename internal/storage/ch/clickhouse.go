package ch

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"library/internal/models"

	"github.com/ClickHouse/clickhouse-go/v2"
)

type ClickHouseDB struct {
	conn clickhouse.Conn
}

// NewClickHouseDB creates a new ClickHouse database connection
func NewClickHouseDB(host string, port int, database, user, password string, useTLS bool) (*ClickHouseDB, error) {
	addr := fmt.Sprintf("%s:%d", host, port)

	options := &clickhouse.Options{
		Addr:     []string{addr},
		Protocol: clickhouse.Native,
		Auth: clickhouse.Auth{
			Database: database,
			Username: user,
			Password: password,
		},
	}

	// Configure TLS if enabled
	if useTLS {
		options.TLS = &tls.Config{
			InsecureSkipVerify: false,
		}
	}

	conn, err := clickhouse.Open(options)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to ClickHouse: %w", err)
	}

	// Test the connection
	if err := conn.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ping ClickHouse: %w", err)
	}

	return &ClickHouseDB{conn: conn}, nil
}

// Initialize is a no-op - tables are managed via migrations
func (db *ClickHouseDB) Initialize(ctx context.Context) error {
	// Tables are managed via migrations (see migrations/ directory)
	// This method is kept for interface compatibility
	return nil
}

// CreateBook creates a new book and returns the book name as identifier
func (db *ClickHouseDB) CreateBook(ctx context.Context, name string) (string, error) {
	err := db.conn.Exec(ctx, `INSERT INTO books (name, is_readable) VALUES (?, ?)`,
		name, true)
	if err != nil {
		return "", fmt.Errorf("failed to create book: %w", err)
	}
	return name, nil
}

// ListReadableBooks returns all books that are available to read
func (db *ClickHouseDB) ListReadableBooks(ctx context.Context) ([]models.Book, error) {
	rows, err := db.conn.Query(ctx, `SELECT name, is_readable FROM books WHERE is_readable = true ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("failed to list readable books: %w", err)
	}
	defer rows.Close()

	var books []models.Book
	for rows.Next() {
		var book models.Book
		if err := rows.Scan(&book.Name, &book.IsReadable); err != nil {
			return nil, fmt.Errorf("failed to scan book: %w", err)
		}
		books = append(books, book)
	}
	return books, nil
}

// ListParticipants returns all participants
func (db *ClickHouseDB) ListParticipants(ctx context.Context) ([]models.Participant, error) {
	rows, err := db.conn.Query(ctx, `SELECT name, is_parent FROM participants ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("failed to list participants: %w", err)
	}
	defer rows.Close()

	var participants []models.Participant
	for rows.Next() {
		var participant models.Participant
		if err := rows.Scan(&participant.Name, &participant.IsParent); err != nil {
			return nil, fmt.Errorf("failed to scan participant: %w", err)
		}
		participants = append(participants, participant)
	}
	return participants, nil
}

// CreateEvent creates a new reading event
func (db *ClickHouseDB) CreateEvent(ctx context.Context, date time.Time, bookName, participantName string) error {
	err := db.conn.Exec(ctx, `INSERT INTO events (date, book_name, participant_name) VALUES (?, ?, ?)`,
		date, bookName, participantName)
	if err != nil {
		return fmt.Errorf("failed to create event: %w", err)
	}
	return nil
}

// GetLastEvents returns the last N events
func (db *ClickHouseDB) GetLastEvents(ctx context.Context, limit int) ([]models.Event, error) {
	rows, err := db.conn.Query(ctx, `SELECT date, book_name, participant_name FROM events ORDER BY date DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get last events: %w", err)
	}
	defer rows.Close()

	var events []models.Event
	for rows.Next() {
		var event models.Event
		if err := rows.Scan(&event.Date, &event.BookName, &event.ParticipantName); err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}
		events = append(events, event)
	}
	return events, nil
}

// GetTopBooks returns top N books by read count within the specified time period
// If participantName is empty, returns statistics for all children (IsParent=false)
// If participantName is provided, returns statistics only for that participant
func (db *ClickHouseDB) GetTopBooks(ctx context.Context, limit int, startDate, endDate time.Time, participantName string) ([]models.BookStat, error) {
	var query string
	var args []interface{}

	if participantName == "" {
		// Get stats for all children
		query = `
			SELECT
				e.book_name,
				toInt32(COUNT(*)) as read_count
			FROM events e
			INNER JOIN participants p ON e.participant_name = p.name
			WHERE e.date >= ?
				AND e.date <= ?
				AND p.is_parent = false
			GROUP BY e.book_name
			ORDER BY read_count DESC, e.book_name ASC
			LIMIT ?
		`
		args = []interface{}{startDate, endDate, limit}
	} else {
		// Get stats for specific participant
		query = `
			SELECT
				book_name,
				toInt32(COUNT(*)) as read_count
			FROM events
			WHERE date >= ?
				AND date <= ?
				AND participant_name = ?
			GROUP BY book_name
			ORDER BY read_count DESC, book_name ASC
			LIMIT ?
		`
		args = []interface{}{startDate, endDate, participantName, limit}
	}

	rows, err := db.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get top books: %w", err)
	}
	defer rows.Close()

	var stats []models.BookStat
	for rows.Next() {
		var bookName string
		var readCount int32
		if err := rows.Scan(&bookName, &readCount); err != nil {
			return nil, fmt.Errorf("failed to scan book stat: %w", err)
		}
		stats = append(stats, models.BookStat{
			BookName:  bookName,
			ReadCount: int(readCount),
		})
	}
	return stats, nil
}

// GetRarelyReadBooks returns books ordered by how long ago they were last read
// If childrenOnly is true, only considers reads by children (IsParent=false)
// If childrenOnly is false, considers reads by all participants
// Books never read are included with DaysSinceLastRead=-1
func (db *ClickHouseDB) GetRarelyReadBooks(ctx context.Context, limit int, childrenOnly bool) ([]models.RareBookStat, error) {
	var query string

	if childrenOnly {
		// Only consider reads by children
		query = `
			SELECT
				b.name as book_name,
				max(e.date) as last_read_date,
				if(max(e.date) <= toDateTime(0), -1, dateDiff('day', max(e.date), now())) as days_since_last_read
			FROM books b
			LEFT JOIN (
				SELECT e.book_name, e.date
				FROM events e
				INNER JOIN participants p ON e.participant_name = p.name
				WHERE p.is_parent = false
			) e ON b.name = e.book_name
			WHERE b.is_readable = true
			GROUP BY b.name
			ORDER BY
				(max(e.date) <= toDateTime(0)) ASC,
				days_since_last_read DESC,
				book_name ASC
			LIMIT ?
		`
	} else {
		// Consider reads by all participants
		query = `
			SELECT
				b.name as book_name,
				max(e.date) as last_read_date,
				if(max(e.date) <= toDateTime(0), -1, dateDiff('day', max(e.date), now())) as days_since_last_read
			FROM books b
			LEFT JOIN events e ON b.name = e.book_name
			WHERE b.is_readable = true
			GROUP BY b.name
			ORDER BY
				(max(e.date) <= toDateTime(0)) ASC,
				days_since_last_read DESC,
				book_name ASC
			LIMIT ?
		`
	}

	rows, err := db.conn.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get rarely read books: %w", err)
	}
	defer rows.Close()

	var stats []models.RareBookStat
	epoch := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)

	for rows.Next() {
		var bookName string
		var lastReadDate time.Time
		var daysSinceLastRead int64

		if err := rows.Scan(&bookName, &lastReadDate, &daysSinceLastRead); err != nil {
			return nil, fmt.Errorf("failed to scan rare book stat: %w", err)
		}

		// ClickHouse returns epoch (1970-01-01) for NULL DateTime values
		// Convert epoch to nil pointer
		var lastReadPtr *time.Time
		if lastReadDate.After(epoch) {
			lastReadPtr = &lastReadDate
		}

		stats = append(stats, models.RareBookStat{
			BookName:          bookName,
			LastReadDate:      lastReadPtr,
			DaysSinceLastRead: int(daysSinceLastRead),
		})
	}

	return stats, nil
}

// Close closes the database connection
func (db *ClickHouseDB) Close() error {
	if db.conn != nil {
		return db.conn.Close()
	}
	return nil
}
