package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"gopanel/internal/platform/auth"
	authhttp "gopanel/internal/platform/auth/handler"
	"gopanel/internal/platform/events"
	"gopanel/internal/platform/tenancy"
)

type Handler struct {
	broker   *events.Broker
	auth     *authhttp.Handler
	shutdown context.Context
}

func New(b *events.Broker, a *authhttp.Handler, shutdown context.Context) *Handler {
	return &Handler{broker: b, auth: a, shutdown: shutdown}
}
func (h *Handler) Register(r chi.Router) { r.With(h.auth.Stream).Get("/app/events", h.stream) }
func (h *Handler) stream(w http.ResponseWriter, r *http.Request) {
	actor, _ := tenancy.Actor(r.Context())
	sub, err := h.broker.Subscribe(actor.TenantID, actor.UserID)
	if err != nil {
		w.Header().Set("Retry-After", "3")
		http.Error(w, "unavailable", 503)
		return
	}
	defer sub.Cancel()
	controller := http.NewResponseController(w)
	write := func(frame string) error {
		if err := controller.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
			return err
		}
		if _, err := fmt.Fprint(w, frame); err != nil {
			return err
		}
		return controller.Flush()
	}
	authorized := func() error {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		return h.auth.StreamAuthorized(r.WithContext(ctx))
	}
	if err = authorized(); err != nil {
		if errors.Is(err, auth.ErrUnauthorized) {
			http.Error(w, "unauthorized", 401)
		} else {
			http.Error(w, "unavailable", 503)
		}
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	if err = write("retry: 3000\nevent: ready\ndata: {}\n\n"); err != nil {
		return
	}
	check := time.NewTicker(2 * time.Second)
	defer check.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	validate := func() bool {
		if err := authorized(); err != nil {
			if errors.Is(err, auth.ErrUnauthorized) {
				_ = write("event: closed\ndata: {}\n\n")
			}
			return false
		}
		return true
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case <-h.shutdown.Done():
			return
		case <-sub.Done:
			return
		case <-check.C:
			if !validate() {
				return
			}
		case <-heartbeat.C:
			if !validate() {
				return
			}
			if write(": heartbeat\n\n") != nil {
				return
			}
		case event := <-sub.Events:
			select {
			case <-sub.Done:
				return
			default:
			}
			if !validate() {
				return
			}
			data, _ := json.Marshal(event)
			if write("event: invalidate\ndata: "+string(data)+"\n\n") != nil {
				return
			}
		}
	}
}
