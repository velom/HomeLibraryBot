package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/testcontainers/testcontainers-go/modules/clickhouse"

	"library/internal/app"
)

func main() {
	ctx := context.Background()

	log.Println("Starting ClickHouse testcontainer...")

	// Start ClickHouse container
	clickhouseContainer, err := clickhouse.Run(ctx,
		"clickhouse/clickhouse-server:latest",
		clickhouse.WithUsername("default"),
		clickhouse.WithPassword("devpassword"),
		clickhouse.WithDatabase("default"),
	)
	if err != nil {
		log.Fatalf("Failed to start ClickHouse container: %v", err)
	}

	// Ensure container cleanup on exit
	defer func() {
		log.Println("Stopping ClickHouse container...")
		if err := clickhouseContainer.Terminate(ctx); err != nil {
			log.Printf("Failed to terminate container: %v", err)
		}
	}()

	// Get connection details
	host, err := clickhouseContainer.Host(ctx)
	if err != nil {
		log.Fatalf("Failed to get container host: %v", err)
	}

	port, err := clickhouseContainer.MappedPort(ctx, "9000/tcp")
	if err != nil {
		log.Fatalf("Failed to get container port: %v", err)
	}

	log.Printf("ClickHouse started at %s:%s", host, port.Port())

	// Set environment variables for the application
	os.Setenv("CLICKHOUSE_HOST", host)
	os.Setenv("CLICKHOUSE_PORT", port.Port())
	os.Setenv("CLICKHOUSE_DATABASE", "default")
	os.Setenv("CLICKHOUSE_USER", "default")
	os.Setenv("CLICKHOUSE_PASSWORD", "devpassword")
	os.Setenv("CLICKHOUSE_USE_TLS", "false")
	os.Setenv("USE_MOCK_DB", "false")
	os.Setenv("WEBHOOK_MODE", "false")

	// Set PORT for HTTP server if not already set
	if os.Getenv("PORT") == "" {
		os.Setenv("PORT", "8080")
	}

	// Ensure TELEGRAM_BOT_TOKEN and ALLOWED_USER_IDS are set
	if os.Getenv("TELEGRAM_BOT_TOKEN") == "" {
		log.Println("⚠️  TELEGRAM_BOT_TOKEN not set. Please set it in your .env file or environment.")
		log.Println("   The bot will fail to start without a valid token.")
	}

	if os.Getenv("ALLOWED_USER_IDS") == "" {
		log.Println("⚠️  ALLOWED_USER_IDS not set. Please set it in your .env file or environment.")
		log.Println("   The bot will not accept any commands without allowed user IDs.")
	}

	log.Println("Starting application with ClickHouse backend...")
	fmt.Println()

	// Create and initialize application
	application, err := app.New()
	if err != nil {
		log.Fatalf("Failed to create application: %v", err)
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Run application in background
	errChan := make(chan error, 1)
	go func() {
		errChan <- application.Run()
	}()

	// Wait for shutdown signal or error
	select {
	case <-sigChan:
		log.Println("Received shutdown signal")
	case err := <-errChan:
		if err != nil {
			log.Fatalf("Application error: %v", err)
		}
	}
}
