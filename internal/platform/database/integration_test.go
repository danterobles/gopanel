//go:build integration

package database

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"gopanel/internal/config"
)

func TestMySQLConnectionSettings(t *testing.T) {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	db, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal("MySQL unavailable for integration test")
	}
	// Two simultaneous connections prove session settings are not applied only once to the pool.
	first, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	for _, conn := range []*sql.Conn{first, second} {
		var zone, mode, charset, version string
		if err := conn.QueryRowContext(ctx, "SELECT @@session.time_zone, @@session.sql_mode, @@character_set_connection, VERSION()").Scan(&zone, &mode, &charset, &version); err != nil {
			t.Fatal(err)
		}
		if zone != "+00:00" || !strings.Contains(mode, "STRICT_TRANS_TABLES") || charset != "utf8mb4" || !strings.HasPrefix(version, "8.") {
			t.Fatalf("invalid DB session settings: %s %s %s %s", zone, mode, charset, version)
		}
		var timestamp time.Time
		if err := conn.QueryRowContext(ctx, "SELECT CAST('2026-10-21 12:30:00.123456' AS DATETIME(6))").Scan(&timestamp); err != nil {
			t.Fatal(err)
		}
		if timestamp.Location() != time.UTC || timestamp.Nanosecond() != 123456000 {
			t.Fatal("DATETIME(6) UTC parsing failed")
		}
	}
}
