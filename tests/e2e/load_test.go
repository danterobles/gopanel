//go:build load

package e2e_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	dbgen "gopanel/internal/db/generated"
	"gopanel/internal/platform/id"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type phaseResult struct {
	Resource    string  `json:"resource"`
	Users       int     `json:"users"`
	Samples     int     `json:"samples"`
	Errors      int     `json:"errors"`
	Duration    float64 `json:"duration_seconds"`
	P50         float64 `json:"p50_ms"`
	P95         float64 `json:"p95_ms"`
	RPS         float64 `json:"rps"`
	MeetsTarget bool    `json:"p95_under_300ms"`
}

func TestLoadHTML(t *testing.T) {
	duration := 20 * time.Second
	if raw := os.Getenv("BENCH_DURATION"); raw != "" {
		var err error
		duration, err = time.ParseDuration(raw)
		if err != nil || duration < time.Second || duration > 2*time.Minute {
			t.Fatal("BENCH_DURATION must be 1s..2m")
		}
	}
	f := setup(t, 10000)
	q := dbgen.New(f.db)
	clients := make([]*http.Client, 50)
	expected := make([]string, 50)
	base, _ := url.Parse(f.base)
	for i := range clients {
		user := id.New()
		tenant, company := f.tenants["empresa-norte"], "Empresa norte"
		if i >= 25 {
			tenant, company = f.tenants["empresa-sur"], "Empresa sur"
		}
		if err := q.CreateUser(t.Context(), dbgen.CreateUserParams{ID: user, Email: fmt.Sprintf("load-%d@fixture.test", i), DisplayName: "Load fixture", PasswordHash: "$argon2id$fixture-login-disabled"}); err != nil {
			t.Fatal(err)
		}
		if err := q.CreateMembership(t.Context(), dbgen.CreateMembershipParams{ID: id.New(), UserID: user, TenantID: tenant, Role: "viewer", IsActive: true}); err != nil {
			t.Fatal(err)
		}
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(raw)
		now := time.Now().UTC()
		if err := q.CreateSession(t.Context(), dbgen.CreateSessionParams{TokenHash: hash[:], UserID: user, ActiveTenantID: sql.NullString{String: tenant, Valid: true}, CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
		c := f.client()
		c.Jar.SetCookies(base, []*http.Cookie{{Name: "gopanel_session", Value: base64.RawURLEncoding.EncodeToString(raw), Path: "/"}})
		clients[i] = c
		expected[i] = company
	}
	plans := map[string]string{}
	for _, resource := range []string{"customers", "products"} {
		raw, err := os.ReadFile("../../internal/db/queries/" + resource + ".sql")
		if err != nil {
			t.Fatal(err)
		}
		name := "ListCustomers"
		if resource == "products" {
			name = "ListProducts"
		}
		if strings.Contains(string(raw), "-- name: "+name+"Recent :many") {
			name += "Recent"
		}
		_, query, ok := strings.Cut(string(raw), "-- name: "+name+" :many")
		if !ok {
			t.Fatal("missing actual list query")
		}
		query, _, _ = strings.Cut(query, "-- name:")
		arguments := []any{}
		placeholders := regexp.MustCompile(`sqlc.arg\(([^)]+)\)`)
		query = placeholders.ReplaceAllStringFunc(query, func(value string) string {
			key := placeholders.FindStringSubmatch(value)[1]
			parameter := ""
			if key == "tenant_id" {
				parameter = f.tenants["empresa-norte"]
			}
			if key == "sort" {
				parameter = "created_desc"
			}
			arguments = append(arguments, parameter)
			return "?"
		})
		arguments = append(arguments, 25, 0)
		rows, err := f.db.QueryContext(t.Context(), "EXPLAIN ANALYZE "+strings.TrimSpace(query), arguments...)
		if err != nil {
			t.Fatal(err)
		}
		plan := ""
		for rows.Next() {
			var line string
			if rows.Scan(&line) != nil {
				t.Fatal("explain scan")
			}
			plan += line + "\n"
		}
		rows.Close()
		plans[resource] = strings.ReplaceAll(plan, f.tenants["empresa-norte"], "<tenant>")
	}
	writeReport(t, "benchmark-query-plans.json", plans)
	f.start(t)
	operator := f.client()
	f.login(t, operator, "platform@gopanel.test")
	var peakRSS int64
	var peakHeap, peakSys uint64
	monitorCtx, cancelMonitor := context.WithCancel(t.Context())
	defer cancelMonitor()
	processPID := f.command.Process.Pid
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-monitorCtx.Done():
				return
			case <-ticker.C:
				resp, err := operator.Get(f.base + "/platform/metrics")
				if err == nil {
					var data struct {
						Heap uint64 `json:"heap_alloc_bytes"`
						Sys  uint64 `json:"go_sys_bytes"`
					}
					_ = json.NewDecoder(resp.Body).Decode(&data)
					resp.Body.Close()
					if data.Heap > peakHeap {
						peakHeap = data.Heap
					}
					if data.Sys > peakSys {
						peakSys = data.Sys
					}
				}
				raw, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(processPID)).Output()
				if err == nil {
					rss, _ := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
					if rss*1024 > peakRSS {
						peakRSS = rss * 1024
					}
				}
			}
		}
	}()
	results := []phaseResult{}
	for _, resource := range []string{"customers", "products"} {
		path := "/app/" + resource + "?page=1&page_size=25&sort=created_desc"
		// Two requests per identity warm both tenants and their sessions.
		var warm sync.WaitGroup
		for _, client := range clients {
			warm.Add(1)
			go func(c *http.Client) {
				defer warm.Done()
				for i := 0; i < 2; i++ {
					resp, err := c.Get(f.base + path)
					if err == nil {
						io.Copy(io.Discard, resp.Body)
						resp.Body.Close()
					}
				}
			}(client)
		}
		warm.Wait()
		type worker struct {
			latencies []float64
			errors    int
		}
		completed := make(chan worker, 50)
		start := time.Now()
		deadline := start.Add(duration)
		for i, client := range clients {
			go func(c *http.Client, company string) {
				out := worker{}
				for time.Now().Before(deadline) {
					began := time.Now()
					resp, err := c.Get(f.base + path)
					if err != nil {
						out.errors++
						continue
					}
					body, err := io.ReadAll(resp.Body)
					resp.Body.Close()
					elapsed := float64(time.Since(began).Microseconds()) / 1000
					out.latencies = append(out.latencies, elapsed)
					if err != nil || resp.StatusCode != 200 || !strings.Contains(string(body), company) {
						out.errors++
					}
				}
				completed <- out
			}(client, expected[i])
		}
		all := []float64{}
		errors := 0
		for range clients {
			worker := <-completed
			all = append(all, worker.latencies...)
			errors += worker.errors
		}
		elapsed := time.Since(start).Seconds()
		sort.Float64s(all)
		if len(all) == 0 {
			t.Fatal("no load samples")
		}
		percentile := func(p float64) float64 { return all[int(math.Ceil(float64(len(all))*p))-1] }
		result := phaseResult{resource, 50, len(all), errors, elapsed, percentile(.5), percentile(.95), float64(len(all)) / elapsed, percentile(.95) < 300}
		results = append(results, result)
		t.Logf("%s: samples=%d p50=%.2fms p95=%.2fms rps=%.1f errors=%d", resource, result.Samples, result.P50, result.P95, result.RPS, result.Errors)
	}
	cancelMonitor()
	<-done
	var mysqlVersion string
	if err := f.db.QueryRowContext(t.Context(), "SELECT VERSION()").Scan(&mysqlVersion); err != nil {
		t.Fatal(err)
	}
	hardware := map[string]any{"goos": runtime.GOOS, "goarch": runtime.GOARCH, "logical_cpus": runtime.NumCPU(), "go": runtime.Version(), "mysql": mysqlVersion, "load_generator": "same host; closed workload; native binary without race"}
	for key, setting := range map[string]string{"cpu": "machdep.cpu.brand_string", "physical_memory_bytes": "hw.memsize"} {
		if raw, err := exec.Command("sysctl", "-n", setting).Output(); err == nil {
			hardware[key] = strings.TrimSpace(string(raw))
		}
	}
	writeReport(t, "benchmark-go.json", map[string]any{"at_utc": time.Now().UTC(), "tenants": 2, "customers_per_tenant": 10000, "products_per_tenant": 10000, "page_size": 25, "users": 50, "warmup_requests_per_phase": 100, "measurement_seconds_per_phase": duration.Seconds(), "hardware": hardware, "server_rss_peak_bytes_sampled": peakRSS, "server_heap_peak_bytes_sampled": peakHeap, "server_go_sys_peak_bytes_sampled": peakSys, "phases": results, "comparison": "Laravel/Filament unavailable; no comparative conclusion"})
	for _, r := range results {
		if r.Errors != 0 {
			t.Errorf("%s load HTTP errors: %d", r.Resource, r.Errors)
		}
	}
}
