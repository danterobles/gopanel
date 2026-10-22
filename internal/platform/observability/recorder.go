// Package observability stores bounded process telemetry, never cached business data.
package observability

import (
	"sort"
	"sync"
	"time"
)

var buckets = []float64{5, 10, 25, 50, 100, 250, 500, 1000, 2500}

type Key struct {
	Method string `json:"method"`
	Route  string `json:"route"`
	Status int    `json:"status"`
}
type Sample struct {
	Key
	Count      uint64   `json:"count"`
	DurationMS float64  `json:"duration_ms_sum"`
	Buckets    []uint64 `json:"duration_ms_cumulative"`
}
type Snapshot struct {
	Active  int64     `json:"active_requests"`
	Bounds  []float64 `json:"duration_ms_bounds"`
	Samples []Sample  `json:"requests"`
}
type Recorder struct {
	mu      sync.Mutex
	active  int64
	samples map[Key]*Sample
}

func New() *Recorder       { return &Recorder{samples: make(map[Key]*Sample)} }
func (r *Recorder) Start() { r.mu.Lock(); r.active++; r.mu.Unlock() }
func (r *Recorder) Finish(method, route string, status int, duration time.Duration) {
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
	default:
		method = "OTHER"
	}
	if route == "" {
		route = "unmatched"
	}
	if status < 100 || status > 599 {
		status = 500
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active--
	key := Key{method, route, status}
	sample := r.samples[key]
	// Registered routes are bounded; guard future dynamic route instrumentation too.
	if sample == nil && len(r.samples) >= 512 {
		key = Key{"OTHER", "overflow", 500}
		sample = r.samples[key]
	}
	if sample == nil {
		sample = &Sample{Key: key, Buckets: make([]uint64, len(buckets))}
		r.samples[key] = sample
	}
	elapsed := float64(duration.Microseconds()) / 1000
	sample.Count++
	sample.DurationMS += elapsed
	for i, bound := range buckets {
		if elapsed <= bound {
			sample.Buckets[i]++
		}
	}
}
func (r *Recorder) Snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := Snapshot{Active: r.active, Bounds: append([]float64(nil), buckets...), Samples: []Sample{}}
	for _, sample := range r.samples {
		copy := *sample
		copy.Buckets = append([]uint64(nil), sample.Buckets...)
		out.Samples = append(out.Samples, copy)
	}
	sort.Slice(out.Samples, func(i, j int) bool {
		a, b := out.Samples[i], out.Samples[j]
		if a.Route != b.Route {
			return a.Route < b.Route
		}
		if a.Method != b.Method {
			return a.Method < b.Method
		}
		return a.Status < b.Status
	})
	return out
}
