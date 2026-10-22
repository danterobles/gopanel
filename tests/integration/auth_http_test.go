//go:build integration

package integration_test

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"gopanel/internal/config"
	dbgen "gopanel/internal/db/generated"
	members "gopanel/internal/modules/members/repository"
	"gopanel/internal/platform/auth"
	authhandler "gopanel/internal/platform/auth/handler"
	"gopanel/internal/platform/auth/password"
	httpserver "gopanel/internal/platform/http"
	"gopanel/internal/platform/id"
)

const fixturePassword = "http-test-fixture-only-password"

type logBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}
func (b *logBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.b.String() }

type authFixture struct {
	cfg                                                config.Config
	db                                                 *sql.DB
	service                                            *auth.Service
	server                                             *httptest.Server
	user, platform, north, south, southMember, foreign string
	logs                                               *logBuffer
	secure                                             bool
}

func newAuthFixture(t *testing.T, secure bool, limit ...int) *authFixture {
	t.Helper()
	cfg, db := scratch(t, true)
	cfg.LoginAttemptsPerMinute = 100
	if len(limit) > 0 {
		cfg.LoginAttemptsPerMinute = limit[0]
	}
	north, _, _ := tenantOwner(t, db, "north")
	south, _, _ := tenantOwner(t, db, "south")
	foreign, _, _ := tenantOwner(t, db, "secret-company")
	q := dbgen.New(db)
	hash := password.Hash(fixturePassword)
	user, platform := id.New(), id.New()
	for _, entry := range []struct {
		id, email string
		platform  bool
	}{{user, "member@auth.test", false}, {platform, "platform@auth.test", true}} {
		if err := q.CreateUser(t.Context(), dbgen.CreateUserParams{ID: entry.id, Email: entry.email, PasswordHash: hash, DisplayName: "Fixture", IsPlatformAdmin: entry.platform}); err != nil {
			t.Fatal(err)
		}
	}
	r := members.New(db)
	if _, err := r.Add(t.Context(), north, user, "admin"); err != nil {
		t.Fatal(err)
	}
	southMember, err := r.Add(t.Context(), south, user, "viewer")
	if err != nil {
		t.Fatal(err)
	}
	service := auth.New(db, cfg, nil)
	logs := &logBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	router := httpserver.NewRouter(db, logger, cfg.HealthTimeout)
	authhandler.New(service, secure, logger).Register(router)
	var server *httptest.Server
	if secure {
		server = httptest.NewTLSServer(router)
	} else {
		server = httptest.NewServer(router)
	}
	t.Cleanup(server.Close)
	return &authFixture{cfg: cfg, db: db, service: service, server: server, user: user, platform: platform, north: north, south: south, southMember: southMember, foreign: foreign, logs: logs, secure: secure}
}
func (f *authFixture) client() *http.Client {
	client := f.server.Client()
	copyClient := *client
	copyClient.Jar, _ = cookiejar.New(nil)
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &copyClient
}
func (f *authFixture) send(t *testing.T, client *http.Client, method, path string, form url.Values, headers map[string]string) (*http.Response, string) {
	t.Helper()
	var reader io.Reader
	if form != nil {
		reader = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, f.server.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusMethodNotAllowed && response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("auth response is cacheable")
	}
	return response, string(body)
}
func csrfFrom(t *testing.T, body string) string {
	t.Helper()
	match := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(body)
	if len(match) != 2 {
		t.Fatal("CSRF form field missing")
	}
	return match[1]
}
func status(t *testing.T, response *http.Response, want int) {
	t.Helper()
	if response.StatusCode != want {
		t.Fatalf("HTTP status %d want %d", response.StatusCode, want)
	}
}
func (f *authFixture) cookieValue(t *testing.T, client *http.Client) string {
	t.Helper()
	base, _ := url.Parse(f.server.URL)
	name := "gopanel_session"
	if f.secure {
		name = "__Host-gopanel_session"
	}
	for _, c := range client.Jar.Cookies(base) {
		if c.Name == name {
			return c.Value
		}
	}
	t.Fatal("session cookie missing")
	return ""
}
func (f *authFixture) login(t *testing.T, client *http.Client, email string) string {
	t.Helper()
	response, body := f.send(t, client, "GET", "/login", nil, nil)
	status(t, response, 200)
	response, _ = f.send(t, client, "POST", "/login", url.Values{"csrf_token": {csrfFrom(t, body)}, "email": {email}, "password": {fixturePassword}, "user_id": {f.platform}, "role": {"owner"}}, nil)
	status(t, response, 303)
	if response.Header.Get("Location") != "/app/tenants/select" {
		t.Fatal("unexpected login redirect")
	}
	return f.cookieValue(t, client)
}
func hashToken(t *testing.T, token string) []byte {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(raw)
	return hash[:]
}

func TestHTTPLoginTenantRotationRevocationAndLogout(t *testing.T) {
	f := newAuthFixture(t, false)
	client := f.client()
	response, _ := f.send(t, client, "GET", "/app", nil, map[string]string{"HX-Request": "true"})
	status(t, response, 200)
	if response.Header.Get("HX-Redirect") != "/login" {
		t.Fatal("HTMX request bypassed authentication")
	}
	oldToken := f.login(t, client, " MEMBER@AUTH.TEST ")
	response, body := f.send(t, client, "GET", "/app/tenants/select", nil, nil)
	status(t, response, 200)
	if !strings.Contains(body, "north") || !strings.Contains(body, "south") || strings.Contains(body, "secret-company") || strings.Contains(body, oldToken) {
		t.Fatal("selector leaked or omitted tenant/session data")
	}
	csrf := csrfFrom(t, body)
	response, _ = f.send(t, client, "POST", "/app/tenants/activate", url.Values{"csrf_token": {csrf}, "tenant_id": {f.north}, "role": {"owner"}, "user_id": {f.platform}}, nil)
	status(t, response, 303)
	newToken := f.cookieValue(t, client)
	if oldToken == newToken {
		t.Fatal("tenant activation did not rotate session")
	}
	if _, err := f.service.Authenticate(t.Context(), oldToken); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("old tenant token remains usable")
	}
	response, body = f.send(t, client, "GET", "/app", nil, nil)
	status(t, response, 200)
	if !strings.Contains(body, "north") || !strings.Contains(body, "admin") || strings.Contains(body, newToken) {
		t.Fatal("untrusted actor fields used or token exposed")
	}
	response, _ = f.send(t, client, "POST", "/app/tenants/activate", url.Values{"csrf_token": {csrf}, "tenant_id": {f.south}}, nil)
	status(t, response, 403)
	csrf = csrfFrom(t, body)
	response, _ = f.send(t, client, "POST", "/app/tenants/activate", url.Values{"csrf_token": {csrf}, "tenant_id": {f.south}, "role": {"owner"}}, map[string]string{"HX-Request": "true"})
	status(t, response, 200)
	if response.Header.Get("HX-Redirect") != "/app" {
		t.Fatal("missing HTMX activation redirect")
	}
	response, body = f.send(t, client, "GET", "/app", nil, nil)
	status(t, response, 200)
	if !strings.Contains(body, "south") || !strings.Contains(body, "viewer") {
		t.Fatal("tenant role not reloaded from membership")
	}
	csrf = csrfFrom(t, body)
	response, denied := f.send(t, client, "POST", "/app/tenants/activate", url.Values{"csrf_token": {csrf}, "tenant_id": {f.foreign}}, map[string]string{"HX-Request": "true"})
	status(t, response, 403)
	if strings.Contains(denied, "<!DOCTYPE") || strings.Contains(denied, f.foreign) || strings.Contains(denied, "secret-company") {
		t.Fatal("denied activation leaked foreign data or returned full HTMX page")
	}
	if err := members.New(f.db).Change(t.Context(), f.south, f.southMember, "viewer", false); err != nil {
		t.Fatal(err)
	}
	response, _ = f.send(t, client, "GET", "/app", nil, nil)
	status(t, response, 303)
	if response.Header.Get("Location") != "/app/tenants/select" {
		t.Fatal("revoked membership retained access")
	}
	response, body = f.send(t, client, "GET", "/app/tenants/select", nil, nil)
	status(t, response, 200)
	if strings.Contains(body, f.south) {
		t.Fatal("inactive membership listed")
	}
	csrf = csrfFrom(t, body)
	response, _ = f.send(t, client, "GET", "/logout", nil, nil)
	status(t, response, 405)
	response, _ = f.send(t, client, "POST", "/logout", url.Values{}, nil)
	status(t, response, 403)
	token := f.cookieValue(t, client)
	response, _ = f.send(t, client, "POST", "/logout", url.Values{"csrf_token": {csrf}}, map[string]string{"HX-Request": "true"})
	status(t, response, 200)
	if response.Header.Get("HX-Redirect") != "/login" {
		t.Fatal("missing logout redirect")
	}
	if _, err := f.service.Authenticate(t.Context(), token); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("logout failed to revoke token")
	}
	for _, secret := range []string{fixturePassword, oldToken, newToken, csrf} {
		if strings.Contains(f.logs.String(), secret) {
			t.Fatal("secret in structured logs")
		}
	}
}

func TestHTTPLoginCSRFGenericErrorsAndValidationFragments(t *testing.T) {
	f := newAuthFixture(t, false)
	client := f.client()
	response, body := f.send(t, client, "GET", "/login", nil, nil)
	status(t, response, 200)
	csrf := csrfFrom(t, body)
	response, _ = f.send(t, client, "POST", "/login", url.Values{"email": {"member@auth.test"}, "password": {fixturePassword}}, nil)
	status(t, response, 403)
	response, _ = f.send(t, client, "POST", "/login", url.Values{"csrf_token": {csrf}, "email": {"member@auth.test"}, "password": {fixturePassword}}, map[string]string{"Origin": "https://attacker.test"})
	status(t, response, 403)
	response, _ = f.send(t, client, "POST", "/login?csrf_token="+csrf, url.Values{"email": {"member@auth.test"}, "password": {fixturePassword}}, nil)
	status(t, response, 403)
	response, fragment := f.send(t, client, "POST", "/login", url.Values{"csrf_token": {csrf}, "email": {"bad-email"}, "password": {fixturePassword}}, map[string]string{"HX-Request": "true"})
	status(t, response, 422)
	if strings.Contains(fragment, "<!DOCTYPE") || !strings.Contains(fragment, "login-form") || strings.Contains(fragment, fixturePassword) {
		t.Fatal("invalid HTMX validation fragment")
	}
	for _, email := range []string{"missing@auth.test", "platform@auth.test"} {
		if email == "platform@auth.test" {
			if _, err := f.db.ExecContext(t.Context(), "UPDATE users SET is_active=FALSE WHERE id=?", f.platform); err != nil {
				t.Fatal(err)
			}
		}
		response, fragment = f.send(t, client, "POST", "/login", url.Values{"csrf_token": {csrf}, "email": {email}, "password": {fixturePassword}}, map[string]string{"HX-Request": "true"})
		status(t, response, 401)
		if !strings.Contains(fragment, "Correo o contraseña incorrectos.") || strings.Contains(fragment, "<!DOCTYPE") || strings.Contains(fragment, fixturePassword) {
			t.Fatal("credential error leaked state")
		}
	}
	var count int
	if err := f.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM sessions").Scan(&count); err != nil || count != 0 {
		t.Fatal("denied login created a session")
	}
	request, err := http.NewRequest("POST", f.server.URL+"/login", strings.NewReader(strings.Repeat("x", 17000)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	status(t, response, 413)
}

func TestHTTPPlatformAdminHasNoImplicitTenantAndSecureCookies(t *testing.T) {
	f := newAuthFixture(t, true)
	client := f.client()
	response, body := f.send(t, client, "GET", "/login", nil, nil)
	status(t, response, 200)
	for _, cookie := range response.Cookies() {
		if !cookie.Secure || !cookie.HttpOnly || cookie.Path != "/" || cookie.Domain != "" || cookie.SameSite != http.SameSiteLaxMode || !strings.HasPrefix(cookie.Name, "__Host-") {
			t.Fatal("unsafe production cookie")
		}
	}
	response, _ = f.send(t, client, "POST", "/login", url.Values{"csrf_token": {csrfFrom(t, body)}, "email": {"platform@auth.test"}, "password": {fixturePassword}}, nil)
	status(t, response, 303)
	found := false
	for _, cookie := range response.Cookies() {
		if cookie.Name == "__Host-gopanel_session" {
			found = true
			if !cookie.Secure || !cookie.HttpOnly || cookie.Path != "/" || cookie.Domain != "" || cookie.SameSite != http.SameSiteLaxMode {
				t.Fatal("unsafe session cookie")
			}
		}
	}
	if !found {
		t.Fatal("host-prefixed session cookie not issued")
	}
	response, body = f.send(t, client, "GET", "/app/tenants/select", nil, nil)
	status(t, response, 200)
	if !strings.Contains(body, "No tienes empresas disponibles") || strings.Contains(body, f.north) || strings.Contains(body, f.south) {
		t.Fatal("platform admin inherited commercial permissions")
	}
	response, _ = f.send(t, client, "POST", "/app/tenants/activate", url.Values{"csrf_token": {csrfFrom(t, body)}, "tenant_id": {f.north}}, nil)
	status(t, response, 403)
}

func TestHTTPExpiredIdleMalformedAndInactiveSessions(t *testing.T) {
	f := newAuthFixture(t, false)
	for _, kind := range []string{"absolute", "idle", "user", "malformed"} {
		t.Run(kind, func(t *testing.T) {
			client := f.client()
			token := f.login(t, client, "member@auth.test")
			hash := hashToken(t, token)
			switch kind {
			case "absolute":
				_, _ = f.db.ExecContext(t.Context(), "UPDATE sessions SET expires_at=? WHERE token_hash=?", time.Now().UTC().Add(-time.Second), hash)
			case "idle":
				_, _ = f.db.ExecContext(t.Context(), "UPDATE sessions SET last_seen_at=? WHERE token_hash=?", time.Now().UTC().Add(-f.cfg.SessionIdleTTL-time.Second), hash)
			case "user":
				_, _ = f.db.ExecContext(t.Context(), "UPDATE users SET is_active=FALSE WHERE id=?", f.user)
				defer f.db.ExecContext(t.Context(), "UPDATE users SET is_active=TRUE WHERE id=?", f.user)
			case "malformed":
				base, _ := url.Parse(f.server.URL)
				client.Jar.SetCookies(base, []*http.Cookie{{Name: "gopanel_session", Value: "malformed", Path: "/"}})
			}
			response, _ := f.send(t, client, "GET", "/app/tenants/select", nil, nil)
			status(t, response, 303)
			if response.Header.Get("Location") != "/login" {
				t.Fatal("unavailable session retained access")
			}
		})
	}
}

func TestHTTPLoginLimitCannotBeBypassedWithProxyHeaders(t *testing.T) {
	f := newAuthFixture(t, false, 1)
	client := f.client()
	response, body := f.send(t, client, "GET", "/login", nil, nil)
	status(t, response, 200)
	form := url.Values{"csrf_token": {csrfFrom(t, body)}, "email": {"missing@auth.test"}, "password": {fixturePassword}}
	response, _ = f.send(t, client, "POST", "/login", form, map[string]string{"X-Forwarded-For": "192.0.2.1"})
	status(t, response, 401)
	form.Set("email", "member@auth.test")
	response, body = f.send(t, client, "POST", "/login", form, map[string]string{"X-Forwarded-For": "192.0.2.2", "HX-Request": "true"})
	status(t, response, 429)
	if response.Header.Get("Retry-After") != "60" || strings.Contains(body, "<!DOCTYPE") {
		t.Fatal("invalid rate-limit response")
	}
	var count int
	if err := f.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM sessions").Scan(&count); err != nil || count != 0 {
		t.Fatal("limited login created session")
	}
}
