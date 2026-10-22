package handler

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"
	"gopanel/internal/platform/auth"
	"gopanel/internal/platform/rbac"
	"gopanel/internal/platform/tenancy"
	"gopanel/internal/ui/pages"
)

type principalKey struct{}
type Handler struct {
	service *auth.Service
	secure  bool
	logger  *slog.Logger
	origin  *http.CrossOriginProtection
}

func New(service *auth.Service, secure bool, logger *slog.Logger) *Handler {
	h := &Handler{service: service, secure: secure, logger: logger, origin: http.NewCrossOriginProtection()}
	h.origin.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.deny(w, r, "Solicitud de otro origen rechazada.") }))
	return h
}
func (h *Handler) Register(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(noStore)
		r.Get("/login", h.loginPage)
		r.With(h.origin.Handler, h.parse, h.loginCSRF).Post("/login", h.login)
		r.Group(func(r chi.Router) {
			r.Use(h.Session)
			r.Get("/app/tenants/select", h.selectPage)
			r.With(h.origin.Handler, h.parse, h.sessionCSRF).Post("/app/tenants/activate", h.activate)
			r.With(h.origin.Handler, h.parse, h.sessionCSRF).Post("/logout", h.logout)
			r.With(h.Tenant).Get("/app", h.app)
		})
	})
}
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Add("Vary", "HX-Request")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}
func requestID(w http.ResponseWriter) string { return w.Header().Get("X-Request-ID") }
func principal(r *http.Request) auth.Principal {
	p, _ := r.Context().Value(principalKey{}).(auth.Principal)
	return p
}
func (h *Handler) sessionName() string {
	if h.secure {
		return "__Host-gopanel_session"
	}
	return "gopanel_session"
}
func (h *Handler) csrfName() string {
	if h.secure {
		return "__Host-gopanel_login_csrf"
	}
	return "gopanel_login_csrf"
}
func (h *Handler) cookie(name, value string, expires time.Time) *http.Cookie {
	return &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: max(1, int(time.Until(expires).Seconds()))}
}
func (h *Handler) clear(w http.ResponseWriter, name string) {
	c := h.cookie(name, "", time.Unix(1, 0))
	c.MaxAge = -1
	http.SetCookie(w, c)
}
func (h *Handler) issue(w http.ResponseWriter, result auth.Issued) {
	http.SetCookie(w, h.cookie(h.sessionName(), result.Token, result.ExpiresAt))
}
func (h *Handler) token(r *http.Request) string {
	c, err := r.Cookie(h.sessionName())
	if err != nil {
		return ""
	}
	return c.Value
}
func redirect(w http.ResponseWriter, r *http.Request, path string) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", path)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, path, http.StatusSeeOther)
}
func (h *Handler) render(w http.ResponseWriter, r *http.Request, status int, full, fragment templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	component := full
	if r.Header.Get("HX-Request") == "true" {
		component = fragment
	}
	if err := component.Render(r.Context(), w); err != nil {
		h.logger.ErrorContext(r.Context(), "auth_render_failed", "request_id", requestID(w))
	}
}
func (h *Handler) failure(w http.ResponseWriter, r *http.Request) {
	h.logger.ErrorContext(r.Context(), "auth_operation_failed", "request_id", requestID(w))
	message := "El servicio no está disponible. Intenta de nuevo en unos momentos."
	h.render(w, r, 503, pages.AuthError(message), pages.AuthNotice(message))
}
func action(path string) string {
	switch path {
	case "/login":
		return "auth.login"
	case "/logout":
		return "auth.logout"
	default:
		return mutationAction(path)
	}
}
func (h *Handler) deny(w http.ResponseWriter, r *http.Request, message string) {
	var p *auth.Principal
	if current, ok := r.Context().Value(principalKey{}).(auth.Principal); ok {
		p = &current
	}
	if err := h.service.Denied(r.Context(), action(r.URL.Path), requestID(w), p); err != nil {
		h.failure(w, r)
		return
	}
	h.render(w, r, 403, pages.AuthError(message), pages.AuthNotice(message))
}
func (h *Handler) parse(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		if err := r.ParseForm(); err != nil {
			status := 400
			var large *http.MaxBytesError
			if errors.As(err, &large) {
				status = 413
			}
			message := "El formulario no es válido o excede el tamaño permitido."
			h.render(w, r, status, pages.AuthError(message), pages.AuthNotice(message))
			return
		}
		next.ServeHTTP(w, r)
	})
}
func suppliedCSRF(r *http.Request) string {
	if value := r.Header.Get("X-CSRF-Token"); value != "" {
		return value
	}
	return r.PostForm.Get("csrf_token")
}
func validCSRF(got, want string) bool {
	return len(got) == 43 && len(want) == 43 && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}
func (h *Handler) loginCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(h.csrfName())
		if err != nil || !validCSRF(suppliedCSRF(r), cookie.Value) {
			h.deny(w, r, "El formulario venció o no es válido. Vuelve a iniciar sesión.")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (h *Handler) sessionCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validCSRF(suppliedCSRF(r), principal(r).CSRF()) {
			h.deny(w, r, "El formulario venció o cambió la sesión. Recarga la página.")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (h *Handler) Session(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := h.service.Authenticate(r.Context(), h.token(r))
		if errors.Is(err, auth.ErrUnauthorized) {
			h.clear(w, h.sessionName())
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

// Tenant must be used after Session. Its ActorContext is never built from request fields.
func (h *Handler) Tenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := principal(r)
		membership, err := h.service.Membership(r.Context(), p)
		if errors.Is(err, auth.ErrTenant) {
			redirect(w, r, "/app/tenants/select")
			return
		}
		if err != nil {
			h.failure(w, r)
			return
		}
		actor := tenancy.ActorContext{UserID: p.UserID(), TenantID: membership.TenantID, TenantName: membership.TenantName, Role: membership.Role}
		next.ServeHTTP(w, r.WithContext(tenancy.WithActor(r.Context(), actor)))
	})
}
func (h *Handler) loginPage(w http.ResponseWriter, r *http.Request) {
	if token := h.token(r); token != "" {
		p, err := h.service.Authenticate(r.Context(), token)
		if err == nil {
			path := "/app/tenants/select"
			if p.TenantID() != "" {
				path = "/app"
			}
			redirect(w, r, path)
			return
		}
		if !errors.Is(err, auth.ErrUnauthorized) {
			h.failure(w, r)
			return
		}
		h.clear(w, h.sessionName())
	}
	csrf := ""
	if cookie, err := r.Cookie(h.csrfName()); err == nil {
		if raw, e := base64.RawURLEncoding.Strict().DecodeString(cookie.Value); e == nil && len(raw) == 32 {
			csrf = cookie.Value
		}
	}
	if csrf == "" {
		raw := make([]byte, 32)
		_, _ = rand.Read(raw)
		csrf = base64.RawURLEncoding.EncodeToString(raw)
	}
	http.SetCookie(w, h.cookie(h.csrfName(), csrf, time.Now().Add(time.Hour)))
	h.render(w, r, 200, pages.LoginPage(csrf, "", ""), pages.LoginForm(csrf, "", ""))
}
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	email := strings.ToLower(strings.TrimSpace(r.PostForm.Get("email")))
	value := r.PostForm.Get("password")
	csrf := suppliedCSRF(r)
	address, err := mail.ParseAddress(email)
	if len(email) > 254 || err != nil || address.Address != email || len(value) == 0 || len(value) > 1024 {
		message := "Ingresa un correo válido y una contraseña de hasta 1024 caracteres."
		h.render(w, r, 422, pages.LoginPage(csrf, email, message), pages.LoginForm(csrf, email, message))
		return
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	issued, err := h.service.Login(r.Context(), email, value, h.token(r), ip, requestID(w))
	if errors.Is(err, auth.ErrCredentials) {
		message := "Correo o contraseña incorrectos."
		h.render(w, r, 401, pages.LoginPage(csrf, email, message), pages.LoginForm(csrf, email, message))
		return
	}
	if errors.Is(err, auth.ErrLimited) || errors.Is(err, auth.ErrBusy) {
		w.Header().Set("Retry-After", "60")
		message := "Demasiados intentos. Espera un minuto antes de volver a intentar."
		h.render(w, r, 429, pages.LoginPage(csrf, email, message), pages.LoginForm(csrf, email, message))
		return
	}
	if err != nil {
		h.failure(w, r)
		return
	}
	h.issue(w, issued)
	h.clear(w, h.csrfName())
	redirect(w, r, "/app/tenants/select")
}
func (h *Handler) selectPage(w http.ResponseWriter, r *http.Request) { h.selectResponse(w, r, 200, "") }
func (h *Handler) selectResponse(w http.ResponseWriter, r *http.Request, status int, message string) {
	p := principal(r)
	rows, err := h.service.Tenants(r.Context(), p)
	if err != nil {
		h.failure(w, r)
		return
	}
	platform, err := h.service.PlatformAdmin(r.Context(), p)
	if err != nil {
		h.failure(w, r)
		return
	}
	h.render(w, r, status, pages.TenantPage(p.CSRF(), rows, message, platform), pages.TenantForm(p.CSRF(), rows, message))
}
func (h *Handler) activate(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	result, err := h.service.Activate(r.Context(), p, r.PostForm.Get("tenant_id"), requestID(w))
	if errors.Is(err, auth.ErrTenant) {
		if err = h.service.Denied(r.Context(), "tenants.activate", requestID(w), &p); err != nil {
			h.failure(w, r)
			return
		}
		h.selectResponse(w, r, 403, "La empresa solicitada no está disponible para tu cuenta.")
		return
	}
	if errors.Is(err, auth.ErrUnauthorized) {
		h.clear(w, h.sessionName())
		redirect(w, r, "/login")
		return
	}
	if err != nil {
		h.failure(w, r)
		return
	}
	h.issue(w, result)
	redirect(w, r, "/app")
}
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	err := h.service.Logout(r.Context(), principal(r), requestID(w))
	if err != nil && !errors.Is(err, auth.ErrUnauthorized) {
		h.failure(w, r)
		return
	}
	h.clear(w, h.sessionName())
	h.clear(w, h.csrfName())
	redirect(w, r, "/login")
}
func (h *Handler) app(w http.ResponseWriter, r *http.Request) {
	actor, _ := tenancy.Actor(r.Context())
	p := principal(r)
	component := pages.ActiveTenant(p.CSRF(), actor.TenantName, actor.Role, rbac.Can(actor.Role, rbac.MembersRead), rbac.Can(actor.Role, rbac.TenantSettings))
	h.render(w, r, 200, component, component)
}
