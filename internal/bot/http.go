package bot

import (
	"context"
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
)

// HTTPServer handles HTTP requests for the Mini App
type HTTPServer struct {
	bot    *Bot
	server *http.Server
}

// NewHTTPServer creates a new HTTP server for the Mini App
func NewHTTPServer(bot *Bot, port int) *HTTPServer {
	mux := http.NewServeMux()

	hs := &HTTPServer{
		bot: bot,
		server: &http.Server{
			Addr:    fmt.Sprintf(":%d", port),
			Handler: mux,
		},
	}

	// Static file serving
	mux.HandleFunc("/", hs.handleIndex)

	// API endpoints
	mux.HandleFunc("/api/books", hs.handleBooks)
	mux.HandleFunc("/api/participants", hs.handleParticipants)
	mux.HandleFunc("/api/events", hs.handleEvents)

	return hs
}

// Start starts the HTTP server
func (hs *HTTPServer) Start() error {
	hs.bot.logger.Info("Starting HTTP server", zap.String("addr", hs.server.Addr))
	return hs.server.ListenAndServe()
}

// Shutdown gracefully shuts down the HTTP server
func (hs *HTTPServer) Shutdown(ctx context.Context) error {
	hs.bot.logger.Info("Shutting down HTTP server")
	return hs.server.Shutdown(ctx)
}

// handleIndex serves the Mini App HTML
func (hs *HTTPServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	http.ServeFile(w, r, "web/index.html")
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
func (hs *HTTPServer) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "success",
		})
	})(w, r)
}
