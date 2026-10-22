package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"gopanel/internal/platform/auth"
	"gopanel/internal/platform/tenancy"
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
	case strings.HasPrefix(path, "/app/products"):
		return "products.csrf"
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

// PassiveProtected uses the normal HTML authorization flow without renewing idle time.
func (h *Handler) PassiveProtected(r chi.Router, routes func(chi.Router)) {
	r.Group(func(r chi.Router) { r.Use(noStore, h.passiveSession, h.Tenant); routes(r) })
}
func (h *Handler) passiveSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := h.service.AuthenticatePassive(r.Context(), h.token(r))
		if errors.Is(err, auth.ErrUnauthorized) {
			redirect(w, r, "/login")
			return
		}
		if err != nil {
			h.failure(w, r)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)))
	})
}

// Stream has explicit status codes: EventSource must never follow HTML login redirects.
func (h *Handler) Stream(next http.Handler) http.Handler {
	return noStore(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" || (r.Header.Get("Origin") != "" && !sameOrigin(r, h.secure)) {
			http.Error(w, "forbidden", 403)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		p, err := h.service.AuthenticatePassive(ctx, h.token(r))
		if errors.Is(err, auth.ErrUnauthorized) {
			cancel()
			http.Error(w, "unauthorized", 401)
			return
		}
		if err != nil {
			cancel()
			http.Error(w, "unavailable", 503)
			return
		}
		member, err := h.service.Membership(ctx, p)
		cancel()
		if errors.Is(err, auth.ErrTenant) {
			http.Error(w, "forbidden", 403)
			return
		}
		if err != nil {
			http.Error(w, "unavailable", 503)
			return
		}
		actor := tenancy.ActorContext{UserID: p.UserID(), TenantID: member.TenantID, TenantName: member.TenantName, Role: member.Role}
		requestCtx := context.WithValue(tenancy.WithActor(r.Context(), actor), principalKey{}, p)
		next.ServeHTTP(w, r.WithContext(requestCtx))
	}))
}
func sameOrigin(r *http.Request, secure bool) bool {
	scheme := "http"
	if secure {
		scheme = "https"
	}
	return r.Header.Get("Origin") == scheme+"://"+r.Host
}
func (h *Handler) StreamAuthorized(r *http.Request) error {
	return h.service.StreamAuthorized(r.Context(), principal(r))
}
