package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"gopanel/internal/modules/members"
	repo "gopanel/internal/modules/members/repository"
	auth "gopanel/internal/platform/auth/handler"
	"gopanel/internal/platform/rbac"
	"gopanel/internal/platform/tenancy"
	"gopanel/internal/ui/pages"
)

type Handler struct {
	s *members.Service
	a *auth.Handler
}

func New(s *members.Service, a *auth.Handler) *Handler { return &Handler{s: s, a: a} }
func (h *Handler) Register(r chi.Router) {
	h.a.Protected(r, true, func(r chi.Router) {
		r.Get("/app/members", h.list)
		r.With(h.a.Mutation).Post("/app/members", h.add)
		r.With(h.a.Mutation).Post("/app/members/{id}", h.change)
	})
}
func (h *Handler) problem(w http.ResponseWriter, r *http.Request, err error, action string) {
	status, message := 503, "El servicio no está disponible."
	switch {
	case errors.Is(err, rbac.ErrForbidden):
		status, message = 403, "No tienes permiso para esta operación."
	case errors.Is(err, rbac.ErrNotFound):
		status, message = 404, "El recurso no está disponible."
	case errors.Is(err, rbac.ErrValidation):
		status, message = 422, "No se pudo guardar el miembro. Revisa el correo, rol y estado; la cuenta debe estar activa y disponible."
	case errors.Is(err, repo.ErrLastOwner):
		status, message = 409, "La empresa debe conservar al menos un Owner activo."
	}
	if status == 503 {
		h.a.Unavailable(w, r)
		return
	}
	actor, _ := tenancy.Actor(r.Context())
	if auditErr := h.s.Rejected(r.Context(), actor, action, w.Header().Get("X-Request-ID")); auditErr != nil {
		h.a.Unavailable(w, r)
		return
	}
	h.a.Problem(w, r, status, message)
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	actor, _ := tenancy.Actor(r.Context())
	offset := 0
	if value := r.URL.Query().Get("offset"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 || n > 100000 {
			offset = -1
		} else {
			offset = n
		}
	}
	rows, err := h.s.List(r.Context(), actor, int32(offset))
	if err != nil {
		h.problem(w, r, err, "memberships.read")
		return
	}
	h.a.View(w, r, 200, "Miembros", pages.Members(auth.CSRF(r), actor.TenantName, rbac.Can(actor.Role, rbac.MembersManage), rows, offset))
}
func (h *Handler) add(w http.ResponseWriter, r *http.Request) {
	actor, _ := tenancy.Actor(r.Context())
	err := h.s.Add(r.Context(), actor, r.PostForm.Get("email"), r.PostForm.Get("role"), w.Header().Get("X-Request-ID"))
	if err != nil {
		h.problem(w, r, err, "memberships.create")
		return
	}
	auth.Redirect(w, r, "/app/members")
}
func (h *Handler) change(w http.ResponseWriter, r *http.Request) {
	actor, _ := tenancy.Actor(r.Context())
	value := r.PostForm.Get("active")
	if value != "true" && value != "false" {
		h.problem(w, r, rbac.ErrValidation, "memberships.update")
		return
	}
	self, err := h.s.Change(r.Context(), actor, chi.URLParam(r, "id"), r.PostForm.Get("role"), value == "true", w.Header().Get("X-Request-ID"))
	if err != nil {
		h.problem(w, r, err, "memberships.update")
		return
	}
	if self {
		auth.Redirect(w, r, "/login")
		return
	}
	auth.Redirect(w, r, "/app/members")
}
