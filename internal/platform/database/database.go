package database

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/go-sql-driver/mysql"
	"gopanel/internal/config"
)

// Open configures a lazy pool; readiness verifies connectivity with PingContext.
func Open(c config.Config) (*sql.DB, error) {
	return open(c, false)
}

// OpenForMigrations is separate so application queries never allow multi-statements.
func OpenForMigrations(c config.Config) (*sql.DB, error) {
	return open(c, true)
}

func open(c config.Config, migration bool) (*sql.DB, error) {
	mc := mysql.NewConfig()
	mc.User, mc.Passwd = c.DBUser, c.DBPassword
	mc.Net, mc.Addr, mc.DBName = "tcp", net.JoinHostPort(c.DBHost, c.DBPort), c.DBName
	mc.ParseTime, mc.Loc = true, time.UTC
	mc.ClientFoundRows = true
	mc.MultiStatements = migration
	mc.Logger = driverLogger{}
	if err := mc.Apply(mysql.Charset("utf8mb4", "utf8mb4_0900_ai_ci")); err != nil {
		return nil, fmt.Errorf("configure MySQL character set")
	}
	mc.Timeout, mc.ReadTimeout, mc.WriteTimeout = 3*time.Second, 3*time.Second, 3*time.Second
	if migration {
		mc.ReadTimeout, mc.WriteTimeout = 30*time.Second, 30*time.Second
	}
	if c.DBTLS {
		mc.TLSConfig = "true"
	}
	mc.Params = map[string]string{
		"time_zone": "'+00:00'",
		"sql_mode":  "'STRICT_TRANS_TABLES,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION'",
	}
	connector, err := mysql.NewConnector(mc)
	if err != nil {
		return nil, fmt.Errorf("configure MySQL connector: invalid configuration")
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(3 * time.Minute)
	db.SetConnMaxIdleTime(time.Minute)
	return db, nil
}

// Driver diagnostics may contain server-provided text. Emit a structured generic
// warning; readiness/request status supplies the observable failure result.
type driverLogger struct{}

func (driverLogger) Print(...any) { slog.Warn("mysql_driver_warning") }
