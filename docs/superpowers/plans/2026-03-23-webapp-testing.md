# Mini App Testing & Local Development — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add test coverage for the Mini App HTTP layer and enable local browser-based development.

**Architecture:** Refactor `validateTelegramInitData` into a standalone function accepting a token parameter. Add a `botToken` field to `HTTPServer` so `authMiddleware` doesn't need `bot.api`. Write tests for auth validation, HTTP endpoints (polling mode), and auth middleware (webhook mode). Add a Telegram SDK shim to `index.html` for browser dev.

**Tech Stack:** Go stdlib `net/http/httptest`, `crypto/hmac`, mock storage (`stubs.MockDB`), `zap.NewNop()`

**Spec:** `docs/superpowers/specs/2026-03-23-webapp-testing-design.md`

---

### Task 1: Refactor `validateTelegramInitData` to standalone function

**Files:**
- Modify: `internal/bot/http.go:64-147` (validateTelegramInitData) and `internal/bot/http.go:19-31` (HTTPServer struct + constructor)

This task changes production code. The function becomes a package-level function accepting `botToken string` instead of using `hs.bot.api.Token()`. The `HTTPServer` struct gains a `botToken` field set at construction time.

- [ ] **Step 1: Add `botToken` field to `HTTPServer` and set it in constructor**

In `internal/bot/http.go`, change the struct and constructor:

```go
// HTTPServer handles HTTP requests for the Mini App
type HTTPServer struct {
	bot         *Bot
	webhookMode bool
	botToken    string // Bot token for initData validation; set from api.Token() at construction
}

// NewHTTPServer creates a new HTTP server for the Mini App
func NewHTTPServer(bot *Bot, webhookMode bool) *HTTPServer {
	return &HTTPServer{
		bot:         bot,
		webhookMode: webhookMode,
		botToken:    bot.api.Token(),
	}
}
```

- [ ] **Step 2: Convert `validateTelegramInitData` from method to standalone function**

Change the signature from:
```go
func (hs *HTTPServer) validateTelegramInitData(initData string) (int64, error) {
```
to:
```go
func validateTelegramInitData(initData string, botToken string, allowedUsers map[int64]bool) (int64, error) {
```

Inside the function body, replace:
- `hs.bot.api.Token()` → `botToken` (line 103)
- `hs.bot.allowedUsers[userData.ID]` → `allowedUsers[userData.ID]` (line 142)

- [ ] **Step 3: Update `authMiddleware` to use `hs.botToken` and standalone function**

In `authMiddleware` (line 174), change:
```go
userID, err := hs.validateTelegramInitData(initData)
```
to:
```go
userID, err := validateTelegramInitData(initData, hs.botToken, hs.bot.allowedUsers)
```

- [ ] **Step 4: Verify build compiles**

Run: `go build -C /Users/velom/GolandProjects/library ./...`
Expected: no errors

- [ ] **Step 5: Run existing tests to ensure no regressions**

Run: `go test -C /Users/velom/GolandProjects/library ./...`
Expected: all existing tests pass

- [ ] **Step 6: Commit**

```bash
git -C /Users/velom/GolandProjects/library add internal/bot/http.go
git -C /Users/velom/GolandProjects/library commit -m "refactor: extract validateTelegramInitData into standalone function with botToken param"
```

---

### Task 2: Auth validation unit tests

**Files:**
- Create: `internal/bot/auth_test.go`

Tests `validateTelegramInitData` directly with a known token, no HTTP layer involved.

- [ ] **Step 1: Write `generateTestInitData` helper and first test**

Create `internal/bot/auth_test.go`:

```go
package bot

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testBotToken = "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11"

// generateTestInitData builds a valid Telegram initData string signed with the given token.
// This mirrors Telegram's signing algorithm: HMAC-SHA256 of sorted key=value pairs,
// using a secret derived from HMAC-SHA256("WebAppData", botToken).
func generateTestInitData(t *testing.T, token string, userID int64, authDate time.Time) string {
	t.Helper()

	userData, err := json.Marshal(map[string]interface{}{
		"id":         userID,
		"first_name": "Test",
		"username":   "testuser",
	})
	require.NoError(t, err)

	params := url.Values{}
	params.Set("user", string(userData))
	params.Set("auth_date", fmt.Sprintf("%d", authDate.Unix()))
	params.Set("query_id", "test-query-id")

	// Build data-check-string (sorted key=value pairs joined by \n)
	var keys []string
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var dataCheckString strings.Builder
	for i, k := range keys {
		if i > 0 {
			dataCheckString.WriteByte('\n')
		}
		dataCheckString.WriteString(k)
		dataCheckString.WriteByte('=')
		dataCheckString.WriteString(params.Get(k))
	}

	// Compute HMAC-SHA256 hash
	secretKey := hmac.New(sha256.New, []byte("WebAppData"))
	secretKey.Write([]byte(token))
	secret := secretKey.Sum(nil)

	h := hmac.New(sha256.New, secret)
	h.Write([]byte(dataCheckString.String()))
	hash := hex.EncodeToString(h.Sum(nil))

	params.Set("hash", hash)
	return params.Encode()
}

func TestValidateInitData_Valid(t *testing.T) {
	allowed := map[int64]bool{42: true}
	initData := generateTestInitData(t, testBotToken, 42, time.Now())

	userID, err := validateTelegramInitData(initData, testBotToken, allowed)
	require.NoError(t, err)
	assert.Equal(t, int64(42), userID)
}
```

- [ ] **Step 2: Run test to verify it passes**

Run: `go test -C /Users/velom/GolandProjects/library -v -run TestValidateInitData_Valid ./internal/bot/`
Expected: PASS

- [ ] **Step 3: Add negative test cases**

Append to `internal/bot/auth_test.go`:

```go
func TestValidateInitData_EmptyString(t *testing.T) {
	allowed := map[int64]bool{42: true}
	_, err := validateTelegramInitData("", testBotToken, allowed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing initData")
}

func TestValidateInitData_TamperedHash(t *testing.T) {
	allowed := map[int64]bool{42: true}
	initData := generateTestInitData(t, testBotToken, 42, time.Now())
	// Replace last char of hash to tamper it
	initData = strings.Replace(initData, "hash=", "hash=00", 1)

	_, err := validateTelegramInitData(initData, testBotToken, allowed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid hash")
}

func TestValidateInitData_ExpiredAuthDate(t *testing.T) {
	allowed := map[int64]bool{42: true}
	initData := generateTestInitData(t, testBotToken, 42, time.Now().Add(-25*time.Hour))

	_, err := validateTelegramInitData(initData, testBotToken, allowed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too old")
}

func TestValidateInitData_UserNotAllowed(t *testing.T) {
	allowed := map[int64]bool{99: true} // userID 42 is not allowed
	initData := generateTestInitData(t, testBotToken, 42, time.Now())

	_, err := validateTelegramInitData(initData, testBotToken, allowed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestValidateInitData_WrongToken(t *testing.T) {
	allowed := map[int64]bool{42: true}
	initData := generateTestInitData(t, testBotToken, 42, time.Now())

	_, err := validateTelegramInitData(initData, "wrong-token", allowed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid hash")
}

func TestValidateInitData_MissingUserField(t *testing.T) {
	// Build initData without a "user" parameter
	params := url.Values{}
	params.Set("auth_date", fmt.Sprintf("%d", time.Now().Unix()))
	params.Set("query_id", "test-query-id")

	var keys []string
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var dcs strings.Builder
	for i, k := range keys {
		if i > 0 {
			dcs.WriteByte('\n')
		}
		dcs.WriteString(k + "=" + params.Get(k))
	}
	secretKey := hmac.New(sha256.New, []byte("WebAppData"))
	secretKey.Write([]byte(testBotToken))
	h := hmac.New(sha256.New, secretKey.Sum(nil))
	h.Write([]byte(dcs.String()))
	params.Set("hash", hex.EncodeToString(h.Sum(nil)))

	allowed := map[int64]bool{42: true}
	_, err := validateTelegramInitData(params.Encode(), testBotToken, allowed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing user")
}
```

- [ ] **Step 4: Run all auth tests**

Run: `go test -C /Users/velom/GolandProjects/library -v -run TestValidateInitData ./internal/bot/`
Expected: all 6 tests PASS

- [ ] **Step 5: Commit**

```bash
git -C /Users/velom/GolandProjects/library add internal/bot/auth_test.go
git -C /Users/velom/GolandProjects/library commit -m "test: add validateTelegramInitData unit tests with initData generator helper"
```

---

### Task 3: HTTP endpoint tests (polling mode)

**Files:**
- Create: `internal/bot/http_test.go`

Tests API endpoints with `webhookMode: false` (auth skipped). Uses `httptest.NewRecorder` with mock storage.

- [ ] **Step 1: Write test helpers and GET /api/books test**

Create `internal/bot/http_test.go`:

```go
package bot

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"library/internal/models"
	"library/internal/storage/stubs"
)

// newTestHTTPServer creates an HTTPServer in polling mode (auth skipped) with mock storage.
// notificationChatID is 0 — non-zero would call bot.api (nil) and panic.
func newTestHTTPServer(t *testing.T) (*HTTPServer, *stubs.MockDB) {
	t.Helper()
	mockDB := stubs.NewMockDB()
	mockDB.Initialize(nil)

	b := &Bot{
		db:                 mockDB,
		allowedUsers:       map[int64]bool{123: true},
		states:             make(map[int64]*ConversationState),
		statesMu:           sync.RWMutex{},
		logger:             zap.NewNop(),
		notificationChatID: 0,
	}
	return &HTTPServer{bot: b, webhookMode: false}, mockDB
}

func TestHandleBooks(t *testing.T) {
	hs, _ := newTestHTTPServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/books", nil)
	rec := httptest.NewRecorder()

	hs.handleBooks(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var books []models.Book
	err := json.NewDecoder(rec.Body).Decode(&books)
	require.NoError(t, err)
	assert.NotEmpty(t, books)
	// MockDB.Initialize creates 10 readable books
	assert.Len(t, books, 10)
}
```

- [ ] **Step 2: Run test to verify it passes**

Run: `go test -C /Users/velom/GolandProjects/library -v -run TestHandleBooks ./internal/bot/`
Expected: PASS

- [ ] **Step 3: Add GET /api/participants test**

Append to `internal/bot/http_test.go`:

```go
func TestHandleParticipants(t *testing.T) {
	hs, _ := newTestHTTPServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/participants", nil)
	rec := httptest.NewRecorder()

	hs.handleParticipants(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var participants []models.Participant
	err := json.NewDecoder(rec.Body).Decode(&participants)
	require.NoError(t, err)
	// MockDB.Initialize creates 4 participants: Alice, Bob (children), Mom, Dad (parents)
	assert.Len(t, participants, 4)

	// Verify we have both children and parents
	hasChild := false
	hasParent := false
	for _, p := range participants {
		if p.IsParent {
			hasParent = true
		} else {
			hasChild = true
		}
	}
	assert.True(t, hasChild, "should have at least one child")
	assert.True(t, hasParent, "should have at least one parent")
}
```

- [ ] **Step 4: Add POST /api/events tests (success + error cases)**

Append to `internal/bot/http_test.go`:

```go
func TestHandleEvents_Success(t *testing.T) {
	hs, mockDB := newTestHTTPServer(t)

	body := `{"date":"2026-03-23","book_name":"Book 1","participant_name":"Alice"}`
	req := httptest.NewRequest(http.MethodPost, "/api/events", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	hs.handleEvents(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]string
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "success", resp["status"])

	// Verify event was stored
	events, err := mockDB.GetLastEvents(nil, 1)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "Book 1", events[0].BookName)
	assert.Equal(t, "Alice", events[0].ParticipantName)
}

func TestHandleEvents_InvalidJSON(t *testing.T) {
	hs, _ := newTestHTTPServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/events", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	hs.handleEvents(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleEvents_MissingFields(t *testing.T) {
	hs, _ := newTestHTTPServer(t)

	body := `{"date":"2026-03-23","book_name":"Book 1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/events", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	hs.handleEvents(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleEvents_EmptyFields(t *testing.T) {
	hs, _ := newTestHTTPServer(t)

	// Valid JSON but all fields empty — hits the "Missing required fields" branch
	body := `{"date":"","book_name":"","participant_name":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/events", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	hs.handleEvents(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleEvents_WrongMethod(t *testing.T) {
	hs, _ := newTestHTTPServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	rec := httptest.NewRecorder()

	hs.handleEvents(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}
```

- [ ] **Step 5: Add GET /web-app test**

Append to `internal/bot/http_test.go`:

```go
func TestHandleIndex(t *testing.T) {
	hs, _ := newTestHTTPServer(t)

	req := httptest.NewRequest(http.MethodGet, "/web-app", nil)
	rec := httptest.NewRecorder()

	hs.handleIndex(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))
	assert.Contains(t, rec.Body.String(), "<!DOCTYPE html>")
}
```

- [ ] **Step 6: Run all endpoint tests**

Run: `go test -C /Users/velom/GolandProjects/library -v -run "TestHandle" ./internal/bot/`
Expected: all 7 tests PASS

- [ ] **Step 7: Commit**

```bash
git -C /Users/velom/GolandProjects/library add internal/bot/http_test.go
git -C /Users/velom/GolandProjects/library commit -m "test: add HTTP endpoint tests for Mini App API (polling mode)"
```

---

### Task 4: Auth middleware tests (webhook mode)

**Files:**
- Modify: `internal/bot/http_test.go` (append middleware tests)

Tests the `authMiddleware` integration with webhook mode enabled.

- [ ] **Step 1: Add webhook test helper and middleware tests**

Append to `internal/bot/http_test.go`:

```go
// newTestHTTPServerWebhook creates an HTTPServer in webhook mode with a known botToken.
func newTestHTTPServerWebhook(t *testing.T) *HTTPServer {
	t.Helper()
	hs, _ := newTestHTTPServer(t)
	hs.webhookMode = true
	hs.botToken = testBotToken // from auth_test.go (same package)
	return hs
}

func TestAuthMiddleware_NoHeader(t *testing.T) {
	hs := newTestHTTPServerWebhook(t)

	handlerCalled := false
	handler := hs.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	})

	req := httptest.NewRequest(http.MethodGet, "/api/books", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.False(t, handlerCalled)
}

func TestAuthMiddleware_InvalidInitData(t *testing.T) {
	hs := newTestHTTPServerWebhook(t)

	handlerCalled := false
	handler := hs.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	})

	req := httptest.NewRequest(http.MethodGet, "/api/books", nil)
	req.Header.Set("Authorization", "tma invalid-data")
	rec := httptest.NewRecorder()

	handler(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.False(t, handlerCalled)
}

func TestAuthMiddleware_ValidInitData(t *testing.T) {
	hs := newTestHTTPServerWebhook(t)

	handlerCalled := false
	handler := hs.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// userID 123 is in allowedUsers (set by newTestHTTPServer)
	initData := generateTestInitData(t, testBotToken, 123, time.Now())
	req := httptest.NewRequest(http.MethodGet, "/api/books", nil)
	req.Header.Set("Authorization", "tma "+initData)
	rec := httptest.NewRecorder()

	handler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, handlerCalled)
}

func TestAuthMiddleware_UserNotAllowed(t *testing.T) {
	hs := newTestHTTPServerWebhook(t)

	handlerCalled := false
	handler := hs.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	})

	// userID 999 is NOT in allowedUsers
	initData := generateTestInitData(t, testBotToken, 999, time.Now())
	req := httptest.NewRequest(http.MethodGet, "/api/books", nil)
	req.Header.Set("Authorization", "tma "+initData)
	rec := httptest.NewRecorder()

	handler(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.False(t, handlerCalled)
}

func TestAuthMiddleware_PollingModeSkipsAuth(t *testing.T) {
	hs, _ := newTestHTTPServer(t) // polling mode (webhookMode: false)

	handlerCalled := false
	handler := hs.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// No Authorization header — should still work in polling mode
	req := httptest.NewRequest(http.MethodGet, "/api/books", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, handlerCalled)
}
```

- [ ] **Step 2: Add `time` import to http_test.go**

Ensure `"time"` is in the import block of `http_test.go` (needed for `time.Now()` in middleware tests).

- [ ] **Step 3: Run all tests**

Run: `go test -C /Users/velom/GolandProjects/library -v ./internal/bot/`
Expected: all tests PASS (auth tests + endpoint tests + middleware tests + existing bot tests)

- [ ] **Step 4: Commit**

```bash
git -C /Users/velom/GolandProjects/library add internal/bot/http_test.go
git -C /Users/velom/GolandProjects/library commit -m "test: add auth middleware tests for webhook mode"
```

---

### Task 5: Telegram SDK mock shim in `index.html`

**Files:**
- Modify: `web/index.html:223-227`

Add a shim that provides a fake `window.Telegram.WebApp` when running outside Telegram.

- [ ] **Step 1: Add shim before `const tg` line**

In `web/index.html`, right after the `<script>` tag (line 223), before `const tg = window.Telegram.WebApp;` (line 225), insert:

```javascript
        // Dev mode: provide mock Telegram SDK when running outside Telegram.
        // Inside Telegram, initData is always a non-empty string so this never activates.
        // If the CDN script fails to load, the shim activates but webhook-mode auth
        // will reject the empty initData — no security bypass possible.
        if (!window.Telegram?.WebApp?.initData) {
            console.warn("[DEV] Telegram WebApp SDK not found, using mock");
            window.Telegram = { WebApp: {
                initData: "",
                initDataUnsafe: { user: { id: 0, first_name: "Dev" } },
                themeParams: {},
                colorScheme: "light",
                expand: function() {},
                ready: function() {},
                close: function() { window.close(); },
                MainButton: { show: function(){}, hide: function(){}, setText: function(){} },
            }};
        }
```

The existing code (`const tg = window.Telegram.WebApp; tg.expand(); tg.ready();`) continues to work unchanged.

- [ ] **Step 2: Verify the embedded file compiles**

Run: `go build -C /Users/velom/GolandProjects/library ./...`
Expected: no errors (the embed directive picks up the modified file)

- [ ] **Step 3: Commit**

```bash
git -C /Users/velom/GolandProjects/library add web/index.html
git -C /Users/velom/GolandProjects/library commit -m "feat: add Telegram SDK mock shim for local browser development"
```

---

### Task 6: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Add "Local Mini App Development" section**

Add after the "Common Commands" section in `CLAUDE.md`:

```markdown
### Local Mini App Development

**Browser (daily development):**
1. `make run-dev` — starts bot in polling mode with ClickHouse testcontainer
2. Open `http://localhost:8081/web-app` in browser
3. A built-in shim provides a mock Telegram SDK; auth is skipped in polling mode

**Telegram (integration check):**
1. `make run-dev` in one terminal
2. `cloudflared tunnel --url http://localhost:8081` in another terminal
3. Set the tunnel HTTPS URL as Mini App URL in BotFather
4. Open Mini App in Telegram for full integration testing
```

- [ ] **Step 2: Commit**

```bash
git -C /Users/velom/GolandProjects/library add CLAUDE.md
git -C /Users/velom/GolandProjects/library commit -m "docs: add local Mini App development instructions to CLAUDE.md"
```

---

### Task 7: Final verification

- [ ] **Step 1: Run full test suite**

Run: `go test -C /Users/velom/GolandProjects/library -v ./...`
Expected: all tests pass, including new auth, endpoint, and middleware tests

- [ ] **Step 2: Verify build**

Run: `go build -C /Users/velom/GolandProjects/library ./...`
Expected: clean build, no errors
