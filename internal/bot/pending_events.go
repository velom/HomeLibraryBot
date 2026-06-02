package bot

import (
	"context"
	"strconv"
	"time"
)

// registerPendingEvent stores a reading event awaiting approval and returns its short id.
func (b *Bot) registerPendingEvent(date time.Time, bookName, participantName, requestedBy string) string {
	b.pendingMu.Lock()
	defer b.pendingMu.Unlock()
	b.pendingSeq++
	id := strconv.FormatUint(b.pendingSeq, 10)
	b.pendingEvents[id] = &pendingEvent{
		Date:            date,
		BookName:        bookName,
		ParticipantName: participantName,
		RequestedBy:     requestedBy,
	}
	return id
}

// popPendingEvent removes and returns the pending event for id.
func (b *Bot) popPendingEvent(id string) (*pendingEvent, bool) {
	b.pendingMu.Lock()
	defer b.pendingMu.Unlock()
	pe, ok := b.pendingEvents[id]
	if ok {
		delete(b.pendingEvents, id)
	}
	return pe, ok
}

// confirmPendingEvent creates the pending reading event and removes it from the queue.
// Returns the popped event, whether the id existed, and any storage error.
func (b *Bot) confirmPendingEvent(ctx context.Context, id string) (*pendingEvent, bool, error) {
	pe, ok := b.popPendingEvent(id)
	if !ok {
		return nil, false, nil
	}
	if err := b.db.CreateEvent(ctx, pe.Date, pe.BookName, pe.ParticipantName); err != nil {
		return pe, true, err
	}
	return pe, true, nil
}
