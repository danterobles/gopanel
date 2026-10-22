//go:build e2e || load

package e2e_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"gopanel/internal/config"
	"gopanel/internal/platform/database"
	"gopanel/internal/platform/id"
	"gopanel/internal/platform/seed"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"
)

type fixture struct {
	cfg           config.Config
	db            *sql.DB
	credentials   map[string]string
	tenants       map[string]string
	address, base string
	command       *exec.Cmd
	log           *os.File
	dir           string
}

func scratch(t *testing.T, apply bool) (config.Config, *sql.DB) {
	t.Helper()
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	if user := os.Getenv("TEST_DB_USER"); user != "" {
		cfg.DBUser = user
		cfg.DBPassword = os.Getenv("TEST_DB_PASSWORD")
	}
	admin, err := database.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	name := "gopanel_test_" + strings.ReplaceAll(id.New(), "-", "")
	if _, err = admin.ExecContext(t.Context(), "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci"); err != nil {
		t.Fatal("create scratch database:", err)
	}
	cfg.DBName = name
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.ExecContext(ctx, "DROP DATABASE `"+name+"`"); err != nil {
			t.Error("scratch cleanup failed")
		}
		admin.Close()
	})
	if apply {
		if err = database.Migrate(t.Context(), cfg, "up"); err != nil {
			t.Fatal(err)
		}
	}
	return cfg, db
}
func setup(t *testing.T, count int) *fixture {
	t.Helper()
	cfg, db := scratch(t, true)
	dir := t.TempDir()
	credentialsPath := filepath.Join(dir, "credentials.json")
	if _, err := seed.Run(t.Context(), db, seed.Options{Count: count, CredentialsPath: credentialsPath}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(credentialsPath)
	if err != nil {
		t.Fatal(err)
	}
	var credentials []seed.Credential
	if json.Unmarshal(data, &credentials) != nil {
		t.Fatal("read private test credentials")
	}
	f := &fixture{cfg: cfg, db: db, dir: dir, credentials: map[string]string{}, tenants: map[string]string{}}
	for _, credential := range credentials {
		f.credentials[credential.Email] = credential.Password
	}
	rows, err := db.QueryContext(t.Context(), "SELECT id,slug FROM tenants")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var tenant, slug string
		if rows.Scan(&tenant, &slug) != nil {
			t.Fatal("tenant scan")
		}
		f.tenants[slug] = tenant
	}
	rows.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f.address = listener.Addr().String()
	listener.Close()
	f.base = "http://" + f.address
	f.log, err = os.OpenFile(filepath.Join(dir, "server.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.stop(t); f.log.Close() })
	return f
}
func (f *fixture) start(t *testing.T) {
	t.Helper()
	bin, err := filepath.Abs("../../bin/gopanel")
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(bin)
	blocked := map[string]bool{"DB_NAME": true, "DB_USER": true, "DB_PASSWORD": true, "HTTP_ADDR": true, "APP_ENV": true}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !blocked[key] {
			command.Env = append(command.Env, entry)
		}
	}
	command.Env = append(command.Env, "DB_NAME="+f.cfg.DBName, "DB_USER="+f.cfg.DBUser, "DB_PASSWORD="+f.cfg.DBPassword, "HTTP_ADDR="+f.address, "APP_ENV=development")
	command.Stdout = f.log
	command.Stderr = f.log
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	f.command = command
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(f.base + "/health/ready")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("native binary not ready")
}
func (f *fixture) stop(t *testing.T) {
	t.Helper()
	if f.command == nil {
		return
	}
	command := f.command
	f.command = nil
	_ = command.Process.Signal(syscall.SIGTERM)
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Error("native server did not shut down cleanly:", err)
		}
	case <-time.After(12 * time.Second):
		command.Process.Kill()
		<-done
		t.Error("native server shutdown timeout")
	}
}
func (f *fixture) client() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func (f *fixture) send(t *testing.T, c *http.Client, method, path string, form url.Values, hx bool) (*http.Response, string) {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(t.Context(), method, f.base+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if hx {
		req.Header.Set("HX-Request", "true")
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(raw)
}
func want(t *testing.T, resp *http.Response, status int) {
	t.Helper()
	if resp.StatusCode != status {
		t.Fatalf("HTTP %d want %d", resp.StatusCode, status)
	}
}

var tokenField = regexp.MustCompile(`name="csrf_token" value="([^"]+)"`)

func csrf(t *testing.T, body string) string {
	t.Helper()
	match := tokenField.FindStringSubmatch(body)
	if len(match) != 2 {
		t.Fatal("CSRF missing")
	}
	return match[1]
}
func (f *fixture) login(t *testing.T, c *http.Client, email string) {
	t.Helper()
	resp, body := f.send(t, c, "GET", "/login", nil, false)
	want(t, resp, 200)
	resp, _ = f.send(t, c, "POST", "/login", url.Values{"csrf_token": {csrf(t, body)}, "email": {email}, "password": {f.credentials[email]}}, false)
	want(t, resp, 303)
}
func (f *fixture) selectTenant(t *testing.T, c *http.Client, slug string) string {
	t.Helper()
	resp, body := f.send(t, c, "GET", "/app/tenants/select", nil, false)
	want(t, resp, 200)
	resp, _ = f.send(t, c, "POST", "/app/tenants/activate", url.Values{"csrf_token": {csrf(t, body)}, "tenant_id": {f.tenants[slug]}}, false)
	want(t, resp, 303)
	resp, body = f.send(t, c, "GET", "/app", nil, false)
	want(t, resp, 200)
	return csrf(t, body)
}
func writeReport(t *testing.T, name string, value any) {
	t.Helper()
	if err := os.MkdirAll("../../.local", 0700); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile("../../.local/"+name, data, 0600); err != nil {
		t.Fatal(err)
	}
}
