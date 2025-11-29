package models

import "time"

// Book represents a book in the library
type Book struct {
	Name       string
	IsReadable bool
}

// Participant represents a family member
type Participant struct {
	Name     string
	IsParent bool
}

// Event represents a reading event
type Event struct {
	Date            time.Time
	BookName        string
	ParticipantName string
}

// BookStat represents book reading statistics
type BookStat struct {
	BookName  string
	ReadCount int
}
