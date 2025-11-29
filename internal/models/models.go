package models

import "time"

// Book represents a book in the library
type Book struct {
	ID         string
	Name       string
	Author     string
	IsReadable bool
}

// Participant represents a family member
type Participant struct {
	ID       string
	Name     string
	IsParent bool
}

// Event represents a reading event
type Event struct {
	Date            time.Time
	BookName        string
	ParticipantName string
}
