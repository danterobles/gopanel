//go:build integration

package integration_test

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOperationalMetricsPlatformOnlyAndNoPII(t *testing.T) {
	f := newAuthFixture(t, false)
	c := f.client()
	resp, _ := f.send(t, c, "GET", "/platform/metrics", nil, nil)
	status(t, resp, 303)
	f.login(t, c, "member@auth.test")
	f.selectTenant(t, c, f.north)
	resp, _ = f.send(t, c, "GET", "/platform/metrics", nil, nil)
	status(t, resp, 403)
	resp, _ = f.send(t, c, "GET", "/app/customers/00000000-0000-4000-8000-000000000000?search=private-token", nil, nil)
	status(t, resp, 404)
	p := f.client()
	f.login(t, p, "platform@auth.test")
	resp, body := f.send(t, p, "GET", "/platform/metrics?tenant_id="+f.north, nil, nil)
	status(t, resp, 200)
	var data map[string]any
	if json.Unmarshal([]byte(body), &data) != nil || data["http"] == nil || data["sse"] == nil || data["heap_alloc_bytes"] == nil {
		t.Fatal("metrics missing")
	}
	for _, secret := range []string{f.north, f.user, f.cookieValue(t, c), "private-token", "member@auth.test", "00000000-0000-4000-8000-000000000000"} {
		if strings.Contains(body, secret) {
			t.Fatal("PII or identifier in metrics")
		}
	}
	if !strings.Contains(body, "/app/customers/{id}") {
		t.Fatal("route pattern not aggregated")
	}
}
