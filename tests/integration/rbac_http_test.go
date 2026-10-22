//go:build integration

package integration_test

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	dbgen "gopanel/internal/db/generated"
	"gopanel/internal/modules/members"
	memberrepo "gopanel/internal/modules/members/repository"
	"gopanel/internal/modules/tenants"
	"gopanel/internal/platform/auth"
	"gopanel/internal/platform/id"
	"gopanel/internal/platform/rbac"
	"gopanel/internal/platform/tenancy"
)

func (f *authFixture) selectTenant(t *testing.T, c *http.Client, tenant string) string {
	t.Helper()
	resp, body := f.send(t, c, "GET", "/app/tenants/select", nil, nil)
	status(t, resp, 200)
	resp, _ = f.send(t, c, "POST", "/app/tenants/activate", url.Values{"csrf_token": {csrfFrom(t, body)}, "tenant_id": {tenant}}, nil)
	status(t, resp, 303)
	resp, body = f.send(t, c, "GET", "/app", nil, nil)
	status(t, resp, 200)
	return csrfFrom(t, body)
}
func (f *authFixture) owner(t *testing.T, slug, tenant string) (*http.Client, string, string, string) {
	t.Helper()
	q := dbgen.New(f.db)
	user, err := q.GetUserByEmail(t.Context(), slug+"@fixture.test")
	if err != nil {
		t.Fatal(err)
	}
	hash, err := q.GetUserByEmail(t.Context(), "member@auth.test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.ExecContext(t.Context(), "UPDATE users SET password_hash=? WHERE id=?", hash.PasswordHash, user.ID); err != nil {
		t.Fatal(err)
	}
	var member string
	if err = f.db.QueryRowContext(t.Context(), "SELECT id FROM tenant_memberships WHERE tenant_id=? AND user_id=?", tenant, user.ID).Scan(&member); err != nil {
		t.Fatal(err)
	}
	c := f.client()
	f.login(t, c, user.Email)
	csrf := f.selectTenant(t, c, tenant)
	return c, user.ID, member, csrf
}
func (f *authFixture) newUser(t *testing.T, email string) string {
	t.Helper()
	q := dbgen.New(f.db)
	u, err := q.GetUserByEmail(t.Context(), "member@auth.test")
	if err != nil {
		t.Fatal(err)
	}
	value := id.New()
	if err = q.CreateUser(t.Context(), dbgen.CreateUserParams{ID: value, Email: email, PasswordHash: u.PasswordHash, DisplayName: "New fixture"}); err != nil {
		t.Fatal(err)
	}
	return value
}
func TestRBACHTTPRolesAndTenantBoundaries(t *testing.T) {
	f := newAuthFixture(t, false)
	c := f.client()
	f.login(t, c, "member@auth.test")
	csrf := f.selectTenant(t, c, f.north)
	resp, body := f.send(t, c, "GET", "/app/members", nil, nil)
	status(t, resp, 200)
	if !strings.Contains(body, "north@fixture.test") || strings.Contains(body, "south@fixture.test") || strings.Contains(body, "Agregar miembro existente") {
		t.Fatal("admin list leaked tenants or manage controls")
	}
	for _, entry := range []struct {
		path string
		form url.Values
	}{
		{"/app/members", url.Values{"email": {"platform@auth.test"}, "role": {"viewer"}}},
		{"/app/members/" + f.southMember, url.Values{"role": {"admin"}, "active": {"true"}}},
		{"/app/settings", url.Values{"name": {"forged"}}},
		{"/platform/tenants", url.Values{"name": {"forged"}, "slug": {"forged"}, "owner_email": {"member@auth.test"}}},
	} {
		entry.form.Set("csrf_token", csrf)
		entry.form.Set("tenant_id", f.south)
		entry.form.Set("user_id", f.platform)
		entry.form.Set("is_platform_admin", "true")
		resp, _ = f.send(t, c, "POST", entry.path, entry.form, nil)
		status(t, resp, 403)
	}
	resp, _ = f.send(t, c, "GET", "/app/settings", nil, nil)
	status(t, resp, 403)
	resp, _ = f.send(t, c, "GET", "/platform", nil, nil)
	status(t, resp, 403)
	csrf = f.selectTenant(t, c, f.south)
	resp, body = f.send(t, c, "GET", "/app", nil, nil)
	status(t, resp, 200)
	if strings.Contains(body, "href=\"/app/members\"") {
		t.Fatal("viewer sees member navigation")
	}
	resp, _ = f.send(t, c, "GET", "/app/members", nil, nil)
	status(t, resp, 403)
	resp, _ = f.send(t, c, "POST", "/app/members/"+f.southMember, url.Values{"csrf_token": {csrf}, "role": {"admin"}, "active": {"true"}}, nil)
	status(t, resp, 403)
	resp, _ = f.send(t, c, "GET", "/app/settings", nil, nil)
	status(t, resp, 403)
	// A forged role in internal context also cannot override a current Viewer membership.
	err := rbac.Require(t.Context(), f.db, tenancy.ActorContext{UserID: f.user, TenantID: f.south, Role: "owner"}, rbac.MembersManage)
	if !errors.Is(err, rbac.ErrForbidden) {
		t.Fatalf("stale context authorized: %v", err)
	}
}
func TestOwnerMembersMutationsValidationCSRFAndRevocation(t *testing.T) {
	f := newAuthFixture(t, false)
	owner, user, ownerMember, csrf := f.owner(t, "north", f.north)
	resp, body := f.send(t, owner, "GET", "/app/members", nil, map[string]string{"HX-Request": "true"})
	status(t, resp, 200)
	if strings.Contains(body, "<!DOCTYPE") || !strings.Contains(body, "Agregar miembro existente") {
		t.Fatal("expected permitted HTMX fragment")
	}
	target := f.newUser(t, "target@stage4.test")
	targetClient := f.client()
	oldToken := f.login(t, targetClient, "target@stage4.test")
	add := url.Values{"email": {" TARGET@STAGE4.TEST "}, "role": {"viewer"}}
	resp, _ = f.send(t, owner, "POST", "/app/members", add, nil)
	status(t, resp, 403)
	add.Set("csrf_token", csrf)
	resp, _ = f.send(t, owner, "POST", "/app/members", add, map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://attacker.test"})
	status(t, resp, 403)
	add.Set("role", "owner")
	resp, _ = f.send(t, owner, "POST", "/app/members", add, nil)
	status(t, resp, 422)
	add.Set("role", "viewer")
	add.Set("tenant_id", f.south)
	add.Set("user_id", f.platform)
	resp, _ = f.send(t, owner, "POST", "/app/members", add, map[string]string{"HX-Request": "true"})
	status(t, resp, 200)
	if resp.Header.Get("HX-Redirect") != "/app/members" {
		t.Fatal("missing HX redirect")
	}
	if _, err := f.service.Authenticate(t.Context(), oldToken); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("membership grant retained old session")
	}
	var member string
	if err := f.db.QueryRowContext(t.Context(), "SELECT id FROM tenant_memberships WHERE tenant_id=? AND user_id=?", f.north, target).Scan(&member); err != nil {
		t.Fatal(err)
	}
	// Unknown/inactive/already-member accounts have indistinguishable errors.
	var failure string
	f.newUser(t, "exact@stage4.test")
	inactive := f.newUser(t, "inactive@stage4.test")
	if _, err := f.db.ExecContext(t.Context(), "UPDATE users SET is_active=FALSE WHERE id=?", inactive); err != nil {
		t.Fatal(err)
	}
	for _, email := range []string{"target@stage4.test", "missing@stage4.test", "inactive@stage4.test", "éxact@stage4.test"} {
		add.Set("email", email)
		resp, body = f.send(t, owner, "POST", "/app/members", add, nil)
		status(t, resp, 422)
		if failure != "" && failure != body {
			t.Fatal("account lookup disclosed reason")
		}
		failure = body
	}
	oldToken = f.login(t, targetClient, "target@stage4.test")
	f.selectTenant(t, targetClient, f.north)
	oldToken = f.cookieValue(t, targetClient)
	update := url.Values{"csrf_token": {csrf}, "role": {"admin"}, "active": {"true"}, "tenant_id": {f.south}, "user_id": {f.platform}}
	resp, _ = f.send(t, owner, "POST", "/app/members/"+member, update, nil)
	status(t, resp, 303)
	if _, err := f.service.Authenticate(t.Context(), oldToken); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("role elevation retained old token")
	}
	row, err := dbgen.New(f.db).GetMembership(t.Context(), dbgen.GetMembershipParams{TenantID: f.north, ID: member})
	if err != nil || row.Role != "admin" {
		t.Fatalf("wrong persisted role: %v", err)
	}
	resp, foreignBody := f.send(t, owner, "POST", "/app/members/"+f.southMember, update, nil)
	status(t, resp, 404)
	resp, missingBody := f.send(t, owner, "POST", "/app/members/"+id.New(), update, nil)
	status(t, resp, 404)
	if foreignBody != missingBody {
		t.Fatal("foreign resource distinguishable")
	}
	for _, state := range []struct{ role, active string }{{"admin", "true"}, {"owner", "false"}} {
		update.Set("role", state.role)
		update.Set("active", state.active)
		resp, _ = f.send(t, owner, "POST", "/app/members/"+ownerMember, update, nil)
		status(t, resp, 409)
	}
	update.Set("role", "owner")
	update.Set("active", "true")
	token := f.cookieValue(t, owner)
	resp, _ = f.send(t, owner, "POST", "/app/members/"+ownerMember, update, nil)
	status(t, resp, 303)
	if _, err = f.service.Authenticate(t.Context(), token); err != nil {
		t.Fatal("no-op revoked session")
	}
	update.Set("role", "viewer")
	update.Set("active", "false")
	resp, _ = f.send(t, owner, "POST", "/app/members/"+member, update, nil)
	status(t, resp, 303)
	resp, _ = f.send(t, owner, "GET", "/app/members?offset=-1", nil, nil)
	status(t, resp, 422)
	resp, _ = f.send(t, owner, "GET", "/app/members?offset=25", nil, nil)
	status(t, resp, 200)
	var count int
	if err = f.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM audit_logs WHERE tenant_id=? AND actor_user_id=? AND action='memberships.update' AND outcome='success'", f.north, user).Scan(&count); err != nil || count != 3 {
		t.Fatalf("success audit count %d: %v", count, err)
	}
	logs := f.logs.String()
	for _, secret := range []string{csrf, token, "target@stage4.test", fixturePassword} {
		if strings.Contains(logs, secret) {
			t.Fatal("logs leaked request data")
		}
	}
}
func TestTenantSettingsAndPlatformIsolation(t *testing.T) {
	f := newAuthFixture(t, false)
	owner, _, _, csrf := f.owner(t, "north", f.north)
	resp, _ := f.send(t, owner, "POST", "/app/settings", url.Values{"csrf_token": {csrf}, "name": {" Norte renombrada "}, "tenant_id": {f.south}, "slug": {"changed"}}, nil)
	status(t, resp, 303)
	north, err := dbgen.New(f.db).GetTenant(t.Context(), f.north)
	if err != nil || north.Name != "Norte renombrada" || north.Slug != "north" {
		t.Fatalf("settings scope: %v", err)
	}
	resp, _ = f.send(t, owner, "POST", "/app/settings", url.Values{"csrf_token": {csrf}, "name": {strings.Repeat("á", 181)}}, nil)
	status(t, resp, 422)
	platform := f.client()
	f.login(t, platform, "platform@auth.test")
	resp, body := f.send(t, platform, "GET", "/app/tenants/select", nil, nil)
	status(t, resp, 200)
	pcsrf := csrfFrom(t, body)
	if !strings.Contains(body, "href=\"/platform\"") {
		t.Fatal("platform entry missing")
	}
	resp, _ = f.send(t, platform, "GET", "/app/members", nil, nil)
	status(t, resp, 303)
	resp, body = f.send(t, platform, "GET", "/platform", nil, map[string]string{"HX-Request": "true"})
	status(t, resp, 200)
	if strings.Contains(body, "<!DOCTYPE") || !strings.Contains(body, "Métricas técnicas") {
		t.Fatal("wrong platform fragment")
	}
	target := f.newUser(t, "bootstrap@stage4.test")
	targetClient := f.client()
	oldToken := f.login(t, targetClient, "bootstrap@stage4.test")
	form := url.Values{"csrf_token": {pcsrf}, "name": {"Nueva empresa"}, "slug": {"new-company"}, "owner_email": {"bootstrap@stage4.test"}}
	resp, _ = f.send(t, platform, "POST", "/platform/tenants", form, nil)
	status(t, resp, 303)
	created, err := dbgen.New(f.db).GetTenantBySlug(t.Context(), "new-company")
	if err != nil {
		t.Fatal(err)
	}
	var owns int
	if err = f.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM tenant_memberships WHERE tenant_id=? AND user_id=? AND role='owner' AND is_active=TRUE", created.ID, target).Scan(&owns); err != nil || owns != 1 {
		t.Fatal("tenant bootstrap did not create owner")
	}
	if _, err = f.service.Authenticate(t.Context(), oldToken); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("bootstrap retained owner's old session")
	}
	rows, err := f.service.Tenants(t.Context(), mustPrincipal(t, f, platform))
	if err != nil || len(rows) != 0 {
		t.Fatal("operator acquired commercial membership")
	}
	resp, _ = f.send(t, platform, "POST", "/platform/tenants", form, nil)
	status(t, resp, 422)
	// Deactivation closes selected sessions, preserves data and supports reactivation.
	update := url.Values{"csrf_token": {pcsrf}, "name": {"Disabled North"}, "active": {"false"}, "slug": {"ignored"}}
	ownerToken := f.cookieValue(t, owner)
	resp, _ = f.send(t, platform, "POST", "/platform/tenants/"+f.north, update, nil)
	status(t, resp, 303)
	if _, err = f.service.Authenticate(t.Context(), ownerToken); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("inactive tenant retained selected session")
	}
	update.Set("active", "true")
	resp, _ = f.send(t, platform, "POST", "/platform/tenants/"+f.north, update, nil)
	status(t, resp, 303)
	north, err = dbgen.New(f.db).GetTenant(t.Context(), f.north)
	if err != nil || !north.IsActive || north.Slug != "north" {
		t.Fatal("reactivation/immutable slug failed")
	}
	resp, _ = f.send(t, platform, "POST", "/platform/tenants/"+id.New(), update, nil)
	status(t, resp, 404)
	if _, err = f.db.ExecContext(t.Context(), "UPDATE users SET is_platform_admin=FALSE WHERE id=?", f.platform); err != nil {
		t.Fatal(err)
	}
	resp, _ = f.send(t, platform, "GET", "/platform", nil, nil)
	status(t, resp, 403)
	resp, _ = f.send(t, platform, "POST", "/platform/tenants", form, nil)
	status(t, resp, 403)
	var count int
	if err = f.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM audit_logs WHERE action='platform.tenants.create' AND outcome='success' AND tenant_id IS NULL AND actor_user_id=?", f.platform).Scan(&count); err != nil || count != 1 {
		t.Fatalf("global audit: %d %v", count, err)
	}
}
func mustPrincipal(t *testing.T, f *authFixture, c *http.Client) auth.Principal {
	t.Helper()
	p, err := f.service.Authenticate(t.Context(), f.cookieValue(t, c))
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func TestMembershipServiceConcurrentOwnersAndStaleRole(t *testing.T) {
	f := newAuthFixture(t, false)
	_, owner, ownerMember, _ := f.owner(t, "north", f.north)
	other := f.newUser(t, "second-owner@stage4.test")
	otherMember, err := memberrepo.New(f.db).Add(t.Context(), f.north, other, "owner")
	if err != nil {
		t.Fatal(err)
	}
	service := members.New(f.db)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, pair := range []struct{ user, member string }{{owner, ownerMember}, {other, otherMember}} {
		wg.Add(1)
		go func(user, member string) {
			defer wg.Done()
			<-start
			_, err := service.Change(t.Context(), tenancy.ActorContext{UserID: user, TenantID: f.north, Role: "owner"}, member, "admin", true, "concurrency")
			results <- err
		}(pair.user, pair.member)
	}
	close(start)
	wg.Wait()
	close(results)
	ok, blocked := 0, 0
	for err := range results {
		if err == nil {
			ok++
		} else if errors.Is(err, memberrepo.ErrLastOwner) {
			blocked++
		} else {
			t.Fatal(err)
		}
	}
	if ok != 1 || blocked != 1 {
		t.Fatalf("concurrent owners: success %d blocked %d", ok, blocked)
	}
	var remaining int
	if err = f.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM tenant_memberships WHERE tenant_id=? AND role='owner' AND is_active=TRUE", f.north).Scan(&remaining); err != nil || remaining != 1 {
		t.Fatal("lost last owner")
	}
	// No stale role accepted after a concurrent demotion has committed.
	row, err := dbgen.New(f.db).GetMembership(t.Context(), dbgen.GetMembershipParams{TenantID: f.north, ID: ownerMember})
	if err != nil {
		t.Fatal(err)
	}
	demoted := owner
	if row.Role == "owner" {
		demoted = other
	}
	err = service.Add(t.Context(), tenancy.ActorContext{UserID: demoted, TenantID: f.north, Role: "owner"}, "platform@auth.test", "viewer", "stale")
	if !errors.Is(err, rbac.ErrForbidden) {
		t.Fatalf("demoted actor authorized: %v", err)
	}
}
func TestAdministrativeAuditFailureRollsBackMutationAndRevocation(t *testing.T) {
	f := newAuthFixture(t, false)
	_, owner, _, _ := f.owner(t, "north", f.north)
	c := f.client()
	token := f.login(t, c, "member@auth.test")
	var member string
	if err := f.db.QueryRowContext(t.Context(), "SELECT id FROM tenant_memberships WHERE tenant_id=? AND user_id=?", f.north, f.user).Scan(&member); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecContext(t.Context(), "RENAME TABLE audit_logs TO unavailable_audit"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := f.db.Exec("RENAME TABLE unavailable_audit TO audit_logs"); err != nil {
			t.Error(err)
		}
	}()
	actor := tenancy.ActorContext{UserID: owner, TenantID: f.north, Role: "owner"}
	_, err := members.New(f.db).Change(t.Context(), actor, member, "viewer", true, "rollback")
	if err == nil {
		t.Fatal("mutation committed without audit")
	}
	row, err := dbgen.New(f.db).GetMembership(t.Context(), dbgen.GetMembershipParams{TenantID: f.north, ID: member})
	if err != nil || row.Role != "admin" {
		t.Fatal("role escaped rollback")
	}
	if _, err = f.service.Authenticate(t.Context(), token); err != nil {
		t.Fatal("revocation escaped rollback")
	}
	err = tenants.New(f.db).Create(t.Context(), f.platform, "Rolled back", "rollback-company", "member@auth.test", "rollback")
	if err == nil {
		t.Fatal("platform creation committed without audit")
	}
	_, err = dbgen.New(f.db).GetTenantBySlug(t.Context(), "rollback-company")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("tenant escaped rollback")
	}
	if _, err = f.service.Authenticate(t.Context(), token); err != nil {
		t.Fatal("platform revocation escaped rollback")
	}
}

func TestEveryAdministrativeMutationRequiresCSRF(t *testing.T) {
	f := newAuthFixture(t, false)
	owner, _, ownerMember, _ := f.owner(t, "north", f.north)
	platform := f.client()
	f.login(t, platform, "platform@auth.test")
	for _, entry := range []struct {
		client *http.Client
		path   string
		fields url.Values
	}{
		{owner, "/app/members", url.Values{"email": {"platform@auth.test"}, "role": {"viewer"}}},
		{owner, "/app/members/" + ownerMember, url.Values{"role": {"admin"}, "active": {"true"}}},
		{owner, "/app/settings", url.Values{"name": {"forged"}}},
		{platform, "/platform/tenants", url.Values{"name": {"forged"}, "slug": {"forged"}, "owner_email": {"member@auth.test"}}},
		{platform, "/platform/tenants/" + f.north, url.Values{"name": {"forged"}, "active": {"false"}}},
	} {
		resp, body := f.send(t, entry.client, "POST", entry.path, entry.fields, map[string]string{"HX-Request": "true"})
		status(t, resp, 403)
		if strings.Contains(body, "<!DOCTYPE") {
			t.Fatal("expected CSRF fragment")
		}
	}
}
func TestMemberListPaginationAndInactiveAccess(t *testing.T) {
	f := newAuthFixture(t, false)
	owner, _, _, _ := f.owner(t, "north", f.north)
	q := dbgen.New(f.db)
	for n := 0; n < 26; n++ {
		email := id.New() + "@page.test"
		user := f.newUser(t, email)
		if err := q.CreateMembership(t.Context(), dbgen.CreateMembershipParams{ID: id.New(), TenantID: f.north, UserID: user, Role: "viewer", IsActive: true}); err != nil {
			t.Fatal(err)
		}
	}
	resp, first := f.send(t, owner, "GET", "/app/members", nil, nil)
	status(t, resp, 200)
	resp, second := f.send(t, owner, "GET", "/app/members?offset=25", nil, nil)
	status(t, resp, 200)
	if strings.Count(first, "<article ") != 25 || strings.Count(second, "<article ") != 3 || !strings.Contains(first, "Siguiente") || !strings.Contains(second, "Anterior") {
		t.Fatal("pagination did not bound member rows")
	}
	// A role held in another tenant cannot authorize this company's settings or member list.
	c := f.client()
	f.login(t, c, "member@auth.test")
	csrf := f.selectTenant(t, c, f.north)
	if _, err := f.db.ExecContext(t.Context(), "UPDATE tenant_memberships SET is_active=FALSE WHERE tenant_id=? AND user_id=?", f.north, f.user); err != nil {
		t.Fatal(err)
	}
	resp, _ = f.send(t, c, "GET", "/app/members", nil, nil)
	status(t, resp, 303)
	resp, _ = f.send(t, c, "POST", "/app/settings", url.Values{"csrf_token": {csrf}, "name": {"not applied"}}, nil)
	status(t, resp, 303)
}
