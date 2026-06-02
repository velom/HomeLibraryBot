package bot

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterPendingEvent_StoresAndReturnsUniqueIDs(t *testing.T) {
	b, _ := newTestBot(t)
	d := time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC)

	id1 := b.registerPendingEvent(d, "The Hobbit", "Alice", "Mini App")
	id2 := b.registerPendingEvent(d, "Matilda", "Bob", "Mini App")

	assert.NotEqual(t, id1, id2)
	assert.Len(t, b.pendingEvents, 2)
	assert.Equal(t, "The Hobbit", b.pendingEvents[id1].BookName)
	assert.Equal(t, "Alice", b.pendingEvents[id1].ParticipantName)
}

func TestRegisterPendingEvent_SharesSeqWithBooks(t *testing.T) {
	b, _ := newTestBot(t)
	d := time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC)

	bookID := b.registerPendingBook("Dune", nil, "Mini App")
	eventID := b.registerPendingEvent(d, "The Hobbit", "Alice", "Mini App")

	assert.NotEqual(t, bookID, eventID, "shared counter must not collide across books/events")
}

func TestPopPendingEvent_RemovesEntry(t *testing.T) {
	b, _ := newTestBot(t)
	d := time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC)
	id := b.registerPendingEvent(d, "The Hobbit", "Alice", "Mini App")

	pe, ok := b.popPendingEvent(id)
	require.True(t, ok)
	assert.Equal(t, "The Hobbit", pe.BookName)
	assert.Empty(t, b.pendingEvents)

	_, ok = b.popPendingEvent(id)
	assert.False(t, ok, "second pop should miss")
}

func TestConfirmPendingEvent_CreatesEvent(t *testing.T) {
	b, mockDB := newTestBot(t)
	d := time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC)
	id := b.registerPendingEvent(d, "The Hobbit", "Alice", "Mini App")

	pe, ok, err := b.confirmPendingEvent(context.Background(), id)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "The Hobbit", pe.BookName)

	events, err := mockDB.GetLastEvents(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "The Hobbit", events[0].BookName)
	assert.Equal(t, "Alice", events[0].ParticipantName)
	assert.Empty(t, b.pendingEvents, "pending entry should be removed")
}

func TestConfirmPendingEvent_UnknownID(t *testing.T) {
	b, _ := newTestBot(t)
	_, ok, err := b.confirmPendingEvent(context.Background(), "nope")
	require.NoError(t, err)
	assert.False(t, ok)
}
