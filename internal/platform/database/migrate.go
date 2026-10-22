package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"gopanel/internal/config"
	"gopanel/migrations"
)

// Migrate owns its pool. Down is exactly one version; callers must explicitly authorize it.
func Migrate(ctx context.Context, cfg config.Config, direction string) error {
	if direction != "up" && direction != "down" {
		return fmt.Errorf("invalid migration direction")
	}
	db, err := OpenForMigrations(cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("migration connection: %w", err)
	}
	defer conn.Close()
	driver, err := migratemysql.WithConnection(ctx, conn, &migratemysql.Config{DatabaseName: cfg.DBName, StatementTimeout: 30 * time.Second})
	if err != nil {
		return fmt.Errorf("initialize migration driver: %w", err)
	}
	source, err := iofs.New(migrations.Files, ".")
	if err != nil {
		return fmt.Errorf("migration source: %w", err)
	}
	runner, err := migrate.NewWithInstance("iofs", source, "mysql", driver)
	if err != nil {
		_ = source.Close()
		_ = driver.Close()
		return fmt.Errorf("migration runner: %w", err)
	}
	defer runner.Close()
	runner.LockTimeout = 5 * time.Second
	if direction == "up" {
		err = runner.Up()
	} else {
		err = runner.Steps(-1)
	}
	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("migrate %s: %w", direction, err)
	}
	return nil
}
