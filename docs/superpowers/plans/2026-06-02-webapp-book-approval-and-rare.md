# Mini App Book-Approval + Web `/rare` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Telegram-approval gate for books created from the Mini App, and a Mini App analog of `/rare` with a label multi-select that excludes deselected labels.

**Architecture:** New HTTP endpoints in `internal/bot/http.go` (`POST /api/books`, `GET /api/labels`, `GET /api/rare`). Book creation registers an in-memory pending entry on the `Bot` struct and posts an inline-keyboard approval message to the notification chat; `book_accept:`/`book_reject:` callbacks (dispatched before the no-conversation-state early return) create or discard it. API-touching code is split from pure logic (`registerPendingBook`/`confirmPendingBook`/`popPendingBook`) so the latter is unit-testable without a live bot API. Reuses the existing exclusion-capable `GetRarelyReadBooks` query. Frontend gains a 3-tab UI in `web/index.html`.

**Tech Stack:** Go 1.25, `github.com/go-telegram/bot`, ClickHouse (unchanged here), mock storage (`internal/storage/stubs`), `net/http/httptest` + testify, vanilla JS Mini App.

---

## File Structure

- `internal/bot/types.go` — add `pendingBooks`, `pendingBooksMu`, `pendingBookSeq` fields and `pendingBook` struct.
- `internal/bot/constructor.go` — initialize `pendingBooks` map.
- `internal/bot/pending_books.go` (new) — pure pending-book logic: `registerPendingBook`, `popPendingBook`, `confirmPendingBook`.
- `internal/bot/http.go` — `POST /api/books`, `GET /api/labels`, `GET /api/rare` handlers + structs + routes.
- `internal/bot/callbacks.go` — `handleBookAcceptCallback`, `handleBookRejectCallback`.
- `internal/bot/handlers.go` — dispatch `book_accept:` / `book_reject:` before the no-state return.
- `internal/bot/pending_books_test.go` (new) — unit tests for pending logic.
- `internal/bot/http_test.go` — handler tests.
- `web/index.html` — tab bar + Add Book + Rare sections + JS.

---

## Task 1: Pending-book state on the Bot struct

**Files:**
- Modify: `internal/bot/types.go`
- Modify: `internal/bot/constructor.go:21-29` (the `botWrapper := &Bot{...}` literal)

- [ ] **Step 1: Add fields and type to `types.go`**

Add to the `Bot` struct (after `llmClient`):

```go
	llmClient            *llm.Client
	pendingBooks         map[string]*pendingBook
	pendingBooksMu       sync.Mutex
	pendingBookSeq       uint64
```

Add the type below the `Bot` struct:

```go
// pendingBook is a book submitted via the Mini App awaiting Telegram approval.
type pendingBook struct {
	Name        string
	Labels      []string
	RequestedBy string
}
```

- [ ] **Step 2: Initialize the map in `constructor.go`**

In the `botWrapper := &Bot{...}` literal, add:

```go
		llmClient:            llmClient,
		pendingBooks:         make(map[string]*pendingBook),
```

- [ ] **Step 3: Verify it compiles**

Run: `make build`
Expected: builds with no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/bot/types.go internal/bot/constructor.go
git commit -m "feat: add pending-book state to Bot struct"
```

---

## Task 2: Pure pending-book logic (register/pop/confirm)

**Files:**
- Create: `internal/bot/pending_books.go`
- Test: `internal/bot/pending_books_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/bot/pending_books_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestRegisterPendingBook_StoresAndReturnsUniqueIDs|TestPopPendingBook_RemovesEntry|TestConfirmPendingBook' ./internal/bot/`
Expected: FAIL — `registerPendingBook`/`popPendingBook`/`confirmPendingBook` undefined.

- [ ] **Step 3: Write the implementation**

Create `internal/bot/pending_books.go`:

```go
package bot

import (
	"context"
	"strconv"
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
	for _, label := range pb.Labels {
		if err := b.db.AddLabelToBook(ctx, pb.Name, label); err != nil {
			return pb.Name, true, err
		}
	}
	return pb.Name, true, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run 'TestRegisterPendingBook_StoresAndReturnsUniqueIDs|TestPopPendingBook_RemovesEntry|TestConfirmPendingBook' ./internal/bot/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/bot/pending_books.go internal/bot/pending_books_test.go
git commit -m "feat: add pending-book register/pop/confirm logic"
```

---

## Task 3: `POST /api/books` endpoint

**Files:**
- Modify: `internal/bot/http.go` (add struct + handler + route)
- Test: `internal/bot/http_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/bot/http_test.go`:

```go
func TestHandleCreateBook_NoNotificationChat_CreatesDirectly(t *testing.T) {
	hs, mockDB := newTestHTTPServer(t) // notificationChatID == 0
	hs.bot.pendingBooks = make(map[string]*pendingBook)

	body := `{"name":"Brand New Book","labels":["sci-fi"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/books", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	hs.handleCreateBook(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	var resp map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "created", resp["status"])

	books, err := mockDB.ListReadableBooks(nil)
	require.NoError(t, err)
	var found bool
	for _, bk := range books {
		if bk.Name == "Brand New Book" {
			found = true
			assert.Contains(t, bk.Labels, "sci-fi")
		}
	}
	assert.True(t, found)
}

func TestHandleCreateBook_EmptyName(t *testing.T) {
	hs, _ := newTestHTTPServer(t)
	hs.bot.pendingBooks = make(map[string]*pendingBook)

	req := httptest.NewRequest(http.MethodPost, "/api/books", bytes.NewBufferString(`{"name":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	hs.handleCreateBook(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleCreateBook_WrongMethod(t *testing.T) {
	hs, _ := newTestHTTPServer(t)
	hs.bot.pendingBooks = make(map[string]*pendingBook)

	req := httptest.NewRequest(http.MethodPut, "/api/books", nil)
	rec := httptest.NewRecorder()

	hs.handleBooks(rec, req) // dispatcher routes by method

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestHandleCreateBook' ./internal/bot/`
Expected: FAIL — `handleCreateBook` undefined.

- [ ] **Step 3: Implement the handler and route**

In `internal/bot/http.go`, change `handleBooks` to dispatch by method. Replace the existing `handleBooks` function (lines ~195-213) with:

```go
// handleBooks routes GET (list) and POST (create) for /api/books.
func (hs *HTTPServer) handleBooks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		hs.handleListBooks(w, r)
	case http.MethodPost:
		hs.handleCreateBook(w, r)
	default:
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// handleListBooks returns the list of readable books.
func (hs *HTTPServer) handleListBooks(w http.ResponseWriter, r *http.Request) {
	hs.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		books, err := hs.bot.db.ListReadableBooks(r.Context())
		if err != nil {
			hs.bot.logger.Error("Failed to list books", zap.Error(err))
			http.Error(w, `{"error":"Failed to fetch books"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(books)
	})(w, r)
}

// CreateBookRequest is the body for POST /api/books.
type CreateBookRequest struct {
	Name   string   `json:"name"`
	Labels []string `json:"labels"`
}

// handleCreateBook either creates a book directly (no notification chat configured)
// or posts an approval message to the notification chat and registers it as pending.
func (hs *HTTPServer) handleCreateBook(w http.ResponseWriter, r *http.Request) {
	hs.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		var req CreateBookRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			hs.bot.logger.Warn("Failed to decode create-book body", zap.Error(err))
			http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
			return
		}

		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			http.Error(w, `{"error":"Missing book name"}`, http.StatusBadRequest)
			return
		}

		labels := make([]string, 0, len(req.Labels))
		for _, l := range req.Labels {
			if l = strings.TrimSpace(l); l != "" {
				labels = append(labels, l)
			}
		}

		// No approval channel: create immediately (keeps local dev usable).
		if hs.bot.notificationChatID == 0 {
			if _, err := hs.bot.db.CreateBook(r.Context(), req.Name); err != nil {
				hs.bot.logger.Error("Failed to create book", zap.Error(err), zap.String("book", req.Name))
				http.Error(w, `{"error":"Failed to create book"}`, http.StatusInternalServerError)
				return
			}
			for _, l := range labels {
				if err := hs.bot.db.AddLabelToBook(r.Context(), req.Name, l); err != nil {
					hs.bot.logger.Error("Failed to add label", zap.Error(err), zap.String("book", req.Name), zap.String("label", l))
				}
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{"status": "created"})
			return
		}

		// Approval channel configured: register pending and ask for confirmation.
		id := hs.bot.registerPendingBook(req.Name, labels, "Mini App")
		hs.bot.sendBookApprovalRequest(r.Context(), id)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": "pending"})
	})(w, r)
}
```

Update `RegisterRoutes` — `/api/books` is already registered to `hs.handleBooks`, no change needed there.

Note: `sendBookApprovalRequest` is implemented in Task 4. To keep this task compiling on its own, add a temporary stub at the bottom of `http.go` that Task 4 replaces — OR implement Task 4's `sendBookApprovalRequest` now. **Implement it now** (copy from Task 4 Step 3) so `http.go` compiles. Mark Task 4 Step 3 done when you do.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run 'TestHandleCreateBook|TestHandleBooks' ./internal/bot/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/bot/http.go internal/bot/http_test.go
git commit -m "feat: POST /api/books with direct-create fallback"
```

---

## Task 4: Approval message + accept/reject callbacks

**Files:**
- Modify: `internal/bot/http.go` (add `sendBookApprovalRequest` — done in Task 3 if you implemented it there)
- Modify: `internal/bot/callbacks.go` (add accept/reject handlers)
- Modify: `internal/bot/handlers.go` (dispatch before no-state return)

- [ ] **Step 1: Add the dispatch in `handlers.go`**

In `handleCallbackQuery`, **before** the `b.statesMu.RLock()` / `if !ok { return }` block (currently around line 163-174), insert:

```go
	// Book-approval callbacks are not tied to a conversation state.
	if strings.HasPrefix(query.Data, "book_accept:") {
		b.handleBookAcceptCallback(ctx, query)
		return
	}
	if strings.HasPrefix(query.Data, "book_reject:") {
		b.handleBookRejectCallback(ctx, query)
		return
	}
```

- [ ] **Step 2: Add the callback handlers in `callbacks.go`**

Append to `internal/bot/callbacks.go`:

```go
// handleBookAcceptCallback approves a pending book: creates it and edits the message.
func (b *Bot) handleBookAcceptCallback(ctx context.Context, query *models.CallbackQuery) {
	id := strings.TrimPrefix(query.Data, "book_accept:")
	name, ok, err := b.confirmPendingBook(ctx, id)
	var text string
	switch {
	case !ok:
		text = "⚠️ This request was already processed."
	case err != nil:
		b.logger.Error("Failed to create approved book", zap.Error(err), zap.String("book", name))
		text = fmt.Sprintf("❌ Error creating \"%s\": %v", name, err)
	default:
		text = fmt.Sprintf("✅ Added: %s", name)
	}
	b.editCallbackMessage(ctx, query, text)
}

// handleBookRejectCallback discards a pending book and edits the message.
func (b *Bot) handleBookRejectCallback(ctx context.Context, query *models.CallbackQuery) {
	id := strings.TrimPrefix(query.Data, "book_reject:")
	pb, ok := b.popPendingBook(id)
	var text string
	if !ok {
		text = "⚠️ This request was already processed."
	} else {
		text = fmt.Sprintf("❌ Rejected: %s", pb.Name)
	}
	b.editCallbackMessage(ctx, query, text)
}

// editCallbackMessage replaces the text of the message that carried the inline keyboard,
// dropping the buttons.
func (b *Bot) editCallbackMessage(ctx context.Context, query *models.CallbackQuery, text string) {
	if query.Message.Message == nil {
		return
	}
	b.api.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:    query.Message.Message.Chat.ID,
		MessageID: query.Message.Message.ID,
		Text:      text,
	})
}
```

Confirm `callbacks.go` already imports `fmt`, `strings`, `go.uber.org/zap`, `github.com/go-telegram/bot/models`, and the `tgbot "github.com/go-telegram/bot"` alias. The dispatcher file `handlers.go` uses `tgbot` for `AnswerCallbackQueryParams`, so the alias exists in the package; add the import to `callbacks.go` if missing (check the file header first).

- [ ] **Step 3: Add `sendBookApprovalRequest` in `http.go` (if not already added in Task 3)**

Add this method (on `*Bot`, so the HTTP handler can call it) near the other notification code in `http.go`:

```go
// sendBookApprovalRequest posts an approval message with inline Approve/Reject buttons.
func (b *Bot) sendBookApprovalRequest(ctx context.Context, id string) {
	b.pendingBooksMu.Lock()
	pb, ok := b.pendingBooks[id]
	b.pendingBooksMu.Unlock()
	if !ok {
		return
	}

	text := fmt.Sprintf("📚 New book pending approval\n\nName: %s", pb.Name)
	if len(pb.Labels) > 0 {
		text += fmt.Sprintf("\nLabels: %s", strings.Join(pb.Labels, ", "))
	}
	text += "\n\nApprove to add it to the library."

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "✅ Approve", CallbackData: "book_accept:" + id},
				{Text: "❌ Reject", CallbackData: "book_reject:" + id},
			},
		},
	}
	b.sendMessageInThreadWithMarkup(ctx, b.notificationChatID, text, b.notificationThreadID, keyboard)
}
```

Add `"github.com/go-telegram/bot/models"` to `http.go` imports if not present (it is not currently — add it).

- [ ] **Step 4: Verify build and full test suite**

Run: `make build`
Expected: builds clean.

Run: `make test`
Expected: all pass (existing + new pending/HTTP tests).

- [ ] **Step 5: Commit**

```bash
git add internal/bot/http.go internal/bot/callbacks.go internal/bot/handlers.go
git commit -m "feat: book approval message and accept/reject callbacks"
```

---

## Task 5: `GET /api/labels` and `GET /api/rare`

**Files:**
- Modify: `internal/bot/http.go` (handlers + routes)
- Test: `internal/bot/http_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/bot/http_test.go`:

```go
func TestHandleLabels(t *testing.T) {
	hs, mockDB := newTestHTTPServer(t)
	require.NoError(t, mockDB.AddLabelToBook(nil, "Book 1", "sci-fi"))
	require.NoError(t, mockDB.AddLabelToBook(nil, "Book 2", "fantasy"))

	req := httptest.NewRequest(http.MethodGet, "/api/labels", nil)
	rec := httptest.NewRecorder()

	hs.handleLabels(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var labels []string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&labels))
	assert.Contains(t, labels, "sci-fi")
	assert.Contains(t, labels, "fantasy")
}

func TestHandleRare_Sections(t *testing.T) {
	hs, _ := newTestHTTPServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/rare", nil)
	rec := httptest.NewRecorder()

	hs.handleRare(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		ChildrenBooks []models.RareBookStat `json:"childrenBooks"`
		AllBooks      []models.RareBookStat `json:"allBooks"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.NotNil(t, resp.ChildrenBooks)
	assert.NotNil(t, resp.AllBooks)
}

func TestHandleRare_ExcludesLabel(t *testing.T) {
	hs, mockDB := newTestHTTPServer(t)
	require.NoError(t, mockDB.AddLabelToBook(nil, "Book 1", "skip-me"))

	req := httptest.NewRequest(http.MethodGet, "/api/rare?exclude=skip-me", nil)
	rec := httptest.NewRecorder()

	hs.handleRare(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		ChildrenBooks []models.RareBookStat `json:"childrenBooks"`
		AllBooks      []models.RareBookStat `json:"allBooks"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	for _, s := range resp.AllBooks {
		assert.NotEqual(t, "Book 1", s.BookName, "excluded label's book must not appear")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestHandleLabels|TestHandleRare' ./internal/bot/`
Expected: FAIL — `handleLabels`/`handleRare` undefined.

- [ ] **Step 3: Implement handlers and routes**

Add to `RegisterRoutes` in `internal/bot/http.go`:

```go
	mux.HandleFunc("/api/labels", hs.handleLabels)
	mux.HandleFunc("/api/rare", hs.handleRare)
```

Add the handlers:

```go
// handleLabels returns all distinct labels in the library.
func (hs *HTTPServer) handleLabels(w http.ResponseWriter, r *http.Request) {
	hs.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		labels, err := hs.bot.db.GetAllLabels(r.Context())
		if err != nil {
			hs.bot.logger.Error("Failed to list labels", zap.Error(err))
			http.Error(w, `{"error":"Failed to fetch labels"}`, http.StatusInternalServerError)
			return
		}
		if labels == nil {
			labels = []string{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(labels)
	})(w, r)
}

// RareResponse is the body returned by GET /api/rare.
type RareResponse struct {
	ChildrenBooks []models.RareBookStat `json:"childrenBooks"`
	AllBooks      []models.RareBookStat `json:"allBooks"`
}

// handleRare returns rarely-read books, excluding any labels passed via repeated ?exclude=.
func (hs *HTTPServer) handleRare(w http.ResponseWriter, r *http.Request) {
	hs.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		var exclude []string
		for _, l := range r.URL.Query()["exclude"] {
			if l = strings.TrimSpace(l); l != "" {
				exclude = append(exclude, l)
			}
		}

		const limit = 10
		children, err := hs.bot.db.GetRarelyReadBooks(r.Context(), limit, true, "", exclude)
		if err != nil {
			hs.bot.logger.Error("Failed to get rare books (children)", zap.Error(err))
			http.Error(w, `{"error":"Failed to fetch rare books"}`, http.StatusInternalServerError)
			return
		}
		all, err := hs.bot.db.GetRarelyReadBooks(r.Context(), limit, false, "", exclude)
		if err != nil {
			hs.bot.logger.Error("Failed to get rare books (all)", zap.Error(err))
			http.Error(w, `{"error":"Failed to fetch rare books"}`, http.StatusInternalServerError)
			return
		}
		if children == nil {
			children = []models.RareBookStat{}
		}
		if all == nil {
			all = []models.RareBookStat{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(RareResponse{ChildrenBooks: children, AllBooks: all})
	})(w, r)
}
```

(`models` is already imported by Task 4 Step 3. If Task 4 was skipped, add `"library/internal/models"` to `http.go` imports.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run 'TestHandleLabels|TestHandleRare' ./internal/bot/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/bot/http.go internal/bot/http_test.go
git commit -m "feat: GET /api/labels and GET /api/rare endpoints"
```

---

## Task 6: Frontend — tabs, Add Book, Rare

**Files:**
- Modify: `web/index.html`

This task has no automated test; verify by building and eyeballing. Keep all existing event-form code working.

- [ ] **Step 1: Add tab bar + section wrappers (HTML)**

Replace the `<h1>📚 Add Reading Event</h1>` line and wrap the existing `<form id="eventForm">…</form>` so the body becomes:

```html
    <div class="tabs">
        <button type="button" class="tab-btn active" data-tab="event">📝 Event</button>
        <button type="button" class="tab-btn" data-tab="book">➕ Book</button>
        <button type="button" class="tab-btn" data-tab="rare">📚 Rare</button>
    </div>

    <section id="tab-event" class="tab-panel active">
        <h1>📚 Add Reading Event</h1>
        <!-- existing <form id="eventForm"> ... </form> stays here unchanged -->
    </section>

    <section id="tab-book" class="tab-panel">
        <h1>➕ Add Book</h1>
        <form id="bookForm">
            <div class="form-group">
                <label for="bookName">📖 Name</label>
                <input type="text" id="bookName" placeholder="Book title" autocomplete="off">
            </div>
            <div class="form-group">
                <label for="bookLabels">🏷 Labels (comma-separated, optional)</label>
                <input type="text" id="bookLabels" placeholder="e.g. sci-fi, classic" autocomplete="off">
            </div>
            <button type="submit" id="bookSubmitBtn">Submit Book</button>
            <div id="bookError" class="error"></div>
            <div id="bookSuccess" class="success"></div>
        </form>
    </section>

    <section id="tab-rare" class="tab-panel">
        <h1>📚 Rarely Read Books</h1>
        <div class="form-group">
            <button type="button" id="labelsToggle" class="secondary-btn">🏷 Labels (all)</button>
            <div id="labelsPanel" class="labels-panel">
                <div class="labels-actions">
                    <span class="clear-selection" id="labelsSelectAll">Select all</span>
                    <span class="clear-selection" id="labelsClear">Clear</span>
                </div>
                <div id="labelsList"></div>
            </div>
        </div>
        <div id="rareLoading" class="loading" style="display:none;">Loading…</div>
        <div id="rareResults"></div>
    </section>
```

- [ ] **Step 2: Add CSS for tabs, panels, labels**

Add inside the existing `<style>` block:

```css
        .tabs { display: flex; gap: 8px; margin-bottom: 16px; }
        .tab-btn {
            flex: 1; padding: 10px; font-size: 14px; font-weight: 600;
            background-color: var(--tg-theme-secondary-bg-color, #f0f0f0);
            color: var(--tg-theme-text-color, #000); border: none; border-radius: 8px; cursor: pointer;
        }
        .tab-btn.active {
            background-color: var(--tg-theme-button-color, #3390ec);
            color: var(--tg-theme-button-text-color, #fff);
        }
        .tab-panel { display: none; }
        .tab-panel.active { display: block; }
        .secondary-btn {
            background-color: var(--tg-theme-secondary-bg-color, #f0f0f0);
            color: var(--tg-theme-text-color, #000);
        }
        .labels-panel {
            display: none; margin-top: 8px; padding: 12px;
            border: 1px solid var(--tg-theme-hint-color, #ccc); border-radius: 8px;
            max-height: 240px; overflow-y: auto;
        }
        .labels-panel.visible { display: block; }
        .labels-actions { display: flex; gap: 16px; margin-bottom: 8px; }
        .label-check { display: flex; align-items: center; gap: 8px; padding: 6px 0; }
        .label-check input { width: auto; }
        .rare-section-title { font-weight: 600; margin: 16px 0 8px; }
        .rare-item { padding: 6px 0; border-bottom: 1px solid var(--tg-theme-hint-color, #eee); font-size: 14px; }
```

- [ ] **Step 3: Add tab-switching + Add Book + Rare JS**

Add to the `<script>` (after the existing `init()` definition, before `init();`):

```javascript
        // --- Tabs ---
        let rareLoaded = false;
        document.querySelectorAll('.tab-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
                document.querySelectorAll('.tab-panel').forEach(p => p.classList.remove('active'));
                btn.classList.add('active');
                const tab = btn.dataset.tab;
                document.getElementById('tab-' + tab).classList.add('active');
                if (tab === 'rare' && !rareLoaded) { rareLoaded = true; initRare(); }
            });
        });

        // --- Add Book ---
        const bookForm = document.getElementById('bookForm');
        const bookNameInput = document.getElementById('bookName');
        const bookLabelsInput = document.getElementById('bookLabels');
        const bookSubmitBtn = document.getElementById('bookSubmitBtn');
        const bookErrorDiv = document.getElementById('bookError');
        const bookSuccessDiv = document.getElementById('bookSuccess');

        bookForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            bookErrorDiv.classList.remove('visible');
            bookSuccessDiv.classList.remove('visible');
            const name = bookNameInput.value.trim();
            if (!name) {
                bookErrorDiv.textContent = 'Please enter a book name';
                bookErrorDiv.classList.add('visible');
                return;
            }
            const labels = bookLabelsInput.value.split(',').map(s => s.trim()).filter(Boolean);
            bookSubmitBtn.disabled = true;
            bookSubmitBtn.textContent = 'Submitting…';
            try {
                const response = await fetch('/api/books', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json', 'Authorization': `tma ${tg.initData}` },
                    body: JSON.stringify({ name, labels })
                });
                if (!response.ok) {
                    const err = await response.json().catch(() => ({}));
                    throw new Error(err.error || 'Failed to submit book');
                }
                const data = await response.json();
                bookSuccessDiv.textContent = data.status === 'pending'
                    ? 'Sent for approval ✅' : 'Book added ✅';
                bookSuccessDiv.classList.add('visible');
                bookForm.reset();
            } catch (err) {
                bookErrorDiv.textContent = err.message || 'Failed to submit book';
                bookErrorDiv.classList.add('visible');
            } finally {
                bookSubmitBtn.disabled = false;
                bookSubmitBtn.textContent = 'Submit Book';
            }
        });

        // --- Rare ---
        const labelsToggle = document.getElementById('labelsToggle');
        const labelsPanel = document.getElementById('labelsPanel');
        const labelsList = document.getElementById('labelsList');
        const labelsSelectAll = document.getElementById('labelsSelectAll');
        const labelsClear = document.getElementById('labelsClear');
        const rareResults = document.getElementById('rareResults');
        const rareLoading = document.getElementById('rareLoading');
        let allLabels = [];

        labelsToggle.addEventListener('click', () => labelsPanel.classList.toggle('visible'));

        function selectedLabels() {
            return Array.from(labelsList.querySelectorAll('input:checked')).map(i => i.value);
        }
        function excludedLabels() {
            return allLabels.filter(l => !selectedLabels().includes(l));
        }
        function updateLabelsToggleText() {
            const sel = selectedLabels().length;
            labelsToggle.textContent = sel === allLabels.length
                ? '🏷 Labels (all)' : `🏷 Labels (${sel}/${allLabels.length})`;
        }

        async function initRare() {
            try {
                const resp = await fetch('/api/labels', { headers: { 'Authorization': `tma ${tg.initData}` } });
                allLabels = (await resp.json()) || [];
            } catch (e) { allLabels = []; }
            labelsList.innerHTML = '';
            allLabels.forEach(label => {
                const wrap = document.createElement('label');
                wrap.className = 'label-check';
                const cb = document.createElement('input');
                cb.type = 'checkbox'; cb.value = label; cb.checked = true;
                cb.addEventListener('change', () => { updateLabelsToggleText(); fetchRare(); });
                const span = document.createElement('span');
                span.textContent = label;
                wrap.appendChild(cb); wrap.appendChild(span);
                labelsList.appendChild(wrap);
            });
            updateLabelsToggleText();
            fetchRare();
        }

        labelsSelectAll.addEventListener('click', () => {
            labelsList.querySelectorAll('input').forEach(i => i.checked = true);
            updateLabelsToggleText(); fetchRare();
        });
        labelsClear.addEventListener('click', () => {
            labelsList.querySelectorAll('input').forEach(i => i.checked = false);
            updateLabelsToggleText(); fetchRare();
        });

        function renderRareSection(title, items) {
            let html = `<div class="rare-section-title">${title}</div>`;
            if (!items || items.length === 0) {
                html += '<div class="no-results">No data available</div>';
                return html;
            }
            items.forEach((s, i) => {
                let suffix;
                if (s.daysSinceLastRead === -1) {
                    suffix = ' (never read)';
                } else {
                    suffix = ` (${s.daysSinceLastRead} days ago, last: ${s.lastReadDate ? s.lastReadDate.slice(0,10) : ''})`;
                }
                html += `<div class="rare-item">${i + 1}. ${s.bookName}${suffix}</div>`;
            });
            return html;
        }

        async function fetchRare() {
            rareLoading.style.display = 'block';
            rareResults.innerHTML = '';
            try {
                const params = excludedLabels().map(l => 'exclude=' + encodeURIComponent(l)).join('&');
                const url = '/api/rare' + (params ? '?' + params : '');
                const resp = await fetch(url, { headers: { 'Authorization': `tma ${tg.initData}` } });
                if (!resp.ok) throw new Error('Failed to load rare books');
                const data = await resp.json();
                rareResults.innerHTML =
                    renderRareSection("👶 By children's choice", data.childrenBooks) +
                    renderRareSection('📖 Overall (all participants)', data.allBooks);
            } catch (e) {
                rareResults.innerHTML = '<div class="no-results">Failed to load</div>';
            } finally {
                rareLoading.style.display = 'none';
            }
        }
```

- [ ] **Step 4: Build and manually verify**

Run: `make build`
Expected: builds clean (the HTML is embedded via `go:embed`).

Run: `make test`
Expected: all pass.

Manual check (optional): `make run-dev`, open `http://localhost:8081/web-app`, confirm the three tabs render, "Add Book" submits, and "Rare" loads two sections with a working label dropdown.

- [ ] **Step 5: Commit**

```bash
git add web/index.html
git commit -m "feat: Mini App tabs with Add Book and Rare views"
```

---

## Task 7: Final verification

- [ ] **Step 1: Full build + test**

Run: `make build && make test`
Expected: both succeed, no failures.

- [ ] **Step 2: Vet/format**

Run: `go vet ./... && go fmt ./...`
Expected: clean; commit any formatting changes.

- [ ] **Step 3: Commit any remaining changes**

```bash
git add -A
git commit -m "chore: vet/fmt"
```
```

---

## Self-Review

**Spec coverage:**
- Feature 1 approval flow → Tasks 1-4. ✅ (pending store, POST endpoint, approval message, accept/reject callbacks, dispatch-before-no-state, direct-create fallback when no notification chat.)
- Feature 2 web `/rare` → Task 5 (`/api/labels`, `/api/rare` with exclusion) + Task 6 (multi-select all-checked-by-default, exclusion semantics). ✅
- Shared tab UI → Task 6. ✅
- Testing section → unit tests in Tasks 2, 3, 5; build/test in 4, 6, 7. ✅
- No storage/migration/model changes (spec "out of scope") → honored. ✅

**Placeholder scan:** Task 4 Step 3 contained a confusing placeholder snippet; the real method is given immediately after with a clear instruction. All code blocks are complete.

**Type consistency:** `registerPendingBook`/`popPendingBook`/`confirmPendingBook` signatures match between Task 2 (impl) and Tasks 3-4 (callers). `pendingBook` fields (`Name`, `Labels`, `RequestedBy`) consistent. `RareResponse` JSON keys (`childrenBooks`/`allBooks`) match the frontend `data.childrenBooks`/`data.allBooks` and the test structs. `daysSinceLastRead`/`lastReadDate`/`bookName` match `models.RareBookStat` json tags.
