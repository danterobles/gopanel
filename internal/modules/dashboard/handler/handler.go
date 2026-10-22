package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"gopanel/internal/modules/dashboard"
	authhttp "gopanel/internal/platform/auth/handler"
	"gopanel/internal/platform/rbac"
	"gopanel/internal/platform/tenancy"
	"gopanel/internal/ui/pages"
)

type Handler struct {
	service *dashboard.Service
	auth    *authhttp.Handler
}

func New(s *dashboard.Service, a *authhttp.Handler) *Handler { return &Handler{service: s, auth: a} }
func (h *Handler) Register(r chi.Router) {
	h.auth.Protected(r, true, func(r chi.Router) { r.Get("/app", h.page) })
	h.auth.PassiveProtected(r, func(r chi.Router) { r.Get("/app/dashboard/widgets", h.widgets) })
}
func (h *Handler) page(w http.ResponseWriter, r *http.Request)    { h.render(w, r, false) }
func (h *Handler) widgets(w http.ResponseWriter, r *http.Request) { h.render(w, r, true) }
func (h *Handler) render(w http.ResponseWriter, r *http.Request, widgets bool) {
	actor, _ := tenancy.Actor(r.Context())
	if scope := r.Header.Get("X-Dashboard-Scope"); widgets && scope != "" && scope != actor.TenantID+":"+actor.Role {
		authhttp.Redirect(w, r, "/app")
		return
	}
	snapshot, err := h.service.Read(r.Context(), actor)
	if errors.Is(err, rbac.ErrForbidden) {
		h.auth.Problem(w, r, 403, "No tienes permiso para consultar las métricas.")
		return
	}
	status := 200
	if err != nil {
		status = 503
		slog.ErrorContext(r.Context(), "dashboard_query_failed", "request_id", w.Header().Get("X-Request-ID"))
	}
	full := pages.DashboardPage(actor, authhttp.CSRF(r), snapshot, err != nil)
	fragment := pages.Dashboard(actor, authhttp.CSRF(r), snapshot, err != nil)
	if widgets {
		fragment = pages.DashboardWidgets(snapshot, err != nil)
	}
	h.auth.Render(w, r, status, full, fragment)
}
