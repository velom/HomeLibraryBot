package bot

import (
	"context"
	"strconv"

	"go.uber.org/zap"
)

// registerPendingBook stores a book awaiting approval and returns its short id.
func (b *Bot) registerPendingBook(name string, labels []string, requestedBy string) string {
	b.pendingBooksMu.Lock()
	defer b.pendingBooksMu.Unlock()
	b.pendingBookSeq++
	id := strconv.FormatUint(b.pendingBookSeq, 10)
	b.pendingBooks[id] = &pendingBook{Name: name, Labels: labels, RequestedBy: requestedBy}
	return id
}

// popPendingBook removes and returns the pending book for id.
func (b *Bot) popPendingBook(id string) (*pendingBook, bool) {
	b.pendingBooksMu.Lock()
	defer b.pendingBooksMu.Unlock()
	pb, ok := b.pendingBooks[id]
	if ok {
		delete(b.pendingBooks, id)
	}
	return pb, ok
}

// confirmPendingBook creates the pending book (with labels) and removes it from the queue.
// Returns the book name, whether the id existed, and any storage error.
func (b *Bot) confirmPendingBook(ctx context.Context, id string) (string, bool, error) {
	pb, ok := b.popPendingBook(id)
	if !ok {
		return "", false, nil
	}
	if _, err := b.db.CreateBook(ctx, pb.Name); err != nil {
		return pb.Name, true, err
	}
	// The book now exists; label-attach failures are logged but not fatal, so the
	// approval message reports success rather than a misleading "failed to create".
	for _, label := range pb.Labels {
		if err := b.db.AddLabelToBook(ctx, pb.Name, label); err != nil {
			b.logger.Error("Failed to add label to approved book",
				zap.String("book", pb.Name), zap.String("label", label), zap.Error(err))
		}
	}
	return pb.Name, true, nil
}
