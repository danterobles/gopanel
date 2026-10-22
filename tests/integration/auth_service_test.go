//go:build integration

package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	dbgen "gopanel/internal/db/generated"
	"gopanel/internal/platform/auth"
	"gopanel/internal/platform/auth/password"
	"gopanel/internal/platform/id"
)

func authIdentity(t *testing.T, db *sql.DB) string {
	t.Helper()
	user := id.New()
	if err := dbgen.New(db).CreateUser(t.Context(), dbgen.CreateUserParams{ID: user, Email: "clock@auth.test", PasswordHash: password.Hash(fixturePassword), DisplayName: "Clock"}); err != nil {
		t.Fatal(err)
	}
	return user
}
func TestSessionClockTouchAbsoluteRotationPersistenceAndCleanup(t *testing.T) {
	cfg, db := scratch(t, true)
	ctx := t.Context()
	user := authIdentity(t, db)
	tenant, _, _ := tenantOwner(t, db, "clock-tenant")
	q := dbgen.New(db)
	if err := q.CreateMembership(ctx, dbgen.CreateMembershipParams{ID: id.New(), TenantID: tenant, UserID: user, Role: "viewer", IsActive: true}); err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Truncate(time.Microsecond)
	now := start
	clock := func() time.Time { return now }
	service := auth.New(db, cfg, clock)
	issued, err := service.Login(ctx, "clock@auth.test", fixturePassword, "", "clock-ip", "test")
	if err != nil {
		t.Fatal(err)
	}
	hash := hashToken(t, issued.Token)
	getSeen := func() time.Time {
		t.Helper()
		var seen time.Time
		if err := db.QueryRowContext(ctx, "SELECT last_seen_at FROM sessions WHERE token_hash=?", hash).Scan(&seen); err != nil {
			t.Fatal(err)
		}
		return seen
	}
	now = start.Add(30 * time.Second)
	if _, err = service.Authenticate(ctx, issued.Token); err != nil {
		t.Fatal(err)
	}
	if !getSeen().Equal(start) {
		t.Fatal("session touched on every request")
	}
	now = start.Add(61 * time.Second)
	p, err := service.Authenticate(ctx, issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	if !getSeen().Equal(now) {
		t.Fatal("due session touch was not persisted")
	}
	restarted := auth.New(db, cfg, clock)
	recovered, err := restarted.Authenticate(ctx, issued.Token)
	if err != nil || recovered.CSRF() != p.CSRF() {
		t.Fatal("session or CSRF did not survive service restart", err)
	}
	rotated, err := restarted.Activate(ctx, recovered, tenant, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !rotated.ExpiresAt.Equal(issued.ExpiresAt) || rotated.Token == issued.Token {
		t.Fatal("rotation extended absolute expiry or reused token")
	}
	if _, err = service.Authenticate(ctx, issued.Token); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("rotated token remains valid")
	}
	hash = hashToken(t, rotated.Token)
	var created time.Time
	if err = db.QueryRowContext(ctx, "SELECT created_at FROM sessions WHERE token_hash=?", hash).Scan(&created); err != nil || !created.Equal(start) {
		t.Fatal("rotation reset creation time")
	}
	now = now.Add(cfg.SessionIdleTTL)
	if _, err = service.Authenticate(ctx, rotated.Token); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("idle boundary accepted")
	}
	if err = service.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sessions").Scan(&count); err != nil || count != 0 {
		t.Fatal("cleanup retained idle session")
	}
	now = start
	issued, err = service.Login(ctx, "clock@auth.test", fixturePassword, "", "other-ip", "test")
	if err != nil {
		t.Fatal(err)
	}
	now = issued.ExpiresAt
	if _, err = db.ExecContext(ctx, "UPDATE sessions SET last_seen_at=? WHERE token_hash=?", now.Add(-time.Second), hashToken(t, issued.Token)); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Authenticate(ctx, issued.Token); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("absolute boundary accepted")
	}
}

func TestLoginRateLimitIsAtomicResetsAndIgnoresAccountExistence(t *testing.T) {
	cfg, db := scratch(t, true)
	cfg.LoginAttemptsPerMinute = 2
	now := time.Now().UTC().Truncate(time.Microsecond)
	service := auth.New(db, cfg, func() time.Time { return now })
	start := make(chan struct{})
	results := make(chan error, 6)
	var wg sync.WaitGroup
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := service.Login(context.Background(), "unknown@auth.test", fixturePassword, "", "192.0.2.50", "test")
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	credentials, limited := 0, 0
	for err := range results {
		switch {
		case errors.Is(err, auth.ErrCredentials):
			credentials++
		case errors.Is(err, auth.ErrLimited):
			limited++
		default:
			t.Fatalf("unexpected concurrent admission: %v", err)
		}
	}
	if credentials != 2 || limited != 4 {
		t.Fatalf("non-atomic login admission: allowed=%d limited=%d", credentials, limited)
	}
	var attempts int
	if err := db.QueryRowContext(t.Context(), "SELECT attempts FROM login_rate_limits").Scan(&attempts); err != nil || attempts != 2 {
		t.Fatal("invalid persisted attempt count", err)
	}
	now = now.Add(time.Minute)
	if _, err := service.Login(t.Context(), "unknown@auth.test", fixturePassword, "", "192.0.2.50", "test"); !errors.Is(err, auth.ErrCredentials) {
		t.Fatal("login window did not reset", err)
	}
	now = now.Add(25 * time.Hour)
	if err := service.Cleanup(t.Context()); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM login_rate_limits").Scan(&rows); err != nil || rows != 0 {
		t.Fatal("old admission buckets not cleaned")
	}
}

func TestAuthenticationAuditHasNoSecretsAndSessionsRollbackOnAuditFailure(t *testing.T) {
	cfg, db := scratch(t, true)
	ctx := t.Context()
	user := authIdentity(t, db)
	service := auth.New(db, cfg, nil)
	issued, err := service.Login(ctx, "clock@auth.test", fixturePassword, "", "audit-ip", "request-test")
	if err != nil {
		t.Fatal(err)
	}
	p, err := service.Authenticate(ctx, issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Logout(ctx, p, "logout-test"); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Login(ctx, "missing@auth.test", fixturePassword, "", "audit-ip", "denial-test"); !errors.Is(err, auth.ErrCredentials) {
		t.Fatal(err)
	}
	rows, err := db.QueryContext(ctx, "SELECT action,outcome,COALESCE(actor_user_id,''),COALESCE(tenant_id,''),COALESCE(entity_id,''),COALESCE(metadata,'') FROM audit_logs ORDER BY created_at,id")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for rows.Next() {
		var action, outcome, actor, tenant, entity, metadata string
		if err = rows.Scan(&action, &outcome, &actor, &tenant, &entity, &metadata); err != nil {
			t.Fatal(err)
		}
		if outcome == "denied" && (actor != "" || tenant != "") {
			t.Fatal("credential denial identifies account")
		}
		if outcome == "success" && actor != user {
			t.Fatal("audit actor not session identity")
		}
		combined := action + outcome + actor + tenant + entity + metadata
		for _, secret := range []string{fixturePassword, issued.Token, p.CSRF(), "clock@auth.test", "missing@auth.test"} {
			if strings.Contains(combined, secret) {
				t.Fatal("secret or PII in audit")
			}
		}
		seen[action+"."+outcome]++
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if seen["auth.login.success"] != 1 || seen["auth.logout.success"] != 1 || seen["auth.login.denied"] != 1 {
		t.Fatal("missing authentication audit outcomes")
	}
	// A missing audit table in this disposable database must prevent an unaudited login.
	if _, err = db.ExecContext(ctx, "RENAME TABLE audit_logs TO unavailable_audit_logs"); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Login(ctx, "clock@auth.test", fixturePassword, "", "audit-ip", "audit-failure"); err == nil {
		t.Fatal("unaudited login committed")
	}
	var count int
	if err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sessions").Scan(&count); err != nil || count != 0 {
		t.Fatal("failed login audit retained session")
	}
}

func TestRepeatedLoginRevokesPreviousBrowserSession(t *testing.T) {
	cfg, db := scratch(t, true)
	authIdentity(t, db)
	service := auth.New(db, cfg, nil)
	ctx := t.Context()
	first, err := service.Login(ctx, "clock@auth.test", fixturePassword, "", "rotation-ip", "test")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Login(ctx, "clock@auth.test", fixturePassword, first.Token, "rotation-ip", "test")
	if err != nil {
		t.Fatal(err)
	}
	if first.Token == second.Token {
		t.Fatal("login reused token")
	}
	if _, err = service.Authenticate(ctx, first.Token); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("previous login token retained access")
	}
	if _, err = service.Authenticate(ctx, second.Token); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sessions").Scan(&count); err != nil || count != 1 {
		t.Fatal("login rotation retained previous session")
	}
}
