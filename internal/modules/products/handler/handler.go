package handler

import (
	"errors"
	"net/http"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"gopanel/internal/modules/products"
	repo "gopanel/internal/modules/products/repository"
	auth "gopanel/internal/platform/auth/handler"
	"gopanel/internal/platform/rbac"
	"gopanel/internal/platform/tenancy"
	"gopanel/internal/ui/pages"
)

type Handler struct {
	s *products.Service
	a *auth.Handler
}

func New(s *products.Service, a *auth.Handler) *Handler { return &Handler{s: s, a: a} }
func (h *Handler) Register(r chi.Router) {
	h.a.Protected(r, true, func(r chi.Router) {
		r.Get("/app/products", h.list)
		r.Get("/app/products/table", h.list)
		r.Get("/app/products/new", h.new)
		r.With(h.a.MutationWithLimit(64<<10)).Post("/app/products/new", h.save)
		r.Get("/app/products/{id}", h.detail)
		r.Get("/app/products/{id}/edit", h.edit)
		r.With(h.a.MutationWithLimit(64<<10)).Post("/app/products/{id}/edit", h.save)
		r.Get("/app/products/{id}/delete", h.confirm)
		r.With(h.a.MutationWithLimit(64<<10)).Post("/app/products/{id}/delete", h.delete)
	})
}
func (h *Handler) view(w http.ResponseWriter, r *http.Request, status int, body templ.Component) {
	actor, _ := tenancy.Actor(r.Context())
	h.a.Render(w, r, status, pages.ProductPage(actor.TenantName, body), body)
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
	h.view(w, r, status, pages.ProductProblem(message))
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
	options, err := products.ParseOptions(r.URL.Query())
	if err != nil {
		// Authorize before reporting query validation errors.
		if _, _, accessErr := h.s.List(r.Context(), actor, repo.ListOptions{}); accessErr != nil {
			h.problem(w, r, accessErr, "products.read")
			return
		}
		h.problem(w, r, err, "products.read")
		return
	}
	rows, total, err := h.s.List(r.Context(), actor, options)
	if err != nil {
		h.problem(w, r, err, "products.read")
		return
	}
	if message == "" && r.URL.Query().Get("notice") == "deleted" {
		message = "Producto dado de baja."
	}
	model := pages.ProductListModel{Rows: rows, Options: options, Total: total, Tenant: actor.TenantName, Create: rbac.Can(actor.Role, rbac.ProductsCreate), Message: message}
	body := pages.ProductList(model)
	if chi.RouteContext(r.Context()).RoutePattern() == "/app/products/table" && r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Push-Url", products.ListURL(options, options.Page))
		body = pages.ProductResults(model)
	}
	h.view(w, r, status, body)
}
func (h *Handler) new(w http.ResponseWriter, r *http.Request) {
	actor, _ := tenancy.Actor(r.Context())
	if err := h.s.CanCreate(r.Context(), actor); err != nil {
		h.problem(w, r, err, "products.create")
		return
	}
	h.view(w, r, 200, pages.ProductForm(auth.CSRF(r), "", products.Input{Status: "active", Currency: "MXN", Price: "0.00"}, nil))
}
func (h *Handler) detail(w http.ResponseWriter, r *http.Request) {
	actor, _ := tenancy.Actor(r.Context())
	row, err := h.s.Get(r.Context(), actor, chi.URLParam(r, "id"), rbac.ProductsRead)
	if err != nil {
		h.problem(w, r, err, "products.read")
		return
	}
	message := ""
	switch r.URL.Query().Get("notice") {
	case "created":
		message = "Producto creado."
	case "updated":
		message = "Producto actualizado."
	}
	h.view(w, r, 200, pages.ProductDetail(row, rbac.Can(actor.Role, rbac.ProductsUpdate), rbac.Can(actor.Role, rbac.ProductsDelete), message))
}
func (h *Handler) edit(w http.ResponseWriter, r *http.Request) {
	actor, _ := tenancy.Actor(r.Context())
	row, err := h.s.Get(r.Context(), actor, chi.URLParam(r, "id"), rbac.ProductsUpdate)
	if err != nil {
		h.problem(w, r, err, "products.update")
		return
	}
	h.view(w, r, 200, pages.ProductForm(auth.CSRF(r), row.ID, products.Input{SKU: row.Sku, Name: row.Name, Description: row.Description.String, Price: products.FormatPrice(row.PriceCents), Currency: row.Currency, Status: row.Status}, nil))
}
func (h *Handler) save(w http.ResponseWriter, r *http.Request) {
	actor, _ := tenancy.Actor(r.Context())
	resource := chi.URLParam(r, "id")
	input := products.Input{SKU: r.PostForm.Get("sku"), Name: r.PostForm.Get("name"), Description: r.PostForm.Get("description"), Price: r.PostForm.Get("price"), Currency: r.PostForm.Get("currency"), Status: r.PostForm.Get("status")}
	action := "products.create"
	if resource != "" {
		action = "products.update"
	}
	value, normalized, err := h.s.Save(r.Context(), actor, resource, input, w.Header().Get("X-Request-ID"))
	var invalid *products.ValidationError
	if errors.As(err, &invalid) {
		if !h.reject(w, r, action) {
			return
		}
		h.view(w, r, 422, pages.ProductForm(auth.CSRF(r), resource, normalized, invalid.Fields))
		return
	}
	if err != nil {
		h.problem(w, r, err, action)
		return
	}
	notice, message, status := "updated", "Producto actualizado.", 200
	if resource == "" {
		notice, message, status = "created", "Producto creado.", 201
	}
	path := "/app/products/" + value
	if r.Header.Get("HX-Request") != "true" {
		auth.Redirect(w, r, path+"?notice="+notice)
		return
	}
	row, err := h.s.Get(r.Context(), actor, value, rbac.ProductsRead)
	if err != nil {
		h.problem(w, r, err, "products.read")
		return
	}
	w.Header().Set("HX-Push-Url", path)
	h.view(w, r, status, pages.ProductDetail(row, rbac.Can(actor.Role, rbac.ProductsUpdate), rbac.Can(actor.Role, rbac.ProductsDelete), message))
}
func (h *Handler) confirm(w http.ResponseWriter, r *http.Request) {
	actor, _ := tenancy.Actor(r.Context())
	row, err := h.s.Get(r.Context(), actor, chi.URLParam(r, "id"), rbac.ProductsDelete)
	if err != nil {
		h.problem(w, r, err, "products.delete")
		return
	}
	h.view(w, r, 200, pages.ProductDelete(auth.CSRF(r), row, ""))
}
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	actor, _ := tenancy.Actor(r.Context())
	resource := chi.URLParam(r, "id")
	err := h.s.Delete(r.Context(), actor, resource, r.PostForm.Get("confirm"), w.Header().Get("X-Request-ID"))
	var invalid *products.ValidationError
	if errors.As(err, &invalid) {
		if !h.reject(w, r, "products.delete") {
			return
		}
		row, e := h.s.Get(r.Context(), actor, resource, rbac.ProductsDelete)
		if e != nil {
			h.problem(w, r, e, "products.delete")
			return
		}
		h.view(w, r, 422, pages.ProductDelete(auth.CSRF(r), row, invalid.Fields["confirm"]))
		return
	}
	if err != nil {
		h.problem(w, r, err, "products.delete")
		return
	}
	if r.Header.Get("HX-Request") != "true" {
		auth.Redirect(w, r, "/app/products?notice=deleted")
		return
	}
	// POST bodies cannot inject list options; deletion returns the default current list.
	w.Header().Set("HX-Push-Url", "/app/products")
	r.URL.RawQuery = ""
	h.listResponse(w, r, 200, "Producto dado de baja.")
}
