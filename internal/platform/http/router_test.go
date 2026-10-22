package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type pingFunc func(context.Context) error

func (f pingFunc) PingContext(ctx context.Context) error { return f(ctx) }

func TestHealth(t *testing.T) {
	for _, tc := range []struct {
		name  string
		ping  pingFunc
		ready int
	}{
		{"healthy", func(context.Context) error { return nil }, 200},
		{"database failure", func(context.Context) error { return errors.New("private-dsn-password") }, 503},
		{"deadline", func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			r := NewRouter(tc.ping, slog.New(slog.NewJSONHandler(&logs, nil)), 5*time.Millisecond)
			for _, route := range []struct {
				path   string
				status int
			}{{"/health/live", 200}, {"/health/ready", tc.ready}} {
				response := httptest.NewRecorder()
				request := httptest.NewRequest("GET", route.path+"?token=private-query", nil)
				request.Header.Set("X-Request-ID", "private-client-id")
				r.ServeHTTP(response, request)
				if response.Code != route.status {
					t.Fatalf("%s: %d", route.path, response.Code)
				}
				if response.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("health must not be cached")
				}
				id := response.Header().Get("X-Request-ID")
				if id == "" || id == "private-client-id" {
					t.Fatal("expected server request ID")
				}
				var body map[string]string
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(response.Body.String(), "private-") {
					t.Fatal("health leaked secret")
				}
				if !strings.Contains(logs.String(), id) {
					t.Fatal("log not correlated")
				}
			}
			if strings.Contains(logs.String(), "private-") {
				t.Fatal("logs leaked request or DSN")
			}
			for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
				var entry map[string]any
				if err := json.Unmarshal([]byte(line), &entry); err != nil {
					t.Fatal(err)
				}
				if entry["route"] != "/health/live" && entry["route"] != "/health/ready" {
					t.Fatalf("unexpected route: %v", entry)
				}
			}
			response := httptest.NewRecorder()
			r.ServeHTTP(response, httptest.NewRequest("POST", "/health/ready", nil))
			if response.Code != http.StatusMethodNotAllowed {
				t.Fatalf("POST health: %d", response.Code)
			}
		})
	}
}

func TestPageAndAssets(t *testing.T) {
	r := NewRouter(pingFunc(func(context.Context) error { return nil }), slog.New(slog.DiscardHandler), time.Second)
	for _, path := range []string{"/", "/static/app.css", "/static/htmx.min.js", "/static/sse.js"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || w.Body.Len() == 0 {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
}

func TestPanicDoesNotLeakAndServerContinues(t *testing.T) {
	var logs bytes.Buffer
	r := NewRouter(pingFunc(func(context.Context) error { panic("private-panic") }), slog.New(slog.NewJSONHandler(&logs, nil)), time.Second)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/health/ready", nil))
	if w.Code != 500 {
		t.Fatalf("panic status: %d", w.Code)
	}
	if strings.Contains(logs.String()+w.Body.String(), "private-panic") {
		t.Fatal("panic value leaked")
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/health/live", nil))
	if w.Code != 200 {
		t.Fatal("server did not recover")
	}
}
