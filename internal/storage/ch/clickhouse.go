package ch

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"library/internal/models"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
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

// Initialize creates the required tables
func (db *ClickHouseDB) Initialize(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS books (
			id String,
			name String,
			author String,
			is_readable Bool
		) ENGINE = MergeTree()
		ORDER BY id`,

		`CREATE TABLE IF NOT EXISTS participants (
			id String,
			name String,
			is_parent Bool
		) ENGINE = MergeTree()
		ORDER BY id`,

		`CREATE TABLE IF NOT EXISTS events (
			date DateTime,
			book_name String,
			participant_name String
		) ENGINE = MergeTree()
		ORDER BY date`,
	}

	for _, query := range queries {
		if err := db.conn.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	// Note: Participants should be added via API or manually
	// No default participants are created automatically
	return nil
}

// CreateBook creates a new book with a generated UUID
func (db *ClickHouseDB) CreateBook(ctx context.Context, name, author string) (string, error) {
	id := uuid.New().String()
	err := db.conn.Exec(ctx, `INSERT INTO books (id, name, author, is_readable) VALUES (?, ?, ?, ?)`,
		id, name, author, true)
	if err != nil {
		return "", fmt.Errorf("failed to create book: %w", err)
	}
	return id, nil
}

// ListReadableBooks returns all books that are available to read
func (db *ClickHouseDB) ListReadableBooks(ctx context.Context) ([]models.Book, error) {
	rows, err := db.conn.Query(ctx, `SELECT id, name, author, is_readable FROM books WHERE is_readable = true ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("failed to list readable books: %w", err)
	}
	defer rows.Close()

	var books []models.Book
	for rows.Next() {
		var book models.Book
		if err := rows.Scan(&book.ID, &book.Name, &book.Author, &book.IsReadable); err != nil {
			return nil, fmt.Errorf("failed to scan book: %w", err)
		}
		books = append(books, book)
	}
	return books, nil
}

// ListParticipants returns all participants
func (db *ClickHouseDB) ListParticipants(ctx context.Context) ([]models.Participant, error) {
	rows, err := db.conn.Query(ctx, `SELECT id, name, is_parent FROM participants ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("failed to list participants: %w", err)
	}
	defer rows.Close()

	var participants []models.Participant
	for rows.Next() {
		var participant models.Participant
		if err := rows.Scan(&participant.ID, &participant.Name, &participant.IsParent); err != nil {
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

// Close closes the database connection
func (db *ClickHouseDB) Close() error {
	if db.conn != nil {
		return db.conn.Close()
	}
	return nil
}
