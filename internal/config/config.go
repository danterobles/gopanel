package config

import (
	"fmt"
	"net"
	"strconv"
	"time"
)

type Config struct {
	Env, HTTPAddr                              string
	DBHost, DBPort, DBName, DBUser, DBPassword string
	DBTLS                                      bool
	HealthTimeout, ShutdownTimeout             time.Duration
}

// Load accepts an environment lookup so validation can be tested without process globals.
func Load(get func(string) string) (Config, error) {
	value := func(key, fallback string) string {
		if v := get(key); v != "" {
			return v
		}
		return fallback
	}
	c := Config{
		Env: value("APP_ENV", "development"), HTTPAddr: value("HTTP_ADDR", ":8080"),
		DBHost: value("DB_HOST", "127.0.0.1"), DBPort: value("DB_PORT", "3306"),
		DBName: value("DB_NAME", "gopanel"), DBUser: value("DB_USER", "gopanel"), DBPassword: get("DB_PASSWORD"),
		HealthTimeout: 2 * time.Second, ShutdownTimeout: 10 * time.Second,
	}
	if c.Env != "development" && c.Env != "test" && c.Env != "production" {
		return Config{}, fmt.Errorf("APP_ENV must be development, test or production")
	}
	if c.DBPassword == "" {
		return Config{}, fmt.Errorf("DB_PASSWORD is required")
	}
	_, port, err := net.SplitHostPort(c.HTTPAddr)
	if err != nil || !validPort(port) {
		return Config{}, fmt.Errorf("HTTP_ADDR must contain a valid host and port")
	}
	if !validPort(c.DBPort) {
		return Config{}, fmt.Errorf("DB_PORT must be a valid port")
	}
	tlsMode := value("DB_TLS", "false")
	if tlsMode != "true" && tlsMode != "false" {
		return Config{}, fmt.Errorf("DB_TLS must be true or false")
	}
	c.DBTLS = tlsMode == "true"
	if c.Env == "production" && !c.DBTLS {
		return Config{}, fmt.Errorf("production requires DB_TLS=true")
	}
	for _, item := range []struct {
		key  string
		dest *time.Duration
	}{
		{"HEALTH_TIMEOUT", &c.HealthTimeout}, {"SHUTDOWN_TIMEOUT", &c.ShutdownTimeout},
	} {
		if raw := get(item.key); raw != "" {
			d, err := time.ParseDuration(raw)
			if err != nil || d <= 0 || d > time.Minute {
				return Config{}, fmt.Errorf("%s must be a duration greater than zero and at most 1m", item.key)
			}
			*item.dest = d
		}
	}
	return c, nil
}

func validPort(raw string) bool {
	n, err := strconv.Atoi(raw)
	return err == nil && n > 0 && n <= 65535
}
