package bot

import (
	"testing"

	"library/internal/models"

	"github.com/stretchr/testify/assert"
)

func TestComputeNextParticipant(t *testing.T) {
	testCases := []struct {
		name         string
		participants []models.Participant
		lastReader   string
		expectedNext string
		description  string
	}{
		{
			name:         "no participants",
			participants: []models.Participant{},
			lastReader:   "",
			expectedNext: "",
			description:  "should return empty string when no participants exist",
		},
		{
			name: "only parents no children",
			participants: []models.Participant{
				{Name: "Mom", IsParent: true},
				{Name: "Dad", IsParent: true},
			},
			lastReader:   "",
			expectedNext: "",
			description:  "should return empty string when only parents exist",
		},
		{
			name: "no events - start with first child",
			participants: []models.Participant{
				{Name: "Alice", IsParent: false},
				{Name: "Bob", IsParent: false},
				{Name: "Charlie", IsParent: false},
			},
			lastReader:   "",
			expectedNext: "Alice",
			description:  "should start with first child alphabetically when no events",
		},
		{
			name: "first child to second child",
			participants: []models.Participant{
				{Name: "Alice", IsParent: false},
				{Name: "Bob", IsParent: false},
				{Name: "Charlie", IsParent: false},
			},
			lastReader:   "Alice",
			expectedNext: "Bob",
			description:  "should rotate from first child to next child",
		},
		{
			name: "middle child to next child",
			participants: []models.Participant{
				{Name: "Alice", IsParent: false},
				{Name: "Bob", IsParent: false},
				{Name: "Charlie", IsParent: false},
			},
			lastReader:   "Bob",
			expectedNext: "Charlie",
			description:  "should rotate from middle child to next child",
		},
		{
			name: "last child to parent",
			participants: []models.Participant{
				{Name: "Alice", IsParent: false},
				{Name: "Bob", IsParent: false},
				{Name: "Mom", IsParent: true},
			},
			lastReader:   "Bob",
			expectedNext: "Mom",
			description:  "should suggest parent after last child",
		},
		{
			name: "last child to multiple parents",
			participants: []models.Participant{
				{Name: "Alice", IsParent: false},
				{Name: "Bob", IsParent: false},
				{Name: "Dad", IsParent: true},
				{Name: "Mom", IsParent: true},
			},
			lastReader:   "Bob",
			expectedNext: "Dad or Mom",
			description:  "should suggest first two parents after last child",
		},
		{
			name: "last child no parents - cycle back",
			participants: []models.Participant{
				{Name: "Alice", IsParent: false},
				{Name: "Bob", IsParent: false},
			},
			lastReader:   "Bob",
			expectedNext: "Alice",
			description:  "should cycle back to first child when no parents",
		},
		{
			name: "parent to first child",
			participants: []models.Participant{
				{Name: "Alice", IsParent: false},
				{Name: "Bob", IsParent: false},
				{Name: "Mom", IsParent: true},
			},
			lastReader:   "Mom",
			expectedNext: "Alice",
			description:  "should return to first child after parent reads",
		},
		{
			name: "any parent to first child - Mom",
			participants: []models.Participant{
				{Name: "Alice", IsParent: false},
				{Name: "Bob", IsParent: false},
				{Name: "Dad", IsParent: true},
				{Name: "Mom", IsParent: true},
			},
			lastReader:   "Mom",
			expectedNext: "Alice",
			description:  "should return to first child after Mom reads",
		},
		{
			name: "any parent to first child - Dad",
			participants: []models.Participant{
				{Name: "Alice", IsParent: false},
				{Name: "Bob", IsParent: false},
				{Name: "Dad", IsParent: true},
				{Name: "Mom", IsParent: true},
			},
			lastReader:   "Dad",
			expectedNext: "Alice",
			description:  "should return to first child after Dad reads",
		},
		{
			name: "unknown participant defaults to first child",
			participants: []models.Participant{
				{Name: "Alice", IsParent: false},
				{Name: "Bob", IsParent: false},
				{Name: "Mom", IsParent: true},
			},
			lastReader:   "Unknown",
			expectedNext: "Alice",
			description:  "should default to first child for unknown participant",
		},
		{
			name: "single child no events",
			participants: []models.Participant{
				{Name: "OnlyChild", IsParent: false},
			},
			lastReader:   "",
			expectedNext: "OnlyChild",
			description:  "should return only child when no events",
		},
		{
			name: "single child cycles to self",
			participants: []models.Participant{
				{Name: "OnlyChild", IsParent: false},
			},
			lastReader:   "OnlyChild",
			expectedNext: "OnlyChild",
			description:  "should cycle only child back to self",
		},
		{
			name: "single child with parent",
			participants: []models.Participant{
				{Name: "OnlyChild", IsParent: false},
				{Name: "Parent", IsParent: true},
			},
			lastReader:   "OnlyChild",
			expectedNext: "Parent",
			description:  "should suggest parent after only child",
		},
		{
			name: "parent after single child returns to child",
			participants: []models.Participant{
				{Name: "OnlyChild", IsParent: false},
				{Name: "Parent", IsParent: true},
			},
			lastReader:   "Parent",
			expectedNext: "OnlyChild",
			description:  "should return to only child after parent",
		},
		{
			name: "three or more parents",
			participants: []models.Participant{
				{Name: "Alice", IsParent: false},
				{Name: "Aunt", IsParent: true},
				{Name: "Dad", IsParent: true},
				{Name: "Mom", IsParent: true},
			},
			lastReader:   "Alice",
			expectedNext: "Aunt or Dad",
			description:  "should suggest first two parents alphabetically",
		},
		{
			name: "alphabetical ordering verification",
			participants: []models.Participant{
				{Name: "Alice", IsParent: false},
				{Name: "Bob", IsParent: false},
				{Name: "Charlie", IsParent: false},
				{Name: "Dad", IsParent: true},
				{Name: "Mom", IsParent: true},
			},
			lastReader:   "",
			expectedNext: "Alice",
			description:  "should start with first child alphabetically",
		},
		{
			name: "alphabetical last child to parents",
			participants: []models.Participant{
				{Name: "Alice", IsParent: false},
				{Name: "Bob", IsParent: false},
				{Name: "Charlie", IsParent: false},
				{Name: "Dad", IsParent: true},
				{Name: "Mom", IsParent: true},
			},
			lastReader:   "Charlie",
			expectedNext: "Dad or Mom",
			description:  "should suggest parents in alphabetical order after last child",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ComputeNextParticipant(tc.participants, tc.lastReader)
			assert.Equal(t, tc.expectedNext, result, tc.description)
		})
	}
}

func TestComputeNextParticipant_FullRotationCycle(t *testing.T) {
	// Test a complete rotation cycle: start -> Alice -> Bob -> Mom -> Alice
	participants := []models.Participant{
		{Name: "Alice", IsParent: false},
		{Name: "Bob", IsParent: false},
		{Name: "Mom", IsParent: true},
	}

	rotationSteps := []struct {
		name         string
		lastReader   string
		expectedNext string
	}{
		{"step 1: no events", "", "Alice"},
		{"step 2: after Alice", "Alice", "Bob"},
		{"step 3: after Bob", "Bob", "Mom"},
		{"step 4: after Mom (restart)", "Mom", "Alice"},
		{"step 5: cycle continues", "Alice", "Bob"},
	}

	for _, step := range rotationSteps {
		t.Run(step.name, func(t *testing.T) {
			result := ComputeNextParticipant(participants, step.lastReader)
			assert.Equal(t, step.expectedNext, result)
		})
	}
}
