package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds the application configuration
type Config struct {
	TelegramToken  string
	AllowedUserIDs []int64

	// Bot mode configuration
	WebhookMode bool   // If true, use webhook mode; if false, use polling mode
	WebhookURL  string // URL for webhook (required if WebhookMode is true)

	// ClickHouse configuration
	ClickHouseHost     string
	ClickHousePort     int
	ClickHouseDatabase string
	ClickHouseUser     string
	ClickHousePassword string
	ClickHouseUseTLS   bool

	UseMockDB bool
}

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv() (*Config, error) {
	config := &Config{}

	// Telegram Bot Token (required)
	config.TelegramToken = os.Getenv("TELEGRAM_BOT_TOKEN")
	if config.TelegramToken == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}

	// Allowed User IDs (required)
	allowedIDsStr := os.Getenv("ALLOWED_USER_IDS")
	if allowedIDsStr == "" {
		return nil, fmt.Errorf("ALLOWED_USER_IDS is required (comma-separated list of Telegram user IDs)")
	}

	idStrs := strings.Split(allowedIDsStr, ",")
	for _, idStr := range idStrs {
		id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid user ID in ALLOWED_USER_IDS: %s", idStr)
		}
		config.AllowedUserIDs = append(config.AllowedUserIDs, id)
	}

	// Bot mode configuration
	config.WebhookMode = os.Getenv("WEBHOOK_MODE") == "true"
	if config.WebhookMode {
		config.WebhookURL = os.Getenv("WEBHOOK_URL")
		if config.WebhookURL == "" {
			return nil, fmt.Errorf("WEBHOOK_URL is required when WEBHOOK_MODE is true")
		}
	}

	// Use Mock DB (default: false)
	config.UseMockDB = os.Getenv("USE_MOCK_DB") == "true"

	// ClickHouse configuration (required if not using mock)
	if !config.UseMockDB {
		config.ClickHouseHost = os.Getenv("CLICKHOUSE_HOST")
		if config.ClickHouseHost == "" {
			return nil, fmt.Errorf("CLICKHOUSE_HOST is required when USE_MOCK_DB is not set")
		}

		portStr := os.Getenv("CLICKHOUSE_PORT")
		if portStr == "" {
			config.ClickHousePort = 9000 // Default ClickHouse native port
		} else {
			port, err := strconv.Atoi(portStr)
			if err != nil {
				return nil, fmt.Errorf("invalid CLICKHOUSE_PORT: %w", err)
			}
			config.ClickHousePort = port
		}

		config.ClickHouseDatabase = os.Getenv("CLICKHOUSE_DATABASE")
		if config.ClickHouseDatabase == "" {
			config.ClickHouseDatabase = "default"
		}

		config.ClickHouseUser = os.Getenv("CLICKHOUSE_USER")
		if config.ClickHouseUser == "" {
			config.ClickHouseUser = "default"
		}

		config.ClickHousePassword = os.Getenv("CLICKHOUSE_PASSWORD")
		// Password is optional, can be empty

		config.ClickHouseUseTLS = os.Getenv("CLICKHOUSE_USE_TLS") == "true"
	}

	return config, nil
}
