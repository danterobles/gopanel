package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"gopanel/internal/config"
	"gopanel/internal/platform/database"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	if len(os.Args) != 2 || (os.Args[1] != "up" && os.Args[1] != "down") {
		logger.Error("usage: migrate up|down")
		os.Exit(1)
	}
	if os.Args[1] == "down" && os.Getenv("ALLOW_MIGRATE_DOWN") != "1" {
		logger.Error("down destroys data: set ALLOW_MIGRATE_DOWN=1 explicitly")
		os.Exit(1)
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		logger.Error("invalid_configuration", "error", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err = database.Migrate(ctx, cfg, os.Args[1]); err != nil {
		logger.Error("migration_failed", "error", err)
		os.Exit(1)
	}
	logger.Info("migration_complete", "direction", os.Args[1])
}
