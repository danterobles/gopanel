//go:build integration

package integration_test

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	dbgen "gopanel/internal/db/generated"
	"gopanel/internal/modules/customers"
	repo "gopanel/internal/modules/customers/repository"
	"gopanel/internal/platform/id"
	"gopanel/internal/platform/rbac"
	"gopanel/internal/platform/tenancy"
)

func customerFields(csrf string) url.Values {
	return url.Values{"csrf_token": {csrf}, "name": {" Cliente <script>alert(1)</script> "}, "email": {" CLIENTE@EXAMPLE.TEST "}, "phone": {" "}, "status": {"active"}, "notes": {" línea uno\r\nlínea dos "}}
}
func customerCount(t *testing.T, db *sql.DB, tenant string) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM customers WHERE tenant_id=? AND deleted_at IS NULL", tenant).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func createCustomer(t *testing.T, f *authFixture, tenant, name, state string) string {
	t.Helper()
	value, err := repo.New(f.db).Create(t.Context(), tenant, repo.Input{Name: name, Status: state})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func TestCustomerHTTPCRUDHTMLHTMXValidationAndSoftDelete(t *testing.T) {
	f := newAuthFixture(t, false)
	c := f.client()
	f.login(t, c, "member@auth.test")
	csrf := f.selectTenant(t, c, f.north)
	resp, body := f.send(t, c, "GET", "/app/customers/new", nil, map[string]string{"HX-Request": "true"})
	status(t, resp, 200)
	if strings.Contains(strings.ToLower(body), "<!doctype") || !strings.Contains(body, "customer-name") {
		t.Fatal("new form must be fragment")
	}
	fields := customerFields(csrf)
	fields.Set("email", "not-an-email")
	resp, body = f.send(t, c, "POST", "/app/customers/new", fields, map[string]string{"HX-Request": "true"})
	status(t, resp, 422)
	if !strings.Contains(body, "customer-email-error") || !strings.Contains(body, "aria-invalid=\"true\"") || !strings.Contains(body, "&lt;script&gt;") || strings.Contains(body, "<script>alert(1)</script>") || customerCount(t, f.db, f.north) != 0 {
		t.Fatal("invalid form wrote data, lost values or allowed HTML")
	}
	fields = customerFields(csrf)
	fields.Set("tenant_id", f.south)
	fields.Set("user_id", f.platform)
	fields.Set("role", "owner")
	fields.Set("id", id.New())
	resp, _ = f.send(t, c, "POST", "/app/customers/new", fields, nil)
	status(t, resp, 303)
	location := resp.Header.Get("Location")
	parsed, _ := url.Parse(location)
	resource := strings.TrimPrefix(parsed.Path, "/app/customers/")
	row, err := repo.New(f.db).Get(t.Context(), f.north, resource)
	if err != nil || row.Email.String != "cliente@example.test" || row.Phone.Valid || row.Notes.String != "línea uno\nlínea dos" || customerCount(t, f.db, f.south) != 0 {
		t.Fatalf("wrong saved customer: %v", err)
	}
	resp, body = f.send(t, c, "GET", location, nil, nil)
	status(t, resp, 200)
	if !strings.Contains(strings.ToLower(body), "<!doctype") || !strings.Contains(body, "Cliente creado.") || strings.Contains(body, "<script>alert(1)</script>") {
		t.Fatal("detail missing notification or unsafe HTML")
	}
	resp, body = f.send(t, c, "GET", parsed.Path+"/edit", nil, nil)
	status(t, resp, 200)
	if !strings.Contains(body, "cliente@example.test") {
		t.Fatal("edit omitted values")
	}
	// Saving unchanged data succeeds and remains an audited action.
	resp, _ = f.send(t, c, "POST", parsed.Path+"/edit", fields, nil)
	status(t, resp, 303)
	fields.Set("name", "Cliente actualizado")
	fields.Set("email", "")
	fields.Set("notes", strings.Repeat("😀", 5000))
	resp, body = f.send(t, c, "POST", parsed.Path+"/edit", fields, map[string]string{"HX-Request": "true"})
	status(t, resp, 200)
	if resp.Header.Get("HX-Push-Url") != parsed.Path || strings.Contains(strings.ToLower(body), "<!doctype") || !strings.Contains(body, "Cliente actualizado.") {
		t.Fatal("update did not return current detail fragment")
	}
	row, err = repo.New(f.db).Get(t.Context(), f.north, resource)
	if err != nil || row.Email.Valid || len([]rune(row.Notes.String)) != 5000 {
		t.Fatal("Unicode notes or NULL email not persisted")
	}
	fields.Set("name", "Segundo cliente")
	fields.Set("notes", "")
	resp, body = f.send(t, c, "POST", "/app/customers/new", fields, map[string]string{"HX-Request": "true"})
	status(t, resp, 201)
	secondPath := resp.Header.Get("HX-Push-Url")
	if secondPath == "" || !strings.Contains(body, "Segundo cliente") {
		t.Fatal("HTMX create did not return saved detail")
	}
	resp, body = f.send(t, c, "GET", parsed.Path+"/delete", nil, nil)
	status(t, resp, 200)
	if !strings.Contains(body, "Confirmar baja") || !strings.Contains(body, "name=\"confirm\" value=\"yes\"") {
		t.Fatal("missing no-JS delete confirmation")
	}
	resp, _ = f.send(t, c, "POST", parsed.Path+"/delete", url.Values{"csrf_token": {csrf}}, nil)
	status(t, resp, 422)
	if customerCount(t, f.db, f.north) != 2 {
		t.Fatal("deleted without confirmation")
	}
	resp, body = f.send(t, c, "POST", parsed.Path+"/delete", url.Values{"csrf_token": {csrf}, "confirm": {"yes"}}, map[string]string{"HX-Request": "true"})
	status(t, resp, 200)
	if resp.Header.Get("HX-Push-Url") != "/app/customers" || !strings.Contains(body, "Cliente dado de baja.") || !strings.Contains(body, "Segundo cliente") {
		t.Fatal("delete did not refresh list")
	}
	for _, suffix := range []string{"", "/edit", "/delete"} {
		resp, _ = f.send(t, c, "GET", parsed.Path+suffix, nil, nil)
		status(t, resp, 404)
	}
	resp, _ = f.send(t, c, "POST", parsed.Path+"/delete", url.Values{"csrf_token": {csrf}, "confirm": {"yes"}}, nil)
	status(t, resp, 404)
	var deleted sql.NullTime
	if err = f.db.QueryRowContext(t.Context(), "SELECT deleted_at FROM customers WHERE tenant_id=? AND id=?", f.north, resource).Scan(&deleted); err != nil || !deleted.Valid {
		t.Fatal("expected soft delete")
	}
	resp, _ = f.send(t, c, "POST", secondPath+"/delete", url.Values{"csrf_token": {csrf}, "confirm": {"yes"}}, nil)
	status(t, resp, 303)
	if resp.Header.Get("Location") != "/app/customers?notice=deleted" || customerCount(t, f.db, f.north) != 0 {
		t.Fatal("normal delete did not redirect or persisted a live record")
	}
	resp, body = f.send(t, c, "GET", resp.Header.Get("Location"), nil, nil)
	status(t, resp, 200)
	if !strings.Contains(body, "No hay clientes para mostrar") {
		t.Fatal("normal delete did not return empty list")
	}
	var audits int
	if err = f.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM audit_logs WHERE tenant_id=? AND actor_user_id=? AND entity_type='customer' AND outcome='success'", f.north, f.user).Scan(&audits); err != nil || audits != 6 {
		t.Fatalf("audit mismatch %d: %v", audits, err)
	}
	for _, secret := range []string{csrf, fields.Get("name"), "cliente@example.test"} {
		if strings.Contains(f.logs.String(), secret) {
			t.Fatal("customer input leaked into logs")
		}
	}
}
func TestCustomerHTTPRolesIDORAndCSRF(t *testing.T) {
	f := newAuthFixture(t, false)
	north := createCustomer(t, f, f.north, "PRIVATE NORTH", "active")
	south := createCustomer(t, f, f.south, "PRIVATE SOUTH", "active")
	c := f.client()
	f.login(t, c, "member@auth.test")
	csrf := f.selectTenant(t, c, f.north)
	for _, suffix := range []string{"", "/edit", "/delete"} {
		resp, foreign := f.send(t, c, "GET", "/app/customers/"+south+suffix, nil, nil)
		status(t, resp, 404)
		resp, missing := f.send(t, c, "GET", "/app/customers/"+id.New()+suffix, nil, nil)
		status(t, resp, 404)
		if foreign != missing || strings.Contains(foreign, "PRIVATE SOUTH") {
			t.Fatal("foreign record distinguishable")
		}
	}
	for _, malformed := range []string{"not-a-uuid", "%F0%9F%98%80", strings.Repeat("a", 200)} {
		resp, _ := f.send(t, c, "GET", "/app/customers/"+malformed, nil, nil)
		status(t, resp, 404)
	}
	fields := customerFields(csrf)
	resp, _ := f.send(t, c, "POST", "/app/customers/"+south+"/edit", fields, nil)
	status(t, resp, 404)
	resp, _ = f.send(t, c, "POST", "/app/customers/"+south+"/delete", url.Values{"csrf_token": {csrf}, "confirm": {"yes"}}, nil)
	status(t, resp, 404)
	for _, path := range []string{"/app/customers/new", "/app/customers/" + north + "/edit", "/app/customers/" + north + "/delete"} {
		resp, _ = f.send(t, c, "POST", path, url.Values{"name": {"forged"}, "status": {"active"}, "confirm": {"yes"}}, map[string]string{"HX-Request": "true"})
		status(t, resp, 403)
		resp, _ = f.send(t, c, "POST", path, fields, map[string]string{"Origin": "https://attacker.test", "Sec-Fetch-Site": "cross-site"})
		status(t, resp, 403)
	}
	resp, _ = f.send(t, c, "POST", "/app/customers/new", url.Values{"csrf_token": {csrf}, "name": {strings.Repeat("x", 70<<10)}, "status": {"active"}}, nil)
	status(t, resp, 413)
	// Same account is only Viewer in South; headers/forms never elevate it.
	csrf = f.selectTenant(t, c, f.south)
	fields = customerFields(csrf)
	fields.Set("tenant_id", f.north)
	fields.Set("role", "owner")
	fields.Set("user_id", f.platform)
	resp, body := f.send(t, c, "GET", "/app/customers", nil, nil)
	status(t, resp, 200)
	if strings.Contains(body, "PRIVATE NORTH") || strings.Contains(body, "Nuevo cliente") {
		t.Fatal("viewer list or controls leaked")
	}
	resp, body = f.send(t, c, "GET", "/app/customers/"+south, nil, nil)
	status(t, resp, 200)
	if strings.Contains(body, "Editar cliente") || strings.Contains(body, "Dar de baja") {
		t.Fatal("viewer mutation controls visible")
	}
	for _, path := range []string{"/app/customers/new", "/app/customers/" + south + "/edit", "/app/customers/" + south + "/delete"} {
		resp, _ = f.send(t, c, "GET", path, nil, map[string]string{"HX-Request": "true"})
		status(t, resp, 403)
		resp, _ = f.send(t, c, "POST", path, fields, map[string]string{"HX-Request": "true"})
		status(t, resp, 403)
	}
	resp, _ = f.send(t, c, "GET", "/app/customers/"+north, nil, nil)
	status(t, resp, 404)
	if customerCount(t, f.db, f.north) != 1 || customerCount(t, f.db, f.south) != 1 {
		t.Fatal("forbidden requests modified data")
	}
}
func TestCustomerFiltersSortPaginationAndHTMXCanonicalURL(t *testing.T) {
	f := newAuthFixture(t, false)
	c := f.client()
	f.login(t, c, "member@auth.test")
	f.selectTenant(t, c, f.north)
	for n := 0; n < 27; n++ {
		state := "active"
		if n%2 == 1 {
			state = "inactive"
		}
		value := createCustomer(t, f, f.north, fmt.Sprintf("Sort %02d", n), state)
		if _, err := f.db.ExecContext(t.Context(), "UPDATE customers SET created_at=? WHERE tenant_id=? AND id=?", time.Date(2026, 1, 1, 0, 0, 26-n, 0, time.UTC), f.north, value); err != nil {
			t.Fatal(err)
		}
	}
	createCustomer(t, f, f.south, "Sort foreign secret", "active")
	actor := tenancy.ActorContext{UserID: f.user, TenantID: f.north, Role: "owner"}
	for _, entry := range []struct{ sort, first string }{{"name_asc", "Sort 00"}, {"name_desc", "Sort 26"}, {"created_asc", "Sort 26"}, {"created_desc", "Sort 00"}} {
		rows, total, err := customers.New(f.db).List(t.Context(), actor, repo.ListOptions{Search: "Sort ", Sort: entry.sort, Page: 1, PageSize: 10})
		if err != nil || total != 27 || len(rows) != 10 || rows[0].Name != entry.first {
			t.Fatalf("sort %s incorrect: %v", entry.sort, err)
		}
	}
	query := url.Values{"search": {"Sort "}, "status": {"inactive"}, "sort": {"name_asc"}, "page_size": {"10"}, "page": {"1"}}
	resp, body := f.send(t, c, "GET", "/app/customers/table?"+query.Encode(), nil, map[string]string{"HX-Request": "true"})
	status(t, resp, 200)
	if strings.Contains(strings.ToLower(body), "<!doctype") || strings.Contains(body, "customer-search") || strings.Contains(body, "Sort foreign") || !strings.Contains(body, "13 clientes") || !strings.Contains(body, "Siguiente") {
		t.Fatal("incorrect filtered table fragment")
	}
	canonical, err := url.Parse(resp.Header.Get("HX-Push-Url"))
	if err != nil || canonical.Path != "/app/customers" || canonical.Query().Get("status") != "inactive" || canonical.Query().Get("search") != "Sort" {
		t.Fatal("canonical filter URL lost state")
	}
	query.Set("page", "2")
	resp, body = f.send(t, c, "GET", "/app/customers/table?"+query.Encode(), nil, nil)
	status(t, resp, 200)
	if !strings.Contains(strings.ToLower(body), "<!doctype") || !strings.Contains(body, "Sort 21") || strings.Contains(body, ">Sort 19<") {
		t.Fatal("second page or full table navigation wrong")
	}
	createCustomer(t, f, f.north, `Literal %_\=`, "active")
	createCustomer(t, f, f.north, "Literal anything", "active")
	query = url.Values{"search": {`%_\=`}}
	resp, body = f.send(t, c, "GET", "/app/customers?"+query.Encode(), nil, nil)
	status(t, resp, 200)
	if !strings.Contains(body, "1 cliente") || !strings.Contains(body, `Literal %_\=`) || strings.Contains(body, "Literal anything") {
		t.Fatal("wildcards not treated literally")
	}
	for _, bad := range []string{"sort=name%3BDROP", "status=deleted", "page=0", "page=99999999999999999999", "page_size=100", "page=10001&page_size=50"} {
		resp, _ = f.send(t, c, "GET", "/app/customers?"+bad, nil, map[string]string{"HX-Request": "true"})
		status(t, resp, 422)
	}
	resp, body = f.send(t, c, "GET", "/app/customers?search=no-match", nil, nil)
	status(t, resp, 200)
	if !strings.Contains(body, "No hay clientes para mostrar") {
		t.Fatal("missing empty state")
	}
	for _, size := range []string{"10", "25", "50"} {
		resp, _ = f.send(t, c, "GET", "/app/customers?page_size="+size, nil, nil)
		status(t, resp, 200)
	}
}
func TestCustomerAuditRollbackAndStalePermission(t *testing.T) {
	f := newAuthFixture(t, false)
	resource := createCustomer(t, f, f.north, "Original", "active")
	service := customers.New(f.db)
	actor := tenancy.ActorContext{UserID: f.user, TenantID: f.north, Role: "owner"}
	if _, err := f.db.ExecContext(t.Context(), "RENAME TABLE audit_logs TO unavailable_customer_audit"); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"", resource} {
		if _, _, err := service.Save(t.Context(), actor, target, customers.Input{Name: "Rollback", Status: "active"}, "audit-failure"); err == nil {
			t.Fatal("saved without audit")
		}
	}
	if err := service.Delete(t.Context(), actor, resource, "yes", "audit-failure"); err == nil {
		t.Fatal("deleted without audit")
	}
	if _, err := f.db.ExecContext(t.Context(), "RENAME TABLE unavailable_customer_audit TO audit_logs"); err != nil {
		t.Fatal(err)
	}
	row, err := repo.New(f.db).Get(t.Context(), f.north, resource)
	if err != nil || row.Name != "Original" || customerCount(t, f.db, f.north) != 1 {
		t.Fatal("mutation escaped failed audit transaction")
	}
	tx, err := f.db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = dbgen.New(tx).LockTenant(t.Context(), f.north); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.ExecContext(t.Context(), "UPDATE tenant_memberships SET role='viewer' WHERE tenant_id=? AND user_id=?", f.north, f.user); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		close(started)
		_, _, err := service.Save(t.Context(), actor, resource, customers.Input{Name: "Forbidden", Status: "active"}, "stale-role")
		result <- err
	}()
	<-started
	select {
	case err := <-result:
		t.Fatalf("mutation did not wait for tenant lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-result:
		if !errors.Is(err, rbac.ErrForbidden) {
			t.Fatalf("stale permission accepted: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("mutation stayed blocked")
	}
	row, err = repo.New(f.db).Get(t.Context(), f.north, resource)
	if err != nil || row.Name != "Original" {
		t.Fatal("demoted actor modified customer")
	}
}

func TestCustomerOwnerAndPlatformScopes(t *testing.T) {
	f := newAuthFixture(t, false)
	owner, _, _, csrf := f.owner(t, "north", f.north)
	resp, _ := f.send(t, owner, "GET", "/app/customers/new", nil, nil)
	status(t, resp, 200)
	resp, _ = f.send(t, owner, "POST", "/app/customers/new", customerFields(csrf), nil)
	status(t, resp, 303)
	platform := f.client()
	f.login(t, platform, "platform@auth.test")
	resp, _ = f.send(t, platform, "GET", "/app/customers", nil, nil)
	status(t, resp, 303)
	resp, body := f.send(t, platform, "GET", "/app/tenants/select", nil, nil)
	status(t, resp, 200)
	resp, _ = f.send(t, platform, "POST", "/app/tenants/activate", url.Values{"csrf_token": {csrfFrom(t, body)}, "tenant_id": {f.north}}, nil)
	status(t, resp, 403)
	if customerCount(t, f.db, f.north) != 1 {
		t.Fatal("platform acquired business access")
	}
}
