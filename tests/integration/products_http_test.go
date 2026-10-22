//go:build integration

package integration_test

import (
	"errors"
	"fmt"
	dbgen "gopanel/internal/db/generated"
	"gopanel/internal/modules/products"
	repo "gopanel/internal/modules/products/repository"
	"gopanel/internal/platform/id"
	"gopanel/internal/platform/rbac"
	"gopanel/internal/platform/tenancy"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func productFields(csrf, sku string) url.Values {
	return url.Values{"csrf_token": {csrf}, "sku": {sku}, "name": {" Producto <script>alert(1)</script> "}, "description": {" uno\r\ndos "}, "price": {"1250.50"}, "currency": {"MXN"}, "status": {"active"}}
}
func productCount(t *testing.T, f *authFixture, tenant string) int {
	t.Helper()
	var n int
	if e := f.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM products WHERE tenant_id=? AND deleted_at IS NULL", tenant).Scan(&n); e != nil {
		t.Fatal(e)
	}
	return n
}
func createProduct(t *testing.T, f *authFixture, tenant, sku, name, state string, price int64) string {
	t.Helper()
	v, e := repo.New(f.db).Create(t.Context(), tenant, repo.Input{Sku: sku, Name: name, Status: state, PriceCents: price, Currency: "MXN"})
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func TestProductHTTPCRUDCollationPricesAndReuse(t *testing.T) {
	f := newAuthFixture(t, false)
	c := f.client()
	f.login(t, c, "member@auth.test")
	csrf := f.selectTenant(t, c, f.north)
	resp, body := f.send(t, c, "GET", "/app/products/new", nil, nil)
	status(t, resp, 200)
	if !strings.Contains(body, "name=\"price\"") {
		t.Fatal("price form missing")
	}
	fields := productFields(csrf, " SKÚ-1 ")
	fields.Set("tenant_id", f.south)
	fields.Set("user_id", f.platform)
	fields.Set("price_cents", "-5")
	resp, body = f.send(t, c, "POST", "/app/products/new", fields, map[string]string{"HX-Request": "true"})
	status(t, resp, 201)
	path := resp.Header.Get("HX-Push-Url")
	resource := strings.TrimPrefix(path, "/app/products/")
	row, e := repo.New(f.db).Get(t.Context(), f.north, resource)
	if e != nil || row.Sku != "SKÚ-1" || row.PriceCents != 125050 || row.Description.String != "uno\ndos" || row.Currency != "MXN" || productCount(t, f, f.south) != 0 {
		t.Fatalf("wrong product: %v", e)
	}
	if strings.Contains(body, "<script>alert") || strings.Contains(strings.ToLower(body), "<!doctype") || !strings.Contains(body, "1250.50 MXN") {
		t.Fatal("unsafe or incorrect detail fragment")
	}
	for _, sku := range []string{"sku-1", "Skú-1", "sku\u0301-1"} {
		fields.Set("sku", sku)
		resp, body = f.send(t, c, "POST", "/app/products/new", fields, nil)
		status(t, resp, 422)
		if !strings.Contains(body, "product-sku-error") || productCount(t, f, f.north) != 1 {
			t.Fatal("collation duplicate not a field error")
		}
	}
	other, _, _, otherCSRF := f.owner(t, "south", f.south)
	fields.Set("csrf_token", otherCSRF)
	resp, _ = f.send(t, other, "POST", "/app/products/new", fields, nil)
	status(t, resp, 303)
	fields.Set("csrf_token", csrf)
	fields.Set("sku", "SKÚ-1")
	resp, _ = f.send(t, c, "POST", path+"/edit", fields, nil)
	status(t, resp, 303) // unchanged is valid
	fields.Set("price", "92233720368547758.07")
	fields.Set("description", strings.Repeat("😀", 5000))
	resp, body = f.send(t, c, "POST", path+"/edit", fields, map[string]string{"HX-Request": "true"})
	status(t, resp, 200)
	if !strings.Contains(body, "92233720368547758.07 MXN") {
		t.Fatal("BIGINT amount rounded")
	}
	for _, entry := range []struct{ field, value string }{{"price", "92233720368547758.08"}, {"price", "1.001"}, {"price", "-1"}, {"currency", "USD"}, {"sku", ""}, {"name", " "}} {
		bad := productFields(csrf, "OTHER")
		bad.Set(entry.field, entry.value)
		resp, body = f.send(t, c, "POST", path+"/edit", bad, map[string]string{"HX-Request": "true"})
		status(t, resp, 422)
		if !strings.Contains(body, "product-"+entry.field+"-error") {
			t.Fatal("field validation missing")
		}
	}
	resp, body = f.send(t, c, "GET", path+"/delete", nil, nil)
	status(t, resp, 200)
	if !strings.Contains(body, "Confirmar baja") {
		t.Fatal("confirmation missing")
	}
	resp, _ = f.send(t, c, "POST", path+"/delete", url.Values{"csrf_token": {csrf}}, nil)
	status(t, resp, 422)
	resp, body = f.send(t, c, "POST", path+"/delete", url.Values{"csrf_token": {csrf}, "confirm": {"yes"}}, map[string]string{"HX-Request": "true"})
	status(t, resp, 200)
	if !strings.Contains(body, "Producto dado de baja.") {
		t.Fatal("delete did not refresh list")
	}
	resp, _ = f.send(t, c, "GET", path, nil, nil)
	status(t, resp, 404)
	fields = productFields(csrf, "sku-1")
	resp, _ = f.send(t, c, "POST", "/app/products/new", fields, nil)
	status(t, resp, 303)
	newPath := strings.Split(resp.Header.Get("Location"), "?")[0]
	resp, _ = f.send(t, c, "POST", newPath+"/delete", url.Values{"csrf_token": {csrf}, "confirm": {"yes"}}, nil)
	status(t, resp, 303)
	var audits int
	if e = f.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM audit_logs WHERE tenant_id=? AND action LIKE 'products.%' AND outcome='success'", f.north).Scan(&audits); e != nil || audits != 6 {
		t.Fatalf("success audits %d %v", audits, e)
	}
	for _, secret := range []string{csrf, fields.Get("name"), "SKÚ-1"} {
		if strings.Contains(f.logs.String(), secret) {
			t.Fatal("product fields leaked into logs")
		}
	}
}
func TestProductPermissionsIDORCSRFAndBodyLimit(t *testing.T) {
	f := newAuthFixture(t, false)
	north := createProduct(t, f, f.north, "NORTH", "North private", "active", 1)
	south := createProduct(t, f, f.south, "SOUTH", "South private", "active", 2)
	c := f.client()
	f.login(t, c, "member@auth.test")
	csrf := f.selectTenant(t, c, f.north)
	for _, suffix := range []string{"", "/edit", "/delete"} {
		resp, foreign := f.send(t, c, "GET", "/app/products/"+south+suffix, nil, nil)
		status(t, resp, 404)
		resp, missing := f.send(t, c, "GET", "/app/products/"+id.New()+suffix, nil, nil)
		status(t, resp, 404)
		if foreign != missing {
			t.Fatal("foreign ID disclosed")
		}
	}
	for _, bad := range []string{"not-uuid", "%F0%9F%98%80", strings.Repeat("x", 100)} {
		resp, _ := f.send(t, c, "GET", "/app/products/"+bad, nil, nil)
		status(t, resp, 404)
	}
	fields := productFields(csrf, "N")
	for _, path := range []string{"/app/products/new", "/app/products/" + north + "/edit", "/app/products/" + north + "/delete"} {
		noCSRF := productFields("", "N")
		resp, _ := f.send(t, c, "POST", path, noCSRF, map[string]string{"HX-Request": "true"})
		status(t, resp, 403)
		resp, _ = f.send(t, c, "POST", path, fields, map[string]string{"Origin": "https://attacker.test", "Sec-Fetch-Site": "cross-site"})
		status(t, resp, 403)
	}
	resp, _ := f.send(t, c, "POST", "/app/products/"+south+"/edit", fields, nil)
	status(t, resp, 404)
	resp, _ = f.send(t, c, "POST", "/app/products/new", url.Values{"csrf_token": {csrf}, "description": {strings.Repeat("x", 70<<10)}}, nil)
	status(t, resp, 413)
	csrf = f.selectTenant(t, c, f.south)
	fields = productFields(csrf, "forged")
	fields.Set("role", "owner")
	fields.Set("tenant_id", f.north)
	resp, body := f.send(t, c, "GET", "/app/products", nil, nil)
	status(t, resp, 200)
	if strings.Contains(body, "North private") || strings.Contains(body, "Nuevo producto") {
		t.Fatal("Viewer scope or controls leaked")
	}
	resp, body = f.send(t, c, "GET", "/app/products/"+south, nil, nil)
	status(t, resp, 200)
	if strings.Contains(body, "Editar producto") || strings.Contains(body, "Dar de baja") {
		t.Fatal("Viewer controls visible")
	}
	for _, path := range []string{"/app/products/new", "/app/products/" + south + "/edit", "/app/products/" + south + "/delete"} {
		resp, _ = f.send(t, c, "GET", path, nil, nil)
		status(t, resp, 403)
		resp, _ = f.send(t, c, "POST", path, fields, map[string]string{"HX-Request": "true"})
		status(t, resp, 403)
	}
	platform := f.client()
	f.login(t, platform, "platform@auth.test")
	resp, _ = f.send(t, platform, "GET", "/app/products", nil, nil)
	status(t, resp, 303)
}
func TestProductSearchOrdersAndPagination(t *testing.T) {
	f := newAuthFixture(t, false)
	c := f.client()
	f.login(t, c, "member@auth.test")
	f.selectTenant(t, c, f.north)
	for n := 0; n < 27; n++ {
		state := "active"
		if n%2 == 1 {
			state = "inactive"
		}
		v := createProduct(t, f, f.north, fmt.Sprintf("ITEM-%02d", n), fmt.Sprintf("Item %02d", n), state, int64(26-n))
		if _, e := f.db.ExecContext(t.Context(), "UPDATE products SET created_at=? WHERE tenant_id=? AND id=?", time.Date(2026, 1, 1, 0, 0, n, 0, time.UTC), f.north, v); e != nil {
			t.Fatal(e)
		}
	}
	createProduct(t, f, f.south, "ITEM-FOREIGN", "Item foreign", "active", 0)
	actor := tenancy.ActorContext{UserID: f.user, TenantID: f.north, Role: "owner"}
	for _, e := range []struct{ sort, first string }{{"name_asc", "Item 00"}, {"name_desc", "Item 26"}, {"price_asc", "Item 26"}, {"price_desc", "Item 00"}, {"created_asc", "Item 00"}, {"created_desc", "Item 26"}} {
		rows, n, err := products.New(f.db).List(t.Context(), actor, repo.ListOptions{Search: "Item", Sort: e.sort, PageSize: 10})
		if err != nil || n != 27 || len(rows) != 10 || rows[0].Name != e.first {
			t.Fatalf("sort %s: %v", e.sort, err)
		}
	}
	q := url.Values{"search": {"ITEM"}, "status": {"inactive"}, "sort": {"price_asc"}, "page": {"2"}, "page_size": {"10"}}
	resp, body := f.send(t, c, "GET", "/app/products/table?"+q.Encode(), nil, map[string]string{"HX-Request": "true"})
	status(t, resp, 200)
	if strings.Contains(strings.ToLower(body), "<!doctype") || strings.Contains(body, "Item foreign") || !strings.Contains(body, "13 productos") || !strings.Contains(body, "Anterior") {
		t.Fatal("wrong table page")
	}
	u, _ := url.Parse(resp.Header.Get("HX-Push-Url"))
	if u.Path != "/app/products" || u.Query().Get("sort") != "price_asc" || u.Query().Get("page") != "2" {
		t.Fatal("URL state lost")
	}
	createProduct(t, f, f.north, `%_\=`, "Literal", "active", 0)
	resp, body = f.send(t, c, "GET", "/app/products?"+url.Values{"search": {`%_\=`}}.Encode(), nil, nil)
	status(t, resp, 200)
	if !strings.Contains(body, "1 producto") || strings.Contains(body, "Item 00") {
		t.Fatal("wildcard search was not literal")
	}
	for _, bad := range []string{"sort=price%3BDROP", "page_size=100", "status=deleted", "page=0"} {
		resp, _ = f.send(t, c, "GET", "/app/products?"+bad, nil, nil)
		status(t, resp, 422)
	}
	for _, size := range []string{"10", "25", "50"} {
		resp, _ = f.send(t, c, "GET", "/app/products?page_size="+size, nil, nil)
		status(t, resp, 200)
	}
	resp, body = f.send(t, c, "GET", "/app/products?search=no-match", nil, nil)
	status(t, resp, 200)
	if !strings.Contains(body, "No hay productos para mostrar") {
		t.Fatal("missing empty state")
	}
}
func TestProductConcurrentSKUAndAuditRollback(t *testing.T) {
	f := newAuthFixture(t, false)
	service := products.New(f.db)
	actor := tenancy.ActorContext{UserID: f.user, TenantID: f.north, Role: "owner"}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, sku := range []string{"SKÚ-race", "sku-RACE"} {
		wg.Add(1)
		go func(sku string) {
			defer wg.Done()
			<-start
			_, _, e := service.Save(t.Context(), actor, "", products.Input{SKU: sku, Name: "Concurrent", Price: "0.01", Currency: "MXN", Status: "active"}, "race")
			results <- e
		}(sku)
	}
	close(start)
	wg.Wait()
	close(results)
	ok, conflict := 0, 0
	for e := range results {
		var invalid *products.ValidationError
		if e == nil {
			ok++
		} else if errors.As(e, &invalid) && invalid.Fields["sku"] != "" {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatal("concurrent SKU invariant failed")
	}
	resource := createProduct(t, f, f.north, "ROLLBACK", "Original", "active", 5)
	if _, e := f.db.ExecContext(t.Context(), "RENAME TABLE audit_logs TO unavailable_product_audit"); e != nil {
		t.Fatal(e)
	}
	for _, target := range []string{"", resource} {
		if _, _, e := service.Save(t.Context(), actor, target, products.Input{SKU: "Changed", Name: "Changed", Price: "999.99", Currency: "MXN", Status: "active"}, "rollback"); e == nil {
			t.Fatal("saved without audit")
		}
	}
	if e := service.Delete(t.Context(), actor, resource, "yes", "rollback"); e == nil {
		t.Fatal("deleted without audit")
	}
	if _, e := f.db.ExecContext(t.Context(), "RENAME TABLE unavailable_product_audit TO audit_logs"); e != nil {
		t.Fatal(e)
	}
	row, e := repo.New(f.db).Get(t.Context(), f.north, resource)
	if e != nil || row.Name != "Original" || row.PriceCents != 5 || productCount(t, f, f.north) != 2 {
		t.Fatal("failed audit escaped rollback")
	}
	tx, e := f.db.BeginTx(t.Context(), nil)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	if _, e = dbgen.New(tx).LockTenant(t.Context(), f.north); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.ExecContext(t.Context(), "UPDATE tenant_memberships SET role='viewer' WHERE tenant_id=? AND user_id=?", f.north, f.user); e != nil {
		t.Fatal(e)
	}
	result := make(chan error, 1)
	go func() {
		_, _, e := service.Save(t.Context(), actor, resource, products.Input{SKU: "Forbidden", Name: "Forbidden", Price: "1", Currency: "MXN", Status: "active"}, "stale")
		result <- e
	}()
	select {
	case e := <-result:
		t.Fatalf("did not wait on tenant lock: %v", e)
	case <-time.After(50 * time.Millisecond):
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-result:
		if !errors.Is(e, rbac.ErrForbidden) {
			t.Fatal("stale role allowed mutation")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("mutation stuck")
	}
}
