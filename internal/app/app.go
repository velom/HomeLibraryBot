package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"

	"library/internal/bot"
	"library/internal/config"
	"library/internal/storage"
	"library/internal/storage/ch"
	"library/internal/storage/stubs"
)

// App represents the application
type App struct {
	config *config.Config
	db     storage.Storage
	bot    *bot.Bot
	server *http.Server
}

// New creates and initializes a new application instance
func New() (*App, error) {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	// Load configuration from environment variables
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	app := &App{config: cfg}

	log.Println("Starting Home Library Bot...")

	// Initialize database
	if err := app.initDatabase(); err != nil {
		return nil, err
	}

	// Initialize bot
	if err := app.initBot(); err != nil {
		return nil, err
	}

	// Initialize HTTP server
	app.initHTTPServer()

	return app, nil
}

// initDatabase initializes the database connection
func (a *App) initDatabase() error {
	var db storage.Storage
	if a.config.UseMockDB {
		log.Println("Using mock database")
		db = stubs.NewMockDB()
	} else {
		tlsStatus := "without TLS"
		if a.config.ClickHouseUseTLS {
			tlsStatus = "with TLS"
		}
		log.Printf("Connecting to ClickHouse at %s:%d (database: %s, user: %s, %s)",
			a.config.ClickHouseHost, a.config.ClickHousePort, a.config.ClickHouseDatabase, a.config.ClickHouseUser, tlsStatus)
		clickhouseDB, err := ch.NewClickHouseDB(
			a.config.ClickHouseHost,
			a.config.ClickHousePort,
			a.config.ClickHouseDatabase,
			a.config.ClickHouseUser,
			a.config.ClickHousePassword,
			a.config.ClickHouseUseTLS,
		)
		if err != nil {
			return fmt.Errorf("failed to connect to ClickHouse: %w", err)
		}
		db = clickhouseDB
	}

	// Initialize database schema and default data
	ctx := context.Background()
	if err := db.Initialize(ctx); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	log.Println("Database initialized successfully")

	a.db = db
	return nil
}

// initBot initializes the Telegram bot
func (a *App) initBot() error {
	telegramBot, err := bot.NewBot(a.config.TelegramToken, a.db, a.config.AllowedUserIDs)
	if err != nil {
		return fmt.Errorf("failed to create Telegram bot: %w", err)
	}
	log.Printf("Bot created successfully. Allowed users: %v", a.config.AllowedUserIDs)

	a.bot = telegramBot
	return nil
}

// initHTTPServer initializes the HTTP server for health checks and webhook
func (a *App) initHTTPServer() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Default port
	}

	// Health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})

	// Root endpoint
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		mode := "polling"
		if a.config.WebhookMode {
			mode = "webhook"
		}
		fmt.Fprintf(w, "Home Library Bot is running (mode: %s)", mode)
	})

	// Webhook endpoint (only used in webhook mode)
	http.HandleFunc("/telegram-webhook", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var update tgbotapi.Update
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			log.Printf("Error decoding webhook update: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Process update in background to respond quickly to Telegram
		go a.bot.HandleWebhookUpdate(update)

		w.WriteHeader(http.StatusOK)
	})

	a.server = &http.Server{
		Addr:         ":" + port,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Start HTTP server in background
	go func() {
		log.Printf("Starting HTTP server on port %s", port)
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()
}

// Run starts the application and blocks until shutdown
func (a *App) Run() error {
	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start bot in appropriate mode
	if a.config.WebhookMode {
		// Webhook mode: configure webhook and wait for HTTP requests
		log.Printf("Starting bot in WEBHOOK mode (URL: %s)", a.config.WebhookURL)
		if err := a.bot.StartWebhook(a.config.WebhookURL); err != nil {
			return fmt.Errorf("failed to setup webhook: %w", err)
		}
		log.Println("Webhook configured. Bot will receive updates via HTTP endpoint /telegram-webhook")
	} else {
		// Polling mode: actively poll Telegram servers
		go func() {
			log.Println("Starting bot in POLLING mode...")
			if err := a.bot.Start(); err != nil {
				log.Fatalf("Failed to start bot: %v", err)
			}
		}()
	}

	// Wait for interrupt signal
	<-sigChan

	log.Println("Shutting down...")
	return a.Shutdown()
}

// Shutdown gracefully shuts down the application
func (a *App) Shutdown() error {
	// Shutdown HTTP server gracefully
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	// Close database
	if err := a.db.Close(); err != nil {
		log.Printf("Error closing database: %v", err)
		return err
	}

	log.Println("Shutdown complete")
	return nil
}
