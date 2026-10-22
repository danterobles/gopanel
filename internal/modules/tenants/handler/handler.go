package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"gopanel/internal/modules/tenants"
	auth "gopanel/internal/platform/auth/handler"
	"gopanel/internal/platform/rbac"
	"gopanel/internal/platform/tenancy"
	"gopanel/internal/ui/pages"
)

type Handler struct {
	s *tenants.Service
	a *auth.Handler
}

func New(s *tenants.Service, a *auth.Handler) *Handler { return &Handler{s: s, a: a} }
func (h *Handler) Register(r chi.Router) {
	h.a.Protected(r, true, func(r chi.Router) {
		r.Get("/app/settings", h.settings)
		r.With(h.a.Mutation).Post("/app/settings", h.rename)
	})
	h.a.Protected(r, false, func(r chi.Router) {
		r.Get("/platform", h.platform)
		r.With(h.a.Mutation).Post("/platform/tenants", h.create)
		r.With(h.a.Mutation).Post("/platform/tenants/{id}", h.update)
	})
}
func (h *Handler) problem(w http.ResponseWriter, r *http.Request, err error, action string, global bool) {
	status, message := 503, "El servicio no está disponible."
	switch {
	case errors.Is(err, rbac.ErrForbidden):
		status, message = 403, "No tienes permiso para esta operación."
	case errors.Is(err, rbac.ErrNotFound):
		status, message = 404, "El recurso no está disponible."
	case errors.Is(err, rbac.ErrValidation):
		status, message = 422, "No se pudo guardar la empresa. Revisa nombre (1–180), slug (3–100, minúsculas, números y guiones) y correo de una cuenta activa disponible."
	}
	if status == 503 {
		h.a.Unavailable(w, r)
		return
	}
	tenant := ""
	if !global {
		actor, _ := tenancy.Actor(r.Context())
		tenant = actor.TenantID
	}
	if e := h.s.Rejected(r.Context(), auth.UserID(r), tenant, action, w.Header().Get("X-Request-ID")); e != nil {
		h.a.Unavailable(w, r)
		return
	}
	h.a.Problem(w, r, status, message)
}
func (h *Handler) settings(w http.ResponseWriter, r *http.Request) {
	actor, _ := tenancy.Actor(r.Context())
	row, err := h.s.Settings(r.Context(), actor)
	if err != nil {
		h.problem(w, r, err, "tenant.settings", false)
		return
	}
	h.a.View(w, r, 200, "Configuración de empresa", pages.Settings(auth.CSRF(r), row))
}
func (h *Handler) rename(w http.ResponseWriter, r *http.Request) {
	actor, _ := tenancy.Actor(r.Context())
	err := h.s.Rename(r.Context(), actor, r.PostForm.Get("name"), w.Header().Get("X-Request-ID"))
	if err != nil {
		h.problem(w, r, err, "tenant.settings", false)
		return
	}
	auth.Redirect(w, r, "/app/settings")
}
func (h *Handler) platform(w http.ResponseWriter, r *http.Request) {
	offset := 0
	if value := r.URL.Query().Get("offset"); value != "" {
		n, e := strconv.Atoi(value)
		if e != nil || n < 0 || n > 100000 {
			offset = -1
		} else {
			offset = n
		}
	}
	rows, stats, err := h.s.Platform(r.Context(), auth.UserID(r), int32(offset))
	if err != nil {
		h.problem(w, r, err, "platform.read", true)
		return
	}
	h.a.View(w, r, 200, "Administración de plataforma", pages.Platform(auth.CSRF(r), rows, stats, offset))
}
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	err := h.s.Create(r.Context(), auth.UserID(r), r.PostForm.Get("name"), r.PostForm.Get("slug"), r.PostForm.Get("owner_email"), w.Header().Get("X-Request-ID"))
	if err != nil {
		h.problem(w, r, err, "platform.tenants.create", true)
		return
	}
	auth.Redirect(w, r, "/platform")
}
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	value := r.PostForm.Get("active")
	if value != "true" && value != "false" {
		h.problem(w, r, rbac.ErrValidation, "platform.tenants.update", true)
		return
	}
	err := h.s.Update(r.Context(), auth.UserID(r), chi.URLParam(r, "id"), r.PostForm.Get("name"), value == "true", w.Header().Get("X-Request-ID"))
	if err != nil {
		h.problem(w, r, err, "platform.tenants.update", true)
		return
	}
	auth.Redirect(w, r, "/platform")
}
