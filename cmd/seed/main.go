package main

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"time"

	"gopanel/internal/config"
	"gopanel/internal/platform/database"
	"gopanel/internal/platform/seed"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		logger.Error("invalid_configuration", "error", err)
		os.Exit(1)
	}
	if cfg.Env != "development" {
		logger.Error("seed_requires_development_environment")
		os.Exit(1)
	}
	count := 100
	if value := os.Getenv("SEED_COUNT"); value != "" {
		count, err = strconv.Atoi(value)
		if err != nil || count < 1 || count > 10000 {
			logger.Error("SEED_COUNT must be between 1 and 10000")
			os.Exit(1)
		}
	}
	db, err := database.Open(cfg)
	if err != nil {
		logger.Error("database_configuration_failed")
		os.Exit(1)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	result, err := seed.Run(ctx, db, seed.Options{Count: count, CredentialsPath: ".local/seed-credentials.json"})
	if err != nil {
		logger.Error("seed_failed", "error", err)
		os.Exit(1)
	}
	logger.Info("seed_complete", "already_seeded", result.AlreadySeeded, "tenants", result.Tenants, "users", result.Users, "memberships", result.Memberships, "customers", result.Customers, "products", result.Products, "credentials_file", ".local/seed-credentials.json")
}
