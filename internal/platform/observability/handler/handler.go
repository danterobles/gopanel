package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	authhttp "gopanel/internal/platform/auth/handler"
	"gopanel/internal/platform/events"
	"gopanel/internal/platform/observability"
	"gopanel/internal/platform/rbac"
	"net/http"
	"runtime"
	"time"
)

type Handler struct {
	db       *sql.DB
	recorder *observability.Recorder
	broker   *events.Broker
	auth     *authhttp.Handler
	started  time.Time
}

func New(db *sql.DB, r *observability.Recorder, b *events.Broker, a *authhttp.Handler) *Handler {
	return &Handler{db: db, recorder: r, broker: b, auth: a, started: time.Now()}
}
func (h *Handler) Register(r chi.Router) {
	h.auth.Protected(r, false, func(r chi.Router) { r.Get("/platform/metrics", h.metrics) })
}
func (h *Handler) metrics(w http.ResponseWriter, r *http.Request) {
	err := rbac.RequirePlatform(r.Context(), h.db, authhttp.UserID(r))
	if errors.Is(err, rbac.ErrForbidden) {
		h.auth.Problem(w, r, 403, "No tienes permiso para consultar métricas operativas.")
		return
	}
	if err != nil {
		h.auth.Unavailable(w, r)
		return
	}
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	data := struct {
		UptimeSeconds float64                `json:"uptime_seconds"`
		HTTP          observability.Snapshot `json:"http"`
		SQL           sql.DBStats            `json:"sql_pool"`
		SSE           events.Stats           `json:"sse"`
		HeapAlloc     uint64                 `json:"heap_alloc_bytes"`
		Sys           uint64                 `json:"go_sys_bytes"`
		Goroutines    int                    `json:"goroutines"`
	}{time.Since(h.started).Seconds(), h.recorder.Snapshot(), h.db.Stats(), h.broker.Stats(), memory.HeapAlloc, memory.Sys, runtime.NumGoroutine()}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(data)
}
