package models

import "time"

// Book represents a book in the library
type Book struct {
	Name       string   `json:"name"`
	IsReadable bool     `json:"isReadable"`
	Labels     []string `json:"labels"`
}

// Participant represents a family member
type Participant struct {
	Name     string `json:"name"`
	IsParent bool   `json:"isParent"`
}

// Event represents a reading event
type Event struct {
	Date            time.Time `json:"date"`
	BookName        string    `json:"bookName"`
	ParticipantName string    `json:"participantName"`
}

// BookStat represents book reading statistics
type BookStat struct {
	BookName  string `json:"bookName"`
	ReadCount int    `json:"readCount"`
}

// RareBookStat represents rarely read book statistics
type RareBookStat struct {
	BookName          string     `json:"bookName"`
	LastReadDate      *time.Time `json:"lastReadDate"` // nil if never read
	DaysSinceLastRead int        `json:"daysSinceLastRead"` // -1 if never read
}
