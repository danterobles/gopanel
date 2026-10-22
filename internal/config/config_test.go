package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	cases := []struct {
		name      string
		env       map[string]string
		wantError string
	}{
		{"defaults", nil, ""},
		{"production TLS", map[string]string{"APP_ENV": "production", "DB_TLS": "true"}, ""},
		{"missing password", map[string]string{"DB_PASSWORD": ""}, "DB_PASSWORD"},
		{"environment", map[string]string{"APP_ENV": "preview"}, "APP_ENV"},
		{"address", map[string]string{"HTTP_ADDR": "localhost"}, "HTTP_ADDR"},
		{"zero HTTP port", map[string]string{"HTTP_ADDR": ":0"}, "HTTP_ADDR"},
		{"DB port", map[string]string{"DB_PORT": "70000"}, "DB_PORT"},
		{"TLS", map[string]string{"DB_TLS": "skip-verify"}, "DB_TLS"},
		{"production without TLS", map[string]string{"APP_ENV": "production"}, "DB_TLS"},
		{"negative deadline", map[string]string{"HEALTH_TIMEOUT": "-1s"}, "HEALTH_TIMEOUT"},
		{"bad deadline", map[string]string{"SHUTDOWN_TIMEOUT": "private-invalid-value"}, "SHUTDOWN_TIMEOUT"},
		{"excess deadline", map[string]string{"SHUTDOWN_TIMEOUT": "2m"}, "SHUTDOWN_TIMEOUT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := map[string]string{"DB_PASSWORD": "private-password"}
			for k, v := range tc.env {
				env[k] = v
			}
			cfg, err := Load(func(k string) string { return env[k] })
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("expected error for %s, got %v", tc.wantError, err)
				}
				if strings.Contains(err.Error(), "private-") {
					t.Fatal("configuration error leaks input")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.HealthTimeout != 2*time.Second || cfg.ShutdownTimeout != 10*time.Second {
				t.Fatal("unexpected defaults")
			}
		})
	}
}

func TestCustomTimeouts(t *testing.T) {
	env := map[string]string{"DB_PASSWORD": "test", "HEALTH_TIMEOUT": "500ms", "SHUTDOWN_TIMEOUT": "30s"}
	cfg, err := Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HealthTimeout != 500*time.Millisecond || cfg.ShutdownTimeout != 30*time.Second {
		t.Fatal("timeouts not applied")
	}
}
