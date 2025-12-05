package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/joho/godotenv"
	"go.uber.org/zap"

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
	logger *zap.Logger
}

// New creates and initializes a new application instance
func New() (*App, error) {
	// Initialize zap logger with Development config
	logger, err := zap.NewDevelopment()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		logger.Info("No .env file found, using system environment variables")
	}

	// Load configuration from environment variables
	cfg, err := config.LoadFromEnv()
	if err != nil {
		logger.Error("Failed to load configuration", zap.Error(err))
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	app := &App{
		config: cfg,
		logger: logger,
	}

	logger.Info("Starting Home Library Bot...")

	// Initialize database
	if err := app.initDatabase(); err != nil {
		logger.Error("Failed to initialize database", zap.Error(err))
		return nil, err
	}

	// Initialize bot
	if err := app.initBot(); err != nil {
		logger.Error("Failed to initialize bot", zap.Error(err))
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
		a.logger.Info("Using mock database")
		db = stubs.NewMockDB()
	} else {
		a.logger.Info("Connecting to ClickHouse",
			zap.String("host", a.config.ClickHouseHost),
			zap.Int("port", a.config.ClickHousePort),
			zap.String("database", a.config.ClickHouseDatabase),
			zap.String("user", a.config.ClickHouseUser),
			zap.Bool("tls", a.config.ClickHouseUseTLS),
		)
		clickhouseDB, err := ch.NewClickHouseDB(
			a.config.ClickHouseHost,
			a.config.ClickHousePort,
			a.config.ClickHouseDatabase,
			a.config.ClickHouseUser,
			a.config.ClickHousePassword,
			a.config.ClickHouseUseTLS,
		)
		if err != nil {
			a.logger.Error("Failed to connect to ClickHouse",
				zap.Error(err),
				zap.String("host", a.config.ClickHouseHost),
				zap.Int("port", a.config.ClickHousePort),
			)
			return fmt.Errorf("failed to connect to ClickHouse: %w", err)
		}
		db = clickhouseDB
	}

	// Initialize database schema and default data
	ctx := context.Background()
	if err := db.Initialize(ctx); err != nil {
		a.logger.Error("Failed to initialize database", zap.Error(err))
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	a.logger.Info("Database initialized successfully")

	a.db = db
	return nil
}

// initBot initializes the Telegram bot
func (a *App) initBot() error {
	telegramBot, err := bot.NewBot(a.config.TelegramToken, a.db, a.config.AllowedUserIDs, a.logger)
	if err != nil {
		a.logger.Error("Failed to create Telegram bot", zap.Error(err))
		return fmt.Errorf("failed to create Telegram bot: %w", err)
	}
	a.logger.Info("Bot created successfully",
		zap.Int64s("allowed_users", a.config.AllowedUserIDs),
	)

	a.bot = telegramBot
	return nil
}

// initHTTPServer initializes the HTTP server for health checks, webhook, and Mini App
func (a *App) initHTTPServer() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Default port
	}

	// Create HTTP mux
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})

	// Root endpoint
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		mode := "polling"
		if a.config.WebhookMode {
			mode = "webhook"
		}
		fmt.Fprintf(w, "Home Library Bot is running (mode: %s)", mode)
	})

	// Webhook endpoint (only used in webhook mode)
	mux.HandleFunc("/telegram-webhook", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			a.logger.Warn("Invalid method for webhook endpoint",
				zap.String("method", r.Method),
				zap.String("remote_addr", r.RemoteAddr),
			)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var update models.Update
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			a.logger.Error("Error decoding webhook update",
				zap.Error(err),
				zap.String("remote_addr", r.RemoteAddr),
			)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		a.logger.Debug("Received webhook update", zap.Int("update_id", int(update.ID)))

		// Process update in background to respond quickly to Telegram
		go a.bot.HandleWebhookUpdate(context.Background(), &update)

		w.WriteHeader(http.StatusOK)
	})

	// Register Mini App routes (web-app and API endpoints)
	// Pass webhook mode to enable/disable authentication
	httpServer := bot.NewHTTPServer(a.bot, a.config.WebhookMode)
	httpServer.RegisterRoutes(mux)

	a.logger.Info("HTTP routes registered",
		zap.Bool("webhook_mode", a.config.WebhookMode),
		zap.String("auth_required", fmt.Sprintf("%v", a.config.WebhookMode)),
	)

	a.server = &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Start HTTP server in background
	go func() {
		a.logger.Info("Starting HTTP server", zap.String("port", port))
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logger.Error("HTTP server error", zap.Error(err))
		}
	}()
}

// Run starts the application and blocks until shutdown
func (a *App) Run() error {
	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start bot in appropriate mode
	ctx := context.Background()

	if a.config.WebhookMode {
		// Webhook mode: configure webhook and wait for HTTP requests
		a.logger.Info("Starting bot in WEBHOOK mode", zap.String("webhook_url", a.config.WebhookURL))
		if err := a.bot.StartWebhook(a.config.WebhookURL); err != nil {
			a.logger.Error("Failed to setup webhook", zap.Error(err), zap.String("webhook_url", a.config.WebhookURL))
			return fmt.Errorf("failed to setup webhook: %w", err)
		}
		a.logger.Info("Webhook configured. Bot will receive updates via HTTP endpoint /telegram-webhook")
	} else {
		// Polling mode: actively poll Telegram servers
		go func() {
			a.logger.Info("Starting bot in POLLING mode")
			if err := a.bot.Start(ctx); err != nil {
				a.logger.Fatal("Failed to start bot", zap.Error(err))
			}
		}()
	}

	// Wait for interrupt signal
	<-sigChan

	a.logger.Info("Shutting down...")
	return a.Shutdown()
}

// Shutdown gracefully shuts down the application
func (a *App) Shutdown() error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Shutdown bot (including its HTTP server)
	if a.bot != nil {
		if err := a.bot.Shutdown(shutdownCtx); err != nil {
			a.logger.Error("Bot shutdown error", zap.Error(err))
		}
	}

	// Shutdown app HTTP server gracefully
	if a.server != nil {
		if err := a.server.Shutdown(shutdownCtx); err != nil {
			a.logger.Error("HTTP server shutdown error", zap.Error(err))
		}
	}

	// Close database
	if err := a.db.Close(); err != nil {
		a.logger.Error("Error closing database", zap.Error(err))
		// Sync logger before returning error
		_ = a.logger.Sync()
		return err
	}

	a.logger.Info("Shutdown complete")
	// Sync logger to flush any buffered log entries
	_ = a.logger.Sync()
	return nil
}
