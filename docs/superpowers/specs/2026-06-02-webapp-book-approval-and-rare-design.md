# Design: Mini App book-approval flow + web `/rare` with label exclusion

Date: 2026-06-02

Two related Mini App features:

1. **Book-creation approval** — adding a new book from the web app no longer creates it
   directly. The bot posts a confirmation message to the notification chat with
   Accept/Reject inline buttons; the book is created only on Accept.
2. **Web `/rare`** — a Mini App analog of the `/rare` command, with a label multi-select.
   All labels selected by default (no filter); deselecting a label excludes books carrying
   it from the results.

## Context

- Mini App frontend is a single embedded `web/index.html`, currently one "Add Reading Event"
  form. No tabs.
- HTTP API in `internal/bot/http.go`: `GET /api/books`, `GET /api/participants`,
  `POST /api/events`. Auth via `authMiddleware` (skipped in polling mode).
- Inline-keyboard callbacks dispatch in `handlers.go:handleCallbackQuery`, prefix-routed.
  **It returns early when the user has no conversation state** (`handlers.go:168`).
- Notifications: `Bot.notificationChatID` / `notificationThreadID`, sent via
  `sendMessageInThread` / `sendMessageInThreadWithMarkup` (`utils.go`).
- `CreateBook(ctx, name)` sets `is_readable=true`; labels applied separately via
  `AddLabelToBook`. `/new_book` only collects a name.
- `GetRarelyReadBooks(ctx, limit, childrenOnly, label, excludeLabels []string)` already
  supports exclusion. `GetAllLabels(ctx)` returns all distinct labels.

## Feature 1 — Book-creation approval

### Backend

**Pending store (in-memory).** Add to the `Bot` struct:

```go
pendingBooks   map[string]*pendingBook
pendingBooksMu sync.Mutex
pendingBookSeq  uint64 // monotonic id source
```

```go
type pendingBook struct {
    Name        string
    Labels      []string
    RequestedBy string // username or id, for the message
}
```

IDs are short decimal strings from an incrementing counter (keeps `callback_data` tiny,
well under Telegram's 64-byte limit). Pending books live only in process memory; a bot
restart drops outstanding approvals. Acceptable for a single-instance home bot — documented,
not solved with a DB table (YAGNI).

**`POST /api/books`.** New handler, registered in `RegisterRoutes`.

Request: `{ "name": string, "labels": []string }` (labels optional).
Behavior:
- Validate `name` non-empty → 400 otherwise.
- If `notificationChatID == 0` (no approval channel configured, e.g. local dev): create the
  book directly (`CreateBook` + `AddLabelToBook` per label) and respond
  `{"status":"created"}`. Keeps dev usable.
- Otherwise: store a `pendingBook`, post a message to the notification chat with an inline
  keyboard, respond `{"status":"pending"}`.

Approval message:
```
📚 New book pending approval

Name: <name>
Labels: <a, b>      (omitted if none)

Approve to add it to the library.
```
Keyboard: `✅ Approve` → `book_accept:<id>`, `❌ Reject` → `book_reject:<id>`.

**Callback handling.** In `handleCallbackQuery`, dispatch `book_accept:` / `book_reject:`
**before** the no-conversation-state early return (the approver generally has no active
conversation). Authorization is already enforced at the top of `handleCallbackQuery`
(`allowedUsers`), so any allowed user may approve.

- `handleBookAcceptCallback`: pop pending by id (under mutex). If missing → edit message to
  "⚠️ Already processed". Else `CreateBook` + `AddLabelToBook` per label; on success edit the
  message to `✅ Added: <name>` (keyboard removed); on storage error edit to `❌ Error: …` and
  keep the pending entry removed.
- `handleBookRejectCallback`: pop pending by id; edit message to `❌ Rejected: <name>`.

Use `b.api.EditMessageText` (with empty `ReplyMarkup` to drop the buttons) and the existing
`AnswerCallbackQuery` already called at the top of the dispatcher.

### Frontend

Add a tab bar (see Shared frontend below) with an **Add Book** tab:
- Text input for name.
- Optional labels input (comma-separated, parsed to `[]string`, trimmed, empties dropped).
- Submit → `POST /api/books`. On `pending`, show "Sent for approval ✅"; on `created`, show
  "Book added ✅". Reset the form; do not auto-close (unlike the event form) so the user can
  add several.

## Feature 2 — Web `/rare` with label exclusion

### Backend

**`GET /api/labels`.** New handler returning `GetAllLabels(ctx)` as a JSON string array.
Populates the selector.

**`GET /api/rare`.** New handler. Query param `exclude` — repeatable
(`?exclude=A&exclude=B`); each value is a label to exclude. Parse via
`r.URL.Query()["exclude"]`, trim, drop empties.
- `children, _ := GetRarelyReadBooks(ctx, 10, true,  "", exclude)`
- `all, _      := GetRarelyReadBooks(ctx, 10, false, "", exclude)`
- Respond `{ "childrenBooks": [RareBookStat...], "allBooks": [RareBookStat...] }`.

No new storage method — reuse the existing exclusion-capable query. Unlike the bot command,
the web endpoint does **not** hardcode excluding `"Сами"`; exclusion is fully user-driven
(all labels selected ⇒ `exclude` empty ⇒ nothing filtered).

### Frontend

**Rare** tab:
- A collapsible "🏷 Labels (N selected)" panel: one checkbox per label from `/api/labels`,
  **all checked by default**. Unchecking a label adds it to `exclude`. "Select all" / "Clear"
  helpers.
- On open and on any checkbox change, fetch `/api/rare?exclude=…` (deselected labels) and
  render two sections, mirroring the bot output:
  - `👶 By children's choice` and `📖 Overall (all participants)`.
  - Each item: `N. <name>` + ` (never read)` when `daysSinceLastRead === -1`, else
    ` (<days> days ago, last: <YYYY-MM-DD>)`.

## Shared frontend — tab system

Introduce a minimal top tab bar in `web/index.html`: **Event** | **Book** | **Rare**.
Three section `<div>`s; clicking a tab toggles a `.active` class. Default tab: Event
(preserves current behavior). All three reuse the existing Telegram-theme CSS variables and
the `tma ${tg.initData}` auth header pattern. Books/participants/labels are fetched lazily or
on init; labels fetched when the Rare tab is first shown.

## Files to touch

| File | Change |
|------|--------|
| `internal/bot/types.go` | `pendingBooks`, `pendingBooksMu`, `pendingBookSeq`, `pendingBook` type |
| `internal/bot/constructor.go` | initialize `pendingBooks` map |
| `internal/bot/http.go` | `POST /api/books`, `GET /api/labels`, `GET /api/rare` handlers + routes; request/response structs |
| `internal/bot/handlers.go` | dispatch `book_accept:` / `book_reject:` before no-state return |
| `internal/bot/callbacks.go` | `handleBookAcceptCallback`, `handleBookRejectCallback` |
| `web/index.html` | tab bar + Add Book section + Rare section + JS |

No storage-interface, ClickHouse, migration, or model changes. Mock storage already supports
everything used.

## Testing

- Mock-backed unit tests for the new HTTP handlers (`internal/bot`):
  - `POST /api/books` with no notification chat → book created directly.
  - `POST /api/books` with notification chat configured → pending registered, not yet created.
  - accept callback creates the book (with labels); reject does not; unknown id is a no-op.
  - `GET /api/rare?exclude=…` returns the expected sections and honors exclusion.
  - `GET /api/labels` returns all labels.
- `make build` and `make test` green.

## Out of scope / non-goals

- Persisting pending approvals across restart (in-memory only).
- `is_readable=false` books from the web (mirrors `/new_book`, which only takes a name).
- Editing/deleting books from the web app.
- Changing the existing `/rare` bot command behavior.
