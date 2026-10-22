//go:build integration

package integration_test

import (
	"errors"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"gopanel/internal/platform/database"
)

func TestMigrationUpDownUpAndDirtyProtection(t *testing.T) {
	cfg, db := scratch(t, false)
	ctx := t.Context()
	if err := database.Migrate(ctx, cfg, "up"); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, cfg, "up"); err != nil {
		t.Fatal("idempotent up:", err)
	}
	var tables int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=? AND engine='InnoDB'", cfg.DBName).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 9 {
		t.Fatalf("expected 8 application tables plus migration version, got %d", tables)
	}
	tenantOwner(t, db, "rollback-fixture")
	for range 2 {
		if err := database.Migrate(ctx, cfg, "down"); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.Migrate(ctx, cfg, "up"); err != nil {
		t.Fatal(err)
	}
	var tenants int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tenants").Scan(&tenants); err != nil {
		t.Fatal(err)
	}
	if tenants != 0 {
		t.Fatal("rollback/reapply retained fixture")
	}
	if _, err := db.ExecContext(ctx, "UPDATE schema_migrations SET dirty=TRUE"); err != nil {
		t.Fatal(err)
	}
	var dirty migrate.ErrDirty
	if err := database.Migrate(ctx, cfg, "up"); !errors.As(err, &dirty) {
		t.Fatalf("dirty migration not rejected: %v", err)
	}
}
