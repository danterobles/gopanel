package httpserver

import (
	"context"
	"crypto/rand"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"gopanel/internal/platform/observability"
	"gopanel/internal/ui/pages"
	"gopanel/web"
)

type Pinger interface{ PingContext(context.Context) error }

type requestIDKey struct{}

func NewRouter(db Pinger, logger *slog.Logger, healthTimeout time.Duration, recorders ...*observability.Recorder) *chi.Mux {
	recorder := observability.New()
	if len(recorders) > 0 {
		recorder = recorders[0]
	}
	r := chi.NewRouter()
	r.Use(observe(logger, recorder))
	r.Use(recoverPanic(logger))
	r.Get("/health/live", func(w http.ResponseWriter, r *http.Request) { health(w, http.StatusOK, "ok") })
	r.Get("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), healthTimeout)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			health(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
		health(w, http.StatusOK, "ok")
	})
	r.Get("/", templ.Handler(pages.Home()).ServeHTTP)
	assets, err := fs.Sub(web.Assets, "static")
	if err != nil {
		panic("embedded assets missing")
	}
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(assets))))
	return r
}

func health(w http.ResponseWriter, status int, state string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write([]byte("{\"status\":\"" + state + "\"}\n"))
}

func observe(logger *slog.Logger, recorder *observability.Recorder) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := rand.Text()
			w.Header().Set("X-Request-ID", id)
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", "same-origin")
			ctx := context.WithValue(r.Context(), requestIDKey{}, id)
			wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			recorder.Start()
			completed := false
			defer func() {
				route := chi.RouteContext(ctx).RoutePattern()
				if route == "" {
					route = "unmatched"
				}
				status := wrapped.Status()
				if status == 0 {
					status = http.StatusOK
					if !completed {
						status = http.StatusInternalServerError
					}
				}
				recorder.Finish(r.Method, route, status, time.Since(start))
				logger.InfoContext(ctx, "http_request", "request_id", id, "method", r.Method, "route", route, "status", status, "duration_ms", float64(time.Since(start).Microseconds())/1000)
			}()
			next.ServeHTTP(wrapped, r.WithContext(ctx))
			completed = true
		})
	}
}

func recoverPanic(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if value := recover(); value != nil {
					if value == http.ErrAbortHandler {
						panic(value)
					}
					logger.ErrorContext(r.Context(), "request_panic", "request_id", r.Context().Value(requestIDKey{}))
					http.Error(w, "internal server error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
