package bot

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"library/internal/storage/stubs"
)

func newTestBot(t *testing.T) (*Bot, *stubs.MockDB) {
	t.Helper()
	mockDB := stubs.NewMockDB()
	mockDB.Initialize(nil)
	b := &Bot{
		db:           mockDB,
		allowedUsers: map[int64]bool{123: true},
		states:       make(map[int64]*ConversationState),
		statesMu:     sync.RWMutex{},
		logger:       zap.NewNop(),
		pendingBooks: make(map[string]*pendingBook),
	}
	return b, mockDB
}

func TestRegisterPendingBook_StoresAndReturnsUniqueIDs(t *testing.T) {
	b, _ := newTestBot(t)

	id1 := b.registerPendingBook("Dune", []string{"sci-fi"}, "alice")
	id2 := b.registerPendingBook("1984", nil, "bob")

	assert.NotEqual(t, id1, id2)
	assert.Len(t, b.pendingBooks, 2)
	assert.Equal(t, "Dune", b.pendingBooks[id1].Name)
	assert.Equal(t, []string{"sci-fi"}, b.pendingBooks[id1].Labels)
}

func TestPopPendingBook_RemovesEntry(t *testing.T) {
	b, _ := newTestBot(t)
	id := b.registerPendingBook("Dune", nil, "alice")

	pb, ok := b.popPendingBook(id)
	require.True(t, ok)
	assert.Equal(t, "Dune", pb.Name)
	assert.Empty(t, b.pendingBooks)

	_, ok = b.popPendingBook(id)
	assert.False(t, ok, "second pop should miss")
}

func TestConfirmPendingBook_CreatesBookWithLabels(t *testing.T) {
	b, mockDB := newTestBot(t)
	id := b.registerPendingBook("Dune", []string{"sci-fi", "classic"}, "alice")

	name, ok, err := b.confirmPendingBook(context.Background(), id)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "Dune", name)

	books, err := mockDB.ListReadableBooks(context.Background())
	require.NoError(t, err)
	var found bool
	for _, bk := range books {
		if bk.Name == "Dune" {
			found = true
			assert.ElementsMatch(t, []string{"sci-fi", "classic"}, bk.Labels)
		}
	}
	assert.True(t, found, "Dune should have been created")
	assert.Empty(t, b.pendingBooks, "pending entry should be removed")
}

func TestConfirmPendingBook_UnknownID(t *testing.T) {
	b, _ := newTestBot(t)
	_, ok, err := b.confirmPendingBook(context.Background(), "nope")
	require.NoError(t, err)
	assert.False(t, ok)
}
