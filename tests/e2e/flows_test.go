//go:build e2e

package e2e_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestNativeApplicationJourneyAndRestart(t *testing.T) {
	f := setup(t, 30)
	f.start(t)
	admin := f.client()
	f.login(t, admin, "shared@gopanel.test")
	token := f.selectTenant(t, admin, "empresa-norte")
	streamCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	req, _ := http.NewRequestWithContext(streamCtx, "GET", f.base+"/app/events", nil)
	response, err := admin.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	want(t, response, 200)
	defer response.Body.Close()
	frames := make(chan string, 16)
	go func() {
		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
			if strings.HasPrefix(scanner.Text(), "event: ") {
				frames <- scanner.Text()
			}
		}
	}()
	await := func(want string) {
		t.Helper()
		select {
		case frame := <-frames:
			if frame != "event: "+want {
				t.Fatal(frame)
			}
		case <-time.After(4 * time.Second):
			t.Fatal("event missing", want)
		}
	}
	await("ready")
	form := url.Values{"csrf_token": {token}, "name": {"Cliente E2E"}, "status": {"active"}, "tenant_id": {f.tenants["empresa-sur"]}, "role": {"owner"}}
	resp, body := f.send(t, admin, "POST", "/app/customers/new", form, true)
	want(t, resp, 201)
	customerPath := resp.Header.Get("HX-Push-Url")
	if !strings.Contains(body, "Cliente E2E") {
		t.Fatal("customer not rendered")
	}
	await("invalidate")
	form.Set("name", "Cliente E2E actualizado")
	resp, _ = f.send(t, admin, "POST", customerPath+"/edit", form, true)
	want(t, resp, 200)
	await("invalidate")
	resp, body = f.send(t, admin, "GET", "/app/customers/table?search=E2E&page_size=10&sort=name_asc", nil, true)
	want(t, resp, 200)
	if strings.Contains(strings.ToLower(body), "<!doctype") || !strings.Contains(body, "actualizado") {
		t.Fatal("fragment filter failed")
	}
	product := url.Values{"csrf_token": {token}, "sku": {"E2E-SKU"}, "name": {"Producto E2E"}, "price": {"99.95"}, "currency": {"MXN"}, "status": {"active"}}
	resp, body = f.send(t, admin, "POST", "/app/products/new", product, true)
	want(t, resp, 201)
	productPath := resp.Header.Get("HX-Push-Url")
	if !strings.Contains(body, "99.95 MXN") {
		t.Fatal("price formatting")
	}
	await("invalidate")
	resp, _ = f.send(t, admin, "POST", "/app/products/new", product, true)
	want(t, resp, 422)
	resp, _ = f.send(t, admin, "POST", productPath+"/delete", url.Values{"csrf_token": {token}, "confirm": {"yes"}}, true)
	want(t, resp, 200)
	await("invalidate")
	resp, _ = f.send(t, admin, "POST", customerPath+"/delete", url.Values{"csrf_token": {token}, "confirm": {"yes"}}, true)
	want(t, resp, 200)
	await("invalidate")
	resp, _ = f.send(t, admin, "POST", "/app/customers/new", url.Values{"name": {"forged"}, "status": {"active"}}, true)
	want(t, resp, 403)
	cancel()
	response.Body.Close()
	f.stop(t)
	f.start(t)
	resp, body = f.send(t, admin, "GET", "/app", nil, false)
	want(t, resp, 200)
	if !strings.Contains(body, "Empresa norte") {
		t.Fatal("session did not survive restart")
	}
	token = f.selectTenant(t, admin, "empresa-sur")
	resp, _ = f.send(t, admin, "POST", "/app/customers/new", url.Values{"csrf_token": {token}, "name": {"forged"}, "status": {"active"}, "role": {"owner"}}, true)
	want(t, resp, 403)
	resp, _ = f.send(t, admin, "GET", customerPath, nil, true)
	want(t, resp, 404)
	owner := f.client()
	f.login(t, owner, "owner.norte@gopanel.test")
	f.selectTenant(t, owner, "empresa-norte")
	resp, body = f.send(t, owner, "GET", "/app/members", nil, false)
	want(t, resp, 200)
	if !strings.Contains(body, "Agregar miembro existente") {
		t.Fatal("owner cannot manage members")
	}
	ownerToken := csrf(t, body)
	resp, _ = f.send(t, owner, "POST", "/app/members", url.Values{"csrf_token": {ownerToken}, "email": {"viewer.sur@gopanel.test"}, "role": {"admin"}}, false)
	want(t, resp, 303)
	var membership string
	if err := f.db.QueryRowContext(t.Context(), "SELECT m.id FROM tenant_memberships m JOIN users u ON u.id=m.user_id WHERE m.tenant_id=? AND u.email=?", f.tenants["empresa-norte"], "viewer.sur@gopanel.test").Scan(&membership); err != nil {
		t.Fatal(err)
	}
	resp, _ = f.send(t, owner, "POST", "/app/members/"+membership, url.Values{"csrf_token": {ownerToken}, "role": {"viewer"}, "active": {"true"}}, false)
	want(t, resp, 303)
	changed := f.client()
	f.login(t, changed, "viewer.sur@gopanel.test")
	f.selectTenant(t, changed, "empresa-norte")
	resp, _ = f.send(t, owner, "POST", "/app/members/"+membership, url.Values{"csrf_token": {ownerToken}, "role": {"viewer"}, "active": {"false"}}, false)
	want(t, resp, 303)
	resp, _ = f.send(t, changed, "GET", "/app", nil, false)
	want(t, resp, 303)
	viewer := f.client()
	f.login(t, viewer, "viewer.norte@gopanel.test")
	f.selectTenant(t, viewer, "empresa-norte")
	resp, _ = f.send(t, viewer, "GET", "/app/members", nil, false)
	want(t, resp, 403)
	platform := f.client()
	f.login(t, platform, "platform@gopanel.test")
	resp, body = f.send(t, platform, "GET", "/platform/metrics", nil, false)
	want(t, resp, 200)
	var metrics map[string]any
	if json.Unmarshal([]byte(body), &metrics) != nil || metrics["http"] == nil {
		t.Fatal("metrics not JSON")
	}
	resp, _ = f.send(t, platform, "GET", "/app", nil, false)
	want(t, resp, 303)
	resp, _ = f.send(t, admin, "POST", "/logout", url.Values{"csrf_token": {token}}, true)
	want(t, resp, 200)
	resp, _ = f.send(t, admin, "GET", "/app", nil, false)
	want(t, resp, 303)
	f.stop(t)
	logs, err := os.ReadFile(f.log.Name())
	if err != nil {
		t.Fatal(err)
	}
	for _, password := range f.credentials {
		if strings.Contains(string(logs), password) {
			t.Fatal("credential in logs")
		}
	}
	writeReport(t, "e2e-result.json", map[string]any{"passed": true, "transport": "native binary HTTP + MySQL", "journey": "login, tenants, RBAC, CRUD/HTMX, SSE, restart, metrics, logout", "at_utc": time.Now().UTC()})
}
