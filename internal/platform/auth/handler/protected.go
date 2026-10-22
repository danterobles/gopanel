package handler

import (
	"net/http"
	"strings"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"gopanel/internal/ui/pages"
)

// Protected shares exactly the session, cache and origin policy used by authentication.
// Domain services still authorize every action and resource.
func (h *Handler) Protected(r chi.Router, tenant bool, routes func(chi.Router)) {
	r.Group(func(r chi.Router) {
		r.Use(noStore, h.Session)
		if tenant {
			r.Use(h.Tenant)
		}
		routes(r)
	})
}
func (h *Handler) Mutation(next http.Handler) http.Handler {
	return h.origin.Handler(h.parse(h.sessionCSRF(next)))
}

// MutationWithLimit uses the same origin/CSRF policy with a module-specific body bound.
func (h *Handler) MutationWithLimit(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return h.origin.Handler(h.parseLimit(h.sessionCSRF(next), limit))
	}
}
func CSRF(r *http.Request) string                                  { return principal(r).CSRF() }
func UserID(r *http.Request) string                                { return principal(r).UserID() }
func Redirect(w http.ResponseWriter, r *http.Request, path string) { redirect(w, r, path) }
func (h *Handler) View(w http.ResponseWriter, r *http.Request, status int, title string, body templ.Component) {
	h.Render(w, r, status, pages.ProtectedPage(title, body), body)
}
func (h *Handler) Problem(w http.ResponseWriter, r *http.Request, status int, message string) {
	h.Render(w, r, status, pages.ProtectedPage("No se pudo completar la solicitud", pages.OperationNotice(message)), pages.OperationNotice(message))
}
func (h *Handler) Unavailable(w http.ResponseWriter, r *http.Request) { h.failure(w, r) }
func mutationAction(path string) string {
	switch {
	case strings.HasPrefix(path, "/app/customers"):
		return "customers.csrf"
	case strings.HasPrefix(path, "/app/members"):
		return "memberships.csrf"
	case path == "/app/settings":
		return "tenant.settings.csrf"
	case strings.HasPrefix(path, "/platform"):
		return "platform.csrf"
	default:
		return "tenants.activate"
	}
}
