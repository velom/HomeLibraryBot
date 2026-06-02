package bot

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

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
		pendingBooks:       make(map[string]*pendingBook),
		pendingEvents:      make(map[string]*pendingEvent),
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
	assert.Len(t, books, 10)
}

func TestHandleParticipants(t *testing.T) {
	hs, _ := newTestHTTPServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/participants", nil)
	rec := httptest.NewRecorder()

	hs.handleParticipants(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var participants []models.Participant
	err := json.NewDecoder(rec.Body).Decode(&participants)
	require.NoError(t, err)
	assert.Len(t, participants, 4)

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

func TestHandleCreateBook_NoNotificationChat_CreatesDirectly(t *testing.T) {
	hs, mockDB := newTestHTTPServer(t) // notificationChatID == 0

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

func TestHandleEvents_WithNotificationChat_Pending(t *testing.T) {
	hs, mockDB := newTestHTTPServer(t)
	hs.bot.notificationChatID = 999 // api is nil, so the approval message send is a guarded no-op

	body := `{"date":"2026-03-23","book_name":"The Hobbit","participant_name":"Alice"}`
	req := httptest.NewRequest(http.MethodPost, "/api/events", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	hs.handleEvents(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	var resp map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "pending", resp["status"])

	// Event must NOT be created yet — it is awaiting approval.
	events, err := mockDB.GetLastEvents(nil, 10)
	require.NoError(t, err)
	assert.Empty(t, events)
	assert.Len(t, hs.bot.pendingEvents, 1)
}

func TestHandleCreateBook_WithNotificationChat_Pending(t *testing.T) {
	hs, mockDB := newTestHTTPServer(t)
	hs.bot.notificationChatID = 999 // api is nil, so the approval message send is a guarded no-op

	body := `{"name":"Pending Title","labels":["x"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/books", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	hs.handleCreateBook(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	var resp map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "pending", resp["status"])

	// Book must NOT exist yet — it is awaiting approval.
	books, err := mockDB.ListReadableBooks(nil)
	require.NoError(t, err)
	for _, bk := range books {
		assert.NotEqual(t, "Pending Title", bk.Name)
	}
	assert.Len(t, hs.bot.pendingBooks, 1)
}

func TestHandleCreateBook_EmptyName(t *testing.T) {
	hs, _ := newTestHTTPServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/books", bytes.NewBufferString(`{"name":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	hs.handleCreateBook(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleBooks_WrongMethod(t *testing.T) {
	hs, _ := newTestHTTPServer(t)

	req := httptest.NewRequest(http.MethodPut, "/api/books", nil)
	rec := httptest.NewRecorder()

	hs.handleBooks(rec, req) // dispatcher routes by method

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestHandleLabels(t *testing.T) {
	hs, mockDB := newTestHTTPServer(t)
	require.NoError(t, mockDB.AddLabelToBook(nil, "The Hobbit", "sci-fi"))
	require.NoError(t, mockDB.AddLabelToBook(nil, "Matilda", "fantasy"))

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
	require.NoError(t, mockDB.AddLabelToBook(nil, "The Hobbit", "skip-me"))

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
		assert.NotEqual(t, "The Hobbit", s.BookName, "excluded label's book must not appear")
	}
}

func TestHandleIndex(t *testing.T) {
	hs, _ := newTestHTTPServer(t)

	req := httptest.NewRequest(http.MethodGet, "/web-app", nil)
	rec := httptest.NewRecorder()

	hs.handleIndex(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))
	assert.Contains(t, rec.Body.String(), "<!DOCTYPE html>")
}

// --- Auth middleware tests (webhook mode) ---

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
