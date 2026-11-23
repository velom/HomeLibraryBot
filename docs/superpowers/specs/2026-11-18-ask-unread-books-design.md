# /ask: fix "never read" answers — design

## Problem

Langfuse session `ask-ABT4HR6G4FE56Z5U7QHTERMOQ4` (2026-11-17): the user asked which books were never read.
Ground truth: 11 of 161 readable books have zero events. The bot answered wrong four times.

Root causes:

1. **Bug — `get_rarely_read_books` is children-only and mislabels.** `toolGetRarelyReadBooks` calls
   `GetRarelyReadBooks(limit, childrenOnly=true, ...)` and prints "никогда не читали" for books children never read.
   In the session 24 of 35 such books had been read by parents. The tool description doesn't mention children-only.
2. **Bug — never-read books sorted last.** The query orders by `(max(e.date) <= toDateTime(0)) ASC`, so with the
   default `limit=10` never-read books never appear; the model had to pass `limit=500` (10 KB output).
3. **Capability gap — no direct "unread" tool.** `get_detailed_book_stats` / `get_participant_stats` drop zero rows
   (intentional, #29). Answering "never read" requires diffing 161 names from `get_books` against 150 in a 16 KB dump.
   `gemini-3.5-flash-lite` fails at this and reported "all books were read".
4. **Bug — participant filter silently mismatches.** `get_last_events` matches case-insensitively; `get_top_books`,
   `get_detailed_book_stats`, `get_participant_stats` use exact `=`. "маша" → "(нет данных)", which the model
   reads as "nobody read it".

## Options considered

- **A. Minimal**: `readers` param + precise wording on `get_rarely_read_books`; footer with zero-read books in stats tools.
  Still leaves set arithmetic / large output; footer is ambiguous with filters and re-inflates output undone by #29.
- **B. A + dedicated `get_unread_books` tool** computed server-side. (chosen)
- **C. Generic `query_reading_stats` tool or text-to-SQL.** Too many optional params for a small model, breaks
  Langfuse tool-name stability, text-to-SQL adds injection/data-access risk. Rejected.

Two independent subagent reviews (LLM-tool-design and Go-implementation perspectives) both recommended B.

## Design (B)

All changes in `internal/bot/ask.go` (+ tests). **No storage interface, SQL, mock or `/rare` changes.**
`GetDetailedBookStats` already returns zero rows (CROSS JOIN + `countIf`) in both CH and mock; the tool layer
just stops discarding them for this new use.

### 1. New tool `get_unread_books`

Description (intent-first so the model picks it):
"Книги, которые НЕ ЧИТАЛИ НИ РАЗУ — за всё время (без дат) или в указанном периоде (since/until);
можно ограничить участником и меткой. Используй для вопросов «не читали», «ни разу», «0 прочтений»,
«что ещё не читали». Возвращает точный список — сверять списки самому не нужно."

Params: `since`, `until` (YYYY-MM-DD, optional), `participant` (optional), `label` (optional),
`readers` (`"all"` | `"children"` | `"parents"`, description: «кого учитывать, по умолчанию все»).

Validation:
- Unknown `readers` value → `error: readers must be one of: all, children, parents`. No silent fallback.
- `participant` set together with `readers != all` → participant wins; header says
  "кто: Диана (readers проигнорирован)".
- Matching participant set empty (no participants, or `readers=children` with no children) →
  `error: нет участников для readers=children`. Without this guard, a CROSS JOIN with zero participants yields
  zero rows (would read as "all read"), and a role filter matching nobody makes every book look unread.

Implementation `toolGetUnreadBooks`:
1. Validate `readers`; `parseDateRange(since, until)`; resolve `participant` (see §4).
2. `participants := ListParticipants` → compute the matching set; empty → error (above).
3. `stats := GetDetailedBookStats(since, until, "", participant)`; `books := ListReadableBooks`
   (labels + canonical book set; label filtering done here since the storage call has no label param).
4. Sum `ReadCount` per book over matching participants; keep books with sum 0 and (if `label`) `has(label)`.
   Events for non-readable / unknown books are ignored, consistent with `get_books`.
5. Output:
   ```
   Ни разу не читали (кто: все; период: всё время): 11 из 161
   1. Ай!
   2. Гном и лис [Зима]
   ...
   ```
   Empty → "Все N книг читались хотя бы раз (кто: ...; период: ...)". Header echoes applied filters so the
   model can't misattribute scope.

### 2. `get_rarely_read_books`

- Add `readers` param (`"all"` | `"children"`, «по умолчанию все»), mapped to `childrenOnly`; unknown value → error.
  `/rare` (callbacks.go, http.go) keeps calling storage with its own flags — unaffected.
- If the result contains never-read rows, drop them and append one line:
  "Есть книги, которые ни разу не читали, — см. get_unread_books." No pointer when there are none.
  (`limit` applies in SQL before the drop; harmless since never-read rows sort last.)
- Description: «книги, которые читали давно (по дате последнего прочтения); по умолчанию учитываются все участники».

### 3. Description / prompt tweaks

- `get_detailed_book_stats`, `get_participant_stats`: append "Для непрочитанных книг используй get_unread_books."
- System prompt (`askSystemPrompt`): one rule — "Вопросы «что не читали / ни разу» — через get_unread_books."

### 4. Participant name resolution (all tools taking `participant`)

Helper `resolveParticipant(ctx, name) (canonical string, errStr string)`: TrimSpace, empty → "";
case-insensitive match against `ListParticipants`; no match →
`error: unknown participant "маша"; known: Вика, Диана, Мама, Папа`. Applied in `executeTool` before calling
`get_last_events`, `get_top_books`, `get_detailed_book_stats`, `get_participant_stats`, `get_unread_books`.
Book-name filter left as-is (out of scope).

### Observability

New tool observation name `get_unread_books`; existing names unchanged (Langfuse evaluators depend on them).

## Testing (`internal/bot/ask_test.go`, mock DB)

- `get_unread_books`: book read only by a parent is NOT listed with readers=all, IS listed with readers=children;
  since/until boundaries (inclusive `until`); label filter; participant filter + "readers проигнорирован" header;
  "all read" message; invalid `readers` → error; empty matching participant set → error.
- `get_rarely_read_books`: default now counts parent reads; never-read rows absent + pointer present;
  no pointer when every book has been read.
- Participant resolution via `executeTool` (not `toolGet*` directly, which bypasses it): lowercase name works for
  `get_top_books` / `get_detailed_book_stats` / `get_unread_books`; unknown name error lists known names.
- Regression: reproduce the session scenario (parents-only books + truly unread) and assert exact unread set.

## Out of scope / follow-ups

- Timezone of day boundaries in `parseDateRange` (UTC vs CH/server TZ) — verify separately.
- `limit` clamping for `get_top_books` / `get_rarely_read_books`; `get_top_books` hidden children-only default,
  `days` vs `since/until` inconsistency, non-readable books leaking in with a participant filter.
- Langfuse dataset item from this session for regression evals.
