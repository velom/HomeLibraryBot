package bot

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
	"library/web"
)

// HTTPServer handles HTTP requests for the Mini App
type HTTPServer struct {
	bot         *Bot
	webhookMode bool // If false (polling mode), skip authentication for easier local dev
}

// NewHTTPServer creates a new HTTP server for the Mini App
func NewHTTPServer(bot *Bot, webhookMode bool) *HTTPServer {
	return &HTTPServer{
		bot:         bot,
		webhookMode: webhookMode,
	}
}

// RegisterRoutes registers Mini App routes on the provided mux
func (hs *HTTPServer) RegisterRoutes(mux *http.ServeMux) {
	// Static file serving for Mini App
	mux.HandleFunc("/web-app", hs.handleIndex)

	// API endpoints
	mux.HandleFunc("/api/books", hs.handleBooks)
	mux.HandleFunc("/api/participants", hs.handleParticipants)
	mux.HandleFunc("/api/events", hs.handleEvents)
}

// handleIndex serves the Mini App HTML from embedded filesystem
func (hs *HTTPServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/web-app" {
		http.NotFound(w, r)
		return
	}

	// Read the embedded index.html
	content, err := web.Content.ReadFile("index.html")
	if err != nil {
		hs.bot.logger.Error("Failed to read embedded index.html", zap.Error(err))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(content)
}

// validateTelegramInitData validates the Telegram Mini App initData
func (hs *HTTPServer) validateTelegramInitData(initData string) (int64, error) {
	if initData == "" {
		return 0, fmt.Errorf("missing initData")
	}

	// Parse the initData
	values, err := url.ParseQuery(initData)
	if err != nil {
		return 0, fmt.Errorf("invalid initData format: %w", err)
	}

	// Extract hash
	hash := values.Get("hash")
	if hash == "" {
		return 0, fmt.Errorf("missing hash in initData")
	}

	// Remove hash from values
	values.Del("hash")

	// Create data-check-string
	var keys []string
	for k := range values {
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
		dataCheckString.WriteString(values.Get(k))
	}

	// Create secret key
	secretKey := hmac.New(sha256.New, []byte("WebAppData"))
	secretKey.Write([]byte(hs.bot.api.Token()))
	secret := secretKey.Sum(nil)

	// Calculate hash
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(dataCheckString.String()))
	calculatedHash := hex.EncodeToString(h.Sum(nil))

	// Verify hash
	if calculatedHash != hash {
		return 0, fmt.Errorf("invalid hash")
	}

	// Check auth_date (data should be recent, within 24 hours)
	authDateStr := values.Get("auth_date")
	if authDateStr == "" {
		return 0, fmt.Errorf("missing auth_date")
	}

	var authDate int64
	fmt.Sscanf(authDateStr, "%d", &authDate)
	if time.Now().Unix()-authDate > 86400 {
		return 0, fmt.Errorf("initData is too old")
	}

	// Extract user ID
	userStr := values.Get("user")
	if userStr == "" {
		return 0, fmt.Errorf("missing user data")
	}

	var userData struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(userStr), &userData); err != nil {
		return 0, fmt.Errorf("invalid user data: %w", err)
	}

	// Check if user is allowed
	if !hs.bot.allowedUsers[userData.ID] {
		return 0, fmt.Errorf("user not allowed")
	}

	return userData.ID, nil
}

// authMiddleware validates Telegram Mini App authentication
// In polling mode (webhookMode=false), authentication is skipped for easier local development
func (hs *HTTPServer) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Skip authentication in polling mode (local development)
		if !hs.webhookMode {
			hs.bot.logger.Debug("Skipping authentication (polling mode)",
				zap.String("path", r.URL.Path),
				zap.String("remote_addr", r.RemoteAddr),
			)
			next(w, r)
			return
		}

		// Extract authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "tma ") {
			hs.bot.logger.Warn("Missing or invalid authorization header")
			http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		initData := strings.TrimPrefix(authHeader, "tma ")

		// Validate initData
		userID, err := hs.validateTelegramInitData(initData)
		if err != nil {
			hs.bot.logger.Warn("Failed to validate initData",
				zap.Error(err),
				zap.String("remote_addr", r.RemoteAddr),
			)
			http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		hs.bot.logger.Debug("Authenticated request",
			zap.Int64("user_id", userID),
			zap.String("path", r.URL.Path),
		)

		next(w, r)
	}
}

// handleBooks returns the list of readable books
func (hs *HTTPServer) handleBooks(w http.ResponseWriter, r *http.Request) {
	hs.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

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

// handleParticipants returns the list of participants
func (hs *HTTPServer) handleParticipants(w http.ResponseWriter, r *http.Request) {
	hs.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		participants, err := hs.bot.db.ListParticipants(r.Context())
		if err != nil {
			hs.bot.logger.Error("Failed to list participants", zap.Error(err))
			http.Error(w, `{"error":"Failed to fetch participants"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(participants)
	})(w, r)
}

// CreateEventRequest represents the request body for creating an event
type CreateEventRequest struct {
	Date            string `json:"date"`
	BookName        string `json:"book_name"`
	ParticipantName string `json:"participant_name"`
}

// handleEvents creates a new reading event
func (hs *HTTPServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	hs.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		var req CreateEventRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			hs.bot.logger.Warn("Failed to decode request body", zap.Error(err))
			http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
			return
		}

		// Validate request
		if req.Date == "" || req.BookName == "" || req.ParticipantName == "" {
			http.Error(w, `{"error":"Missing required fields"}`, http.StatusBadRequest)
			return
		}

		// Parse date
		date, err := time.Parse("2006-01-02", req.Date)
		if err != nil {
			hs.bot.logger.Warn("Failed to parse date",
				zap.Error(err),
				zap.String("date", req.Date),
			)
			http.Error(w, `{"error":"Invalid date format"}`, http.StatusBadRequest)
			return
		}

		// Create event
		err = hs.bot.db.CreateEvent(r.Context(), date, req.BookName, req.ParticipantName)
		if err != nil {
			hs.bot.logger.Error("Failed to create event",
				zap.Error(err),
				zap.String("book", req.BookName),
				zap.String("participant", req.ParticipantName),
			)
			http.Error(w, `{"error":"Failed to create event"}`, http.StatusInternalServerError)
			return
		}

		hs.bot.logger.Info("Event created via Mini App",
			zap.String("date", req.Date),
			zap.String("book", req.BookName),
			zap.String("participant", req.ParticipantName),
		)

		// Send notification to configured chat
		if hs.bot.notificationChatID != 0 {
			notificationText := fmt.Sprintf("New reading event!\n\nDate: %s\nBook: %s\nReader: %s",
				date.Format("2006-01-02"), req.BookName, req.ParticipantName)
			hs.bot.sendMessageInThread(r.Context(), hs.bot.notificationChatID, notificationText, hs.bot.notificationThreadID)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "success",
		})
	})(w, r)
}
