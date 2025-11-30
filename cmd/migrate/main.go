package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"
)

func main() {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found, using existing environment variables")
	}

	// Read database configuration from environment
	host := getEnv("CLICKHOUSE_HOST", "localhost")
	port := getEnv("CLICKHOUSE_PORT", "9000")
	database := getEnv("CLICKHOUSE_DATABASE", "default")
	user := getEnv("CLICKHOUSE_USER", "default")
	password := getEnv("CLICKHOUSE_PASSWORD", "")
	useTLS := getEnv("CLICKHOUSE_USE_TLS", "false")

	// Construct ClickHouse DSN
	// Format: clickhouse://username:password@host:port/database?parameters
	dsn := fmt.Sprintf("clickhouse://%s:%s@%s:%s/%s?dial_timeout=10s&max_execution_time=60",
		user, password, host, port, database)

	if useTLS == "true" {
		dsn += "&secure=true"
	}

	// Open database connection
	db, err := sql.Open("clickhouse", dsn)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Test connection
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	log.Println("Connected to ClickHouse successfully")

	// Get command from arguments (default to "up")
	command := "up"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	// Set migrations directory
	migrationsDir := "./migrations"

	// Set goose dialect to clickhouse
	if err := goose.SetDialect("clickhouse"); err != nil {
		log.Fatalf("Failed to set dialect: %v", err)
	}

	// Run goose command
	log.Printf("Running migrations: %s", command)
	switch command {
	case "up":
		if err := goose.Up(db, migrationsDir); err != nil {
			log.Fatalf("Failed to run migrations: %v", err)
		}
		log.Println("Migrations completed successfully")
	case "down":
		if err := goose.Down(db, migrationsDir); err != nil {
			log.Fatalf("Failed to rollback migration: %v", err)
		}
		log.Println("Rollback completed successfully")
	case "status":
		if err := goose.Status(db, migrationsDir); err != nil {
			log.Fatalf("Failed to get migration status: %v", err)
		}
	case "version":
		version, err := goose.GetDBVersion(db)
		if err != nil {
			log.Fatalf("Failed to get version: %v", err)
		}
		log.Printf("Current migration version: %d", version)
	case "create":
		if len(os.Args) < 3 {
			log.Fatal("Usage: migrate create <migration_name>")
		}
		migrationName := os.Args[2]
		if err := goose.Create(db, migrationsDir, migrationName, "sql"); err != nil {
			log.Fatalf("Failed to create migration: %v", err)
		}
		log.Printf("Created migration: %s", migrationName)
	default:
		log.Fatalf("Unknown command: %s. Available commands: up, down, status, version, create", command)
	}
}

// getEnv retrieves environment variable or returns default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
