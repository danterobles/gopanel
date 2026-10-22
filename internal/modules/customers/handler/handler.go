package handler

import (
	"errors"
	"net/http"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"gopanel/internal/modules/customers"
	repo "gopanel/internal/modules/customers/repository"
	auth "gopanel/internal/platform/auth/handler"
	"gopanel/internal/platform/rbac"
	"gopanel/internal/platform/tenancy"
	"gopanel/internal/ui/pages"
)

type Handler struct {
	s *customers.Service
	a *auth.Handler
}

func New(s *customers.Service, a *auth.Handler) *Handler { return &Handler{s: s, a: a} }
func (h *Handler) Register(r chi.Router) {
	h.a.Protected(r, true, func(r chi.Router) {
		r.Get("/app/customers", h.list)
		r.Get("/app/customers/table", h.list)
		r.Get("/app/customers/new", h.new)
		r.With(h.a.MutationWithLimit(64<<10)).Post("/app/customers/new", h.save)
		r.Get("/app/customers/{id}", h.detail)
		r.Get("/app/customers/{id}/edit", h.edit)
		r.With(h.a.MutationWithLimit(64<<10)).Post("/app/customers/{id}/edit", h.save)
		r.Get("/app/customers/{id}/delete", h.confirm)
		r.With(h.a.MutationWithLimit(64<<10)).Post("/app/customers/{id}/delete", h.delete)
	})
}
func (h *Handler) view(w http.ResponseWriter, r *http.Request, status int, body templ.Component) {
	actor, _ := tenancy.Actor(r.Context())
	h.a.Render(w, r, status, pages.CustomerPage(actor.TenantName, body), body)
}
func (h *Handler) problem(w http.ResponseWriter, r *http.Request, err error, action string) {
	status, message := 503, "El servicio no está disponible."
	switch {
	case errors.Is(err, rbac.ErrForbidden):
		status, message = 403, "No tienes permiso para esta operación."
	case errors.Is(err, repo.ErrNotFound):
		status, message = 404, "El recurso no está disponible."
	case errors.Is(err, repo.ErrInvalidOptions):
		status, message = 422, "Revisa filtros, orden, página y tamaño (10, 25 o 50)."
	}
	if status == 503 {
		h.a.Unavailable(w, r)
		return
	}
	if !h.reject(w, r, action) {
		return
	}
	h.view(w, r, status, pages.CustomerProblem(message))
}
func (h *Handler) reject(w http.ResponseWriter, r *http.Request, action string) bool {
	actor, _ := tenancy.Actor(r.Context())
	if err := h.s.Rejected(r.Context(), actor, action, w.Header().Get("X-Request-ID")); err != nil {
		h.a.Unavailable(w, r)
		return false
	}
	return true
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) { h.listResponse(w, r, 200, "") }
func (h *Handler) listResponse(w http.ResponseWriter, r *http.Request, status int, message string) {
	actor, _ := tenancy.Actor(r.Context())
	options, err := customers.ParseOptions(r.URL.Query())
	if err != nil {
		// Authorize before reporting query validation errors.
		if _, _, accessErr := h.s.List(r.Context(), actor, repo.ListOptions{}); accessErr != nil {
			h.problem(w, r, accessErr, "customers.read")
			return
		}
		h.problem(w, r, err, "customers.read")
		return
	}
	rows, total, err := h.s.List(r.Context(), actor, options)
	if err != nil {
		h.problem(w, r, err, "customers.read")
		return
	}
	if message == "" && r.URL.Query().Get("notice") == "deleted" {
		message = "Cliente dado de baja."
	}
	model := pages.CustomerListModel{Rows: rows, Options: options, Total: total, Tenant: actor.TenantName, Create: rbac.Can(actor.Role, rbac.CustomersCreate), Message: message}
	body := pages.CustomerList(model)
	if chi.RouteContext(r.Context()).RoutePattern() == "/app/customers/table" && r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Push-Url", customers.ListURL(options, options.Page))
		body = pages.CustomerResults(model)
	}
	h.view(w, r, status, body)
}
func (h *Handler) new(w http.ResponseWriter, r *http.Request) {
	actor, _ := tenancy.Actor(r.Context())
	if err := h.s.CanCreate(r.Context(), actor); err != nil {
		h.problem(w, r, err, "customers.create")
		return
	}
	h.view(w, r, 200, pages.CustomerForm(auth.CSRF(r), "", customers.Input{Status: "active"}, nil))
}
func (h *Handler) detail(w http.ResponseWriter, r *http.Request) {
	actor, _ := tenancy.Actor(r.Context())
	row, err := h.s.Get(r.Context(), actor, chi.URLParam(r, "id"), rbac.CustomersRead)
	if err != nil {
		h.problem(w, r, err, "customers.read")
		return
	}
	message := ""
	switch r.URL.Query().Get("notice") {
	case "created":
		message = "Cliente creado."
	case "updated":
		message = "Cliente actualizado."
	}
	h.view(w, r, 200, pages.CustomerDetail(row, rbac.Can(actor.Role, rbac.CustomersUpdate), rbac.Can(actor.Role, rbac.CustomersDelete), message))
}
func (h *Handler) edit(w http.ResponseWriter, r *http.Request) {
	actor, _ := tenancy.Actor(r.Context())
	row, err := h.s.Get(r.Context(), actor, chi.URLParam(r, "id"), rbac.CustomersUpdate)
	if err != nil {
		h.problem(w, r, err, "customers.update")
		return
	}
	h.view(w, r, 200, pages.CustomerForm(auth.CSRF(r), row.ID, customers.Input{Name: row.Name, Email: row.Email.String, Phone: row.Phone.String, Status: row.Status, Notes: row.Notes.String}, nil))
}
func (h *Handler) save(w http.ResponseWriter, r *http.Request) {
	actor, _ := tenancy.Actor(r.Context())
	resource := chi.URLParam(r, "id")
	input := customers.Input{Name: r.PostForm.Get("name"), Email: r.PostForm.Get("email"), Phone: r.PostForm.Get("phone"), Status: r.PostForm.Get("status"), Notes: r.PostForm.Get("notes")}
	action := "customers.create"
	if resource != "" {
		action = "customers.update"
	}
	value, normalized, err := h.s.Save(r.Context(), actor, resource, input, w.Header().Get("X-Request-ID"))
	var invalid *customers.ValidationError
	if errors.As(err, &invalid) {
		if !h.reject(w, r, action) {
			return
		}
		h.view(w, r, 422, pages.CustomerForm(auth.CSRF(r), resource, normalized, invalid.Fields))
		return
	}
	if err != nil {
		h.problem(w, r, err, action)
		return
	}
	notice, message, status := "updated", "Cliente actualizado.", 200
	if resource == "" {
		notice, message, status = "created", "Cliente creado.", 201
	}
	path := "/app/customers/" + value
	if r.Header.Get("HX-Request") != "true" {
		auth.Redirect(w, r, path+"?notice="+notice)
		return
	}
	row, err := h.s.Get(r.Context(), actor, value, rbac.CustomersRead)
	if err != nil {
		h.problem(w, r, err, "customers.read")
		return
	}
	w.Header().Set("HX-Push-Url", path)
	h.view(w, r, status, pages.CustomerDetail(row, rbac.Can(actor.Role, rbac.CustomersUpdate), rbac.Can(actor.Role, rbac.CustomersDelete), message))
}
func (h *Handler) confirm(w http.ResponseWriter, r *http.Request) {
	actor, _ := tenancy.Actor(r.Context())
	row, err := h.s.Get(r.Context(), actor, chi.URLParam(r, "id"), rbac.CustomersDelete)
	if err != nil {
		h.problem(w, r, err, "customers.delete")
		return
	}
	h.view(w, r, 200, pages.CustomerDelete(auth.CSRF(r), row, ""))
}
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	actor, _ := tenancy.Actor(r.Context())
	resource := chi.URLParam(r, "id")
	err := h.s.Delete(r.Context(), actor, resource, r.PostForm.Get("confirm"), w.Header().Get("X-Request-ID"))
	var invalid *customers.ValidationError
	if errors.As(err, &invalid) {
		if !h.reject(w, r, "customers.delete") {
			return
		}
		row, e := h.s.Get(r.Context(), actor, resource, rbac.CustomersDelete)
		if e != nil {
			h.problem(w, r, e, "customers.delete")
			return
		}
		h.view(w, r, 422, pages.CustomerDelete(auth.CSRF(r), row, invalid.Fields["confirm"]))
		return
	}
	if err != nil {
		h.problem(w, r, err, "customers.delete")
		return
	}
	if r.Header.Get("HX-Request") != "true" {
		auth.Redirect(w, r, "/app/customers?notice=deleted")
		return
	}
	// POST bodies cannot inject list options; deletion returns the default current list.
	w.Header().Set("HX-Push-Url", "/app/customers")
	r.URL.RawQuery = ""
	h.listResponse(w, r, 200, "Cliente dado de baja.")
}
