package httpserver

import (
	"github.com/go-chi/chi/v5"
	"gopanel/internal/platform/observability"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAbortedRequestReleasesTelemetryGauge(t *testing.T) {
	recorder := observability.New()
	router := chi.NewRouter()
	router.Use(observe(slog.New(slog.DiscardHandler), recorder))
	router.Get("/abort", func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) })
	func() {
		defer func() {
			if recover() != http.ErrAbortHandler {
				t.Fatal("abort signal changed")
			}
		}()
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/abort", nil))
	}()
	snapshot := recorder.Snapshot()
	if snapshot.Active != 0 || len(snapshot.Samples) != 1 || snapshot.Samples[0].Count != 1 || snapshot.Samples[0].Status != 500 {
		t.Fatal("aborted request leaked active gauge", snapshot)
	}
}
