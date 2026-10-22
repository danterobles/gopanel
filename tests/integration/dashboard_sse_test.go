//go:build integration

package integration_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"gopanel/internal/modules/customers"
	"gopanel/internal/modules/dashboard"
	"gopanel/internal/modules/products"
	"gopanel/internal/platform/events"
	"gopanel/internal/platform/tenancy"
)

func TestDashboardUTCCountsIsolationAndHTTP(t *testing.T) {
	f := newAuthFixture(t, false)
	now := time.Date(2026, 3, 1, 10, 30, 0, 0, time.UTC)
	start := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	for i, entry := range []struct {
		date    time.Time
		state   string
		deleted bool
	}{{start.Add(-time.Microsecond), "active", false}, {start, "active", false}, {start.Add(time.Microsecond), "inactive", false}, {now.Add(-time.Microsecond), "active", false}, {now, "active", false}, {now.Add(time.Hour), "active", false}, {start.Add(time.Hour), "active", true}} {
		id := createCustomer(t, f, f.north, fmt.Sprint("customer", i), entry.state)
		if _, err := f.db.ExecContext(t.Context(), "UPDATE customers SET created_at=?,deleted_at=? WHERE tenant_id=? AND id=?", entry.date, func() any {
			if entry.deleted {
				return now
			}
			return nil
		}(), f.north, id); err != nil {
			t.Fatal(err)
		}
	}
	createCustomer(t, f, f.south, "foreign customer", "active")
	createProduct(t, f, f.north, "a", "a", "active", 1)
	createProduct(t, f, f.north, "b", "b", "inactive", 1)
	deleted := createProduct(t, f, f.north, "c", "c", "active", 1)
	if _, err := f.db.ExecContext(t.Context(), "UPDATE products SET deleted_at=? WHERE tenant_id=? AND id=?", now, f.north, deleted); err != nil {
		t.Fatal(err)
	}
	createProduct(t, f, f.south, "a", "foreign", "active", 1)
	snapshot, err := dashboard.New(f.db, func() time.Time { return now }).Read(t.Context(), tenancy.ActorContext{UserID: f.user, TenantID: f.north, Role: "viewer"})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ActiveCustomers != 5 || snapshot.ActiveProducts != 1 || snapshot.RecentCustomers != 3 || len(snapshot.Days) != 30 || snapshot.Days[0].Total != 2 || snapshot.Days[29].Total != 1 || snapshot.Days[1].Total != 0 {
		t.Fatalf("wrong aggregate: %+v", snapshot)
	}
	c := f.client()
	f.login(t, c, "member@auth.test")
	f.selectTenant(t, c, f.south)
	resp, guardBody := f.send(t, c, "GET", "/app/dashboard/widgets", nil, map[string]string{"HX-Request": "true", "X-Dashboard-Scope": f.north + ":admin"})
	status(t, resp, 200)
	if resp.Header.Get("HX-Redirect") != "/app" || guardBody != "" {
		t.Fatal("stale dashboard mixed tenants")
	}
	resp, body := f.send(t, c, "GET", "/app?tenant_id="+f.north, nil, nil)
	status(t, resp, 200)
	for _, part := range []string{"Dashboard", "south", "viewer", "sse-connect=\"/app/events\"", "/static/sse.js", "id=\"active-customers\" class=\"mt-3 text-3xl font-bold\">1<"} {
		if !strings.Contains(body, part) {
			t.Fatalf("dashboard missing %q", part)
		}
	}
	if strings.Contains(body, "foreign customer") {
		t.Fatal("PII in aggregates")
	}
	resp, body = f.send(t, c, "GET", "/app/dashboard/widgets?tenant_id="+f.north, nil, map[string]string{"HX-Request": "true"})
	status(t, resp, 200)
	if strings.Contains(strings.ToLower(body), "<!doctype") || strings.Contains(body, "sse-connect") {
		t.Fatal("widgets reconnect stream")
	}
	resp, body = f.send(t, c, "GET", "/app/dashboard/widgets", nil, nil)
	status(t, resp, 200)
	if !strings.Contains(strings.ToLower(body), "<!doctype") {
		t.Fatal("normal widgets request missing document")
	}
	emptyActor := tenancy.ActorContext{UserID: f.user, TenantID: f.foreign, Role: "owner"}
	if _, err = dashboard.New(f.db, nil).Read(t.Context(), emptyActor); err == nil {
		t.Fatal("forged owner read foreign dashboard")
	}
	f.selectTenant(t, c, f.north)
	if _, err = f.db.ExecContext(t.Context(), "RENAME TABLE products TO products_unavailable"); err != nil {
		t.Fatal(err)
	}
	resp, body = f.send(t, c, "GET", "/app/dashboard/widgets", nil, map[string]string{"HX-Request": "true"})
	status(t, resp, 503)
	if !strings.Contains(body, "No se pudieron consultar") || strings.Contains(body, "products_unavailable") {
		t.Fatal("unsafe failure state")
	}
	if _, err = f.db.ExecContext(t.Context(), "RENAME TABLE products_unavailable TO products"); err != nil {
		t.Fatal(err)
	}
}

type eventFrame struct {
	name, data string
	err        error
}
type eventReader struct {
	response *http.Response
	frames   chan eventFrame
	cancel   context.CancelFunc
}

func openEvents(t *testing.T, f *authFixture, c *http.Client, headers map[string]string) *eventReader {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	req, err := http.NewRequestWithContext(ctx, "GET", f.server.URL+"/app/events?tenant_id="+f.foreign, nil)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := c.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	status(t, resp, 200)
	if resp.Header.Get("Content-Type") != "text/event-stream" || resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("X-Accel-Buffering") != "no" {
		t.Fatal("SSE headers missing")
	}
	reader := &eventReader{response: resp, frames: make(chan eventFrame, 40), cancel: cancel}
	t.Cleanup(func() { cancel(); resp.Body.Close() })
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		frame := eventFrame{}
		for scanner.Scan() {
			line := scanner.Text()
			if line == ": heartbeat" {
				select {
				case reader.frames <- eventFrame{name: "heartbeat"}:
				case <-ctx.Done():
					return
				}
				continue
			}
			if line == "" {
				if frame.name != "" {
					select {
					case reader.frames <- frame:
					case <-ctx.Done():
						return
					}
				}
				frame = eventFrame{}
				continue
			}
			if strings.HasPrefix(line, "event: ") {
				frame.name = strings.TrimPrefix(line, "event: ")
			}
			if strings.HasPrefix(line, "data: ") {
				frame.data = strings.TrimPrefix(line, "data: ")
			}
		}
		err := scanner.Err()
		if err == nil {
			err = io.EOF
		}
		select {
		case reader.frames <- eventFrame{err: err}:
		case <-ctx.Done():
		}
	}()
	reader.next(t, "ready")
	return reader
}
func (r *eventReader) next(t *testing.T, name string) eventFrame {
	t.Helper()
	select {
	case frame := <-r.frames:
		if frame.err != nil || frame.name != name {
			t.Fatalf("event got %+v want %s", frame, name)
		}
		return frame
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
		return eventFrame{}
	}
}
func (r *eventReader) quiet(t *testing.T) {
	t.Helper()
	select {
	case frame := <-r.frames:
		t.Fatalf("unexpected frame: %+v", frame)
	case <-time.After(100 * time.Millisecond):
	}
}
func (r *eventReader) close() { r.cancel(); r.response.Body.Close() }

func TestSSETenantIsolationCommitRollbackAndReconnect(t *testing.T) {
	f := newAuthFixture(t, false)
	c := f.client()
	f.login(t, c, "member@auth.test")
	csrf := f.selectTenant(t, c, f.north)
	stream := openEvents(t, f, c, nil)
	south, _, _, _ := f.owner(t, "south", f.south)
	other := openEvents(t, f, south, nil)
	fields := url.Values{"csrf_token": {csrf}, "name": {"Private customer"}, "status": {"active"}}
	resp, _ := f.send(t, c, "POST", "/app/customers/new", fields, nil)
	status(t, resp, 303)
	location, _ := url.Parse(resp.Header.Get("Location"))
	resource := strings.TrimPrefix(location.Path, "/app/customers/")
	first := stream.next(t, "invalidate")
	var data map[string]string
	if err := json.Unmarshal([]byte(first.data), &data); err != nil {
		t.Fatal(err)
	}
	if len(data) != 3 || data["type"] != "invalidate" || data["resource"] != "customers" || data["version"] == "" || strings.Contains(first.data, f.north) || strings.Contains(first.data, resource) {
		t.Fatal("event contains wrong fields")
	}
	other.quiet(t)
	// The row and audit must already be visible when the event arrives.
	var n int
	if err := f.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM audit_logs WHERE entity_id=? AND action='customers.create'", resource).Scan(&n); err != nil || n != 1 {
		t.Fatal("event before audit commit", err)
	}
	fields.Set("name", "Updated")
	resp, _ = f.send(t, c, "POST", "/app/customers/"+resource+"/edit", fields, nil)
	status(t, resp, 303)
	stream.next(t, "invalidate")
	resp, _ = f.send(t, c, "POST", "/app/customers/"+resource+"/delete", url.Values{"csrf_token": {csrf}, "confirm": {"yes"}}, nil)
	status(t, resp, 303)
	stream.next(t, "invalidate")
	resp, _ = f.send(t, c, "POST", "/app/products/new", productFields(csrf, "EVENT-SKU"), nil)
	status(t, resp, 303)
	productLocation, _ := url.Parse(resp.Header.Get("Location"))
	productPath := productLocation.Path
	if e := stream.next(t, "invalidate"); !strings.Contains(e.data, "products") {
		t.Fatal(e)
	}
	resp, _ = f.send(t, c, "POST", productPath+"/edit", productFields(csrf, "EVENT-SKU"), nil)
	status(t, resp, 303)
	stream.next(t, "invalidate")
	resp, _ = f.send(t, c, "POST", productPath+"/delete", url.Values{"csrf_token": {csrf}, "confirm": {"yes"}}, nil)
	status(t, resp, 303)
	stream.next(t, "invalidate")
	stream.close()
	stream = openEvents(t, f, c, map[string]string{"Last-Event-ID": "untrusted-stale-id"})
	stream.quiet(t)
	actor := tenancy.ActorContext{UserID: f.user, TenantID: f.north, Role: "owner"}
	customerID := createCustomer(t, f, f.north, "rollback", "active")
	productID := createProduct(t, f, f.north, "rollback", "rollback", "active", 1)
	if _, err := f.db.ExecContext(t.Context(), "RENAME TABLE audit_logs TO audit_unavailable"); err != nil {
		t.Fatal(err)
	}
	ca := customers.New(f.db, f.broker)
	pa := products.New(f.db, f.broker)
	ci := customers.Input{Name: "rollback", Status: "active"}
	pi := products.Input{SKU: "rollback", Name: "rollback", Price: "1.00", Currency: "MXN", Status: "active"}
	for _, rid := range []string{"", customerID} {
		if _, _, err := ca.Save(t.Context(), actor, rid, ci, ""); err == nil {
			t.Fatal("customer audit failure committed")
		}
	}
	for _, rid := range []string{"", productID} {
		pi.SKU = "new-rollback"
		if _, _, err := pa.Save(t.Context(), actor, rid, pi, ""); err == nil {
			t.Fatal("product audit failure committed")
		}
	}
	if ca.Delete(t.Context(), actor, customerID, "yes", "") == nil || pa.Delete(t.Context(), actor, productID, "yes", "") == nil {
		t.Fatal("delete without audit succeeded")
	}
	if _, err := f.db.ExecContext(t.Context(), "RENAME TABLE audit_unavailable TO audit_logs"); err != nil {
		t.Fatal(err)
	}
	stream.quiet(t)
	other.quiet(t)
	fields.Set("name", "")
	resp, _ = f.send(t, c, "POST", "/app/customers/new", fields, nil)
	status(t, resp, 422)
	stream.quiet(t)
	resp, _ = f.send(t, c, "POST", "/app/products/new", productFields(csrf, "bad"), map[string]string{"Origin": "https://other.test"})
	status(t, resp, 403)
	stream.quiet(t)
	stream.close()
	other.close()
}

func TestSSERevalidationRevocationAndPassiveActivity(t *testing.T) {
	for _, mode := range []string{"logout", "rotate", "expiry", "idle", "membership", "tenant", "user"} {
		t.Run(mode, func(t *testing.T) {
			f := newAuthFixture(t, false)
			c := f.client()
			f.login(t, c, "member@auth.test")
			csrf := f.selectTenant(t, c, f.north)
			token := f.cookieValue(t, c)
			hash := hashToken(t, token)
			idle := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Microsecond)
			if _, err := f.db.ExecContext(t.Context(), "UPDATE sessions SET last_seen_at=? WHERE token_hash=?", idle, hash); err != nil {
				t.Fatal(err)
			}
			stream := openEvents(t, f, c, nil)
			resp, _ := f.send(t, c, "GET", "/app/dashboard/widgets", nil, map[string]string{"HX-Request": "true"})
			status(t, resp, 200)
			var seen time.Time
			if err := f.db.QueryRowContext(t.Context(), "SELECT last_seen_at FROM sessions WHERE token_hash=?", hash).Scan(&seen); err != nil || !seen.Equal(idle) {
				t.Fatal("background activity renewed session", seen, err)
			}
			var err error
			switch mode {
			case "logout":
				resp, _ = f.send(t, c, "POST", "/logout", url.Values{"csrf_token": {csrf}}, nil)
				status(t, resp, 303)
			case "rotate":
				f.selectTenant(t, c, f.south)
			case "expiry":
				_, err = f.db.ExecContext(t.Context(), "UPDATE sessions SET expires_at=? WHERE token_hash=?", time.Now().Add(-time.Second), hash)
			case "idle":
				_, err = f.db.ExecContext(t.Context(), "UPDATE sessions SET last_seen_at=? WHERE token_hash=?", time.Now().Add(-time.Hour), hash)
			case "membership":
				_, err = f.db.ExecContext(t.Context(), "UPDATE tenant_memberships SET is_active=0 WHERE tenant_id=? AND user_id=?", f.north, f.user)
			case "tenant":
				_, err = f.db.ExecContext(t.Context(), "UPDATE tenants SET is_active=0 WHERE id=?", f.north)
			case "user":
				_, err = f.db.ExecContext(t.Context(), "UPDATE users SET is_active=0 WHERE id=?", f.user)
			}
			if err != nil {
				t.Fatal(err)
			}
			stream.next(t, "closed")
			select {
			case frame := <-stream.frames:
				if frame.err == nil {
					t.Fatal("data after revocation")
				}
			case <-time.After(time.Second):
				t.Fatal("stream did not close")
			}
			stream.close()
		})
	}
}

func TestSSEAdmissionAuthorizationAndShutdown(t *testing.T) {
	f := newAuthFixture(t, false)
	c := f.client()
	resp, _ := f.send(t, c, "GET", "/app/events", nil, nil)
	status(t, resp, 401)
	f.login(t, c, "platform@auth.test")
	resp, _ = f.send(t, c, "GET", "/app/events", nil, nil)
	status(t, resp, 403)
	c = f.client()
	f.login(t, c, "member@auth.test")
	f.selectTenant(t, c, f.south)
	resp, _ = f.send(t, c, "GET", "/app/events", nil, map[string]string{"Origin": "https://other.test"})
	status(t, resp, 403)
	streams := []*eventReader{}
	for i := 0; i < events.MaxUserConnections; i++ {
		streams = append(streams, openEvents(t, f, c, nil))
	}
	resp, _ = f.send(t, c, "GET", "/app/events", nil, nil)
	status(t, resp, 503)
	if resp.Header.Get("Retry-After") != "3" {
		t.Fatal("capacity retry missing")
	}
	f.broker.Close()
	for _, stream := range streams {
		select {
		case frame := <-stream.frames:
			if frame.err == nil {
				t.Fatal("expected shutdown EOF")
			}
		case <-time.After(time.Second):
			t.Fatal("shutdown stuck")
		}
		stream.close()
	}
}

func TestSSEHeartbeatBeyondServerWriteTimeout(t *testing.T) {
	f := newAuthFixture(t, false)
	c := f.client()
	f.login(t, c, "member@auth.test")
	f.selectTenant(t, c, f.north)
	stream := openEvents(t, f, c, nil)
	select {
	case frame := <-stream.frames:
		if frame.name != "heartbeat" || frame.err != nil {
			t.Fatalf("heartbeat lost: %+v", frame)
		}
	case <-time.After(17 * time.Second):
		t.Fatal("heartbeat timeout")
	}
	f.broker.Publish(f.north, "customers")
	stream.next(t, "invalidate")
	stream.close()
}
func TestSSEDatabaseFailureClosesAndReconnects(t *testing.T) {
	f := newAuthFixture(t, false)
	c := f.client()
	f.login(t, c, "member@auth.test")
	f.selectTenant(t, c, f.north)
	stream := openEvents(t, f, c, nil)
	if _, err := f.db.ExecContext(t.Context(), "RENAME TABLE sessions TO sessions_unavailable"); err != nil {
		t.Fatal(err)
	}
	select {
	case frame := <-stream.frames:
		if frame.err == nil {
			t.Fatal("data sent during DB failure")
		}
	case <-time.After(4 * time.Second):
		t.Fatal("stream retained invalid authorization")
	}
	stream.close()
	if _, err := f.db.ExecContext(t.Context(), "RENAME TABLE sessions_unavailable TO sessions"); err != nil {
		t.Fatal(err)
	}
	reconnected := openEvents(t, f, c, nil)
	reconnected.close()
}
