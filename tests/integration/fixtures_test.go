//go:build integration

package integration_test

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"gopanel/internal/config"
	dbgen "gopanel/internal/db/generated"
	"gopanel/internal/platform/database"
	"gopanel/internal/platform/id"
)

func scratch(t *testing.T, apply bool) (config.Config, *sql.DB) {
	t.Helper()
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	if v := os.Getenv("TEST_DB_USER"); v != "" {
		cfg.DBUser = v
		cfg.DBPassword = os.Getenv("TEST_DB_PASSWORD")
	}
	admin, err := database.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	name := "gopanel_test_" + strings.ReplaceAll(id.New(), "-", "")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err = admin.ExecContext(ctx, "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci"); err != nil {
		admin.Close()
		t.Fatalf("create disposable database (requires test credentials with CREATE DATABASE): %v", err)
	}
	cfg.DBName = name
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.ExecContext(cleanup, "DROP DATABASE `"+name+"`"); err != nil {
			t.Errorf("cleanup disposable database: %v", err)
		}
		_ = admin.Close()
	})
	if apply {
		if err = database.Migrate(t.Context(), cfg, "up"); err != nil {
			t.Fatal(err)
		}
	}
	return cfg, db
}

func tenantOwner(t *testing.T, db *sql.DB, slug string) (string, string, string) {
	t.Helper()
	q := dbgen.New(db)
	ctx := t.Context()
	user, tenant, member := id.New(), id.New(), id.New()
	if err := q.CreateUser(ctx, dbgen.CreateUserParams{ID: user, Email: slug + "@fixture.test", PasswordHash: "$argon2id$fixture-not-for-login", DisplayName: slug}); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	q = q.WithTx(tx)
	if err = q.CreateTenant(ctx, dbgen.CreateTenantParams{ID: tenant, Name: slug, Slug: slug}); err != nil {
		t.Fatal(err)
	}
	if err = q.CreateMembership(ctx, dbgen.CreateMembershipParams{ID: member, TenantID: tenant, UserID: user, Role: "owner", IsActive: true}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return tenant, user, member
}
