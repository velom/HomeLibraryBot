# Label Query Commands Design

## Summary

Add two new Telegram bot commands for querying label-book relationships:

1. **`/book_labels`** — Show all labels assigned to a specific book
2. **`/books_by_label`** — Show all books that have a specific label

## Motivation

Currently users can add labels to books (`/add_label`) and filter rarely-read books by label (`/rare`), but there's no way to:
- See what labels a specific book has
- Browse books by label

These commands complete the label management workflow.

## Design

### Command: `/book_labels`

**Flow:**
1. User sends `/book_labels`
2. Bot fetches readable books via `ListReadableBooks()` and shows inline keyboard
3. User picks a book (callback `booklabels:N` where N is index)
4. Bot displays the book's labels (already populated in `Book.Labels`)

**No new storage methods needed** — `ListReadableBooks()` already returns books with their `Labels` field.

**Edge cases:**
- No readable books → "No readable books available"
- Book has no labels → "No labels found for '<book name>'"

### Command: `/books_by_label`

**Flow:**
1. User sends `/books_by_label`
2. Bot fetches all labels via `GetAllLabels()` and shows inline keyboard
3. User picks a label (callback `booksbylabel:label_name`)
4. Bot fetches and displays matching books

**New storage method needed:** `GetBooksByLabel(ctx, label) ([]models.Book, error)`
- Inverse of existing `GetBooksWithoutLabel`
- ClickHouse: `SELECT name, is_readable, labels FROM books WHERE is_readable = true AND has(labels, ?) ORDER BY name`
- Mock: filter books that contain the label

**Edge cases:**
- No labels exist → "No labels found. Use /add_label to add labels to books"
- No books with label → "No books found with label '<label>'"

## Changes Required

### Storage layer
1. **`internal/storage/storage.go`** — Add `GetBooksByLabel(ctx context.Context, label string) ([]models.Book, error)` to interface
2. **`internal/storage/ch/clickhouse.go`** — Implement with `has(labels, ?)` ClickHouse array function
3. **`internal/storage/stubs/mock.go`** — Implement with in-memory filter
4. **`internal/storage/stubs/mock_test.go`** — Add test for `GetBooksByLabel`

### Bot layer
5. **`internal/bot/commands.go`** — Add `handleBookLabelsStart()` and `handleBooksByLabelStart()`, update `/start` message
6. **`internal/bot/callbacks.go`** — Add `handleBookLabelsCallback()` and `handleBooksByLabelCallback()`
7. **`internal/bot/handlers.go`** — Register new commands and callback prefixes

## Implementation patterns

- Both commands are single-callback flows (command → keyboard → callback → response → done)
- Follow existing patterns: conversation state with `Step=1`, callback sets `Step=-1`
- Inline keyboards in 2-column layout matching existing style
- Callback data format: index-based for book selection (`booklabels:0`), name-based for label selection (`booksbylabel:Adventure`)
