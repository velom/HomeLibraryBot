package bot

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"library/internal/models"
)

func TestBuildAskSystemPrompt_WithData(t *testing.T) {
	books := []models.Book{
		{Name: "Колобок", IsReadable: true, Labels: []string{"Сказки", "Детям"}},
		{Name: "Теремок", IsReadable: true, Labels: nil},
	}
	participants := []models.Participant{
		{Name: "Миша", IsParent: false},
		{Name: "Папа", IsParent: true},
	}
	events := []models.Event{
		{Date: time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC), BookName: "Колобок", ParticipantName: "Миша"},
	}

	prompt := buildAskSystemPrompt(books, participants, events)

	assert.Contains(t, prompt, "Колобок")
	assert.Contains(t, prompt, "Сказки, Детям")
	assert.Contains(t, prompt, "Теремок")
	assert.Contains(t, prompt, "Миша")
	assert.Contains(t, prompt, "ребёнок")
	assert.Contains(t, prompt, "Папа")
	assert.Contains(t, prompt, "родитель")
	assert.Contains(t, prompt, "2026-03-25")
	assert.Contains(t, prompt, "помощник семейной библиотеки")
}

func TestBuildAskSystemPrompt_EmptyData(t *testing.T) {
	prompt := buildAskSystemPrompt(nil, nil, nil)

	assert.Contains(t, prompt, "помощник семейной библиотеки")
	assert.Contains(t, prompt, "Книги")
	assert.Contains(t, prompt, "Участники")
	assert.Contains(t, prompt, "Последние события")
}

func TestBuildAskSystemPrompt_ContainsDate(t *testing.T) {
	prompt := buildAskSystemPrompt(nil, nil, nil)

	today := time.Now().Format("2006-01-02")
	assert.Contains(t, prompt, today)
}

func TestBuildAskSystemPrompt_BookWithNoLabels(t *testing.T) {
	books := []models.Book{
		{Name: "Книга без меток", IsReadable: true, Labels: nil},
	}
	prompt := buildAskSystemPrompt(books, nil, nil)

	assert.Contains(t, prompt, "Книга без меток")
}
