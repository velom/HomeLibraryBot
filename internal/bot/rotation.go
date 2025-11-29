package bot

import (
	"library/internal/models"
)

// ComputeNextParticipant determines who should read next based on rotation logic.
//
// Rotation rules:
// 1. Children rotate alphabetically (sorted by name)
// 2. After the last child, a parent is suggested
// 3. After a parent, rotation returns to the first child
// 4. If no events exist, start with the first child
// 5. If last participant is unknown, default to first child
func ComputeNextParticipant(participants []models.Participant, lastParticipantName string) string {
	if len(participants) == 0 {
		return ""
	}

	// Separate children and parents (already sorted by name from storage)
	var children, parents []string
	for _, p := range participants {
		if p.IsParent {
			parents = append(parents, p.Name)
		} else {
			children = append(children, p.Name)
		}
	}

	// Must have at least one child
	if len(children) == 0 {
		return ""
	}

	// If no last participant (no events yet), start with first child
	if lastParticipantName == "" {
		return children[0]
	}

	// Check if last participant was a parent
	for _, p := range parents {
		if p == lastParticipantName {
			// After parent, return to first child
			return children[0]
		}
	}

	// Last participant was a child, find next in rotation
	for i, child := range children {
		if child == lastParticipantName {
			if i == len(children)-1 {
				// Last child in list, suggest a parent
				if len(parents) > 0 {
					if len(parents) == 1 {
						return parents[0]
					}
					// Multiple parents, suggest first two
					return parents[0] + " or " + parents[1]
				}
				// No parents, cycle back to first child
				return children[0]
			}
			// Return next child
			return children[i+1]
		}
	}

	// Unknown participant, default to first child
	return children[0]
}
