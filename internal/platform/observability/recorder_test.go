package observability

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestConcurrentRecordingHistogramAndCopies(t *testing.T) {
	r := New()
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); r.Start(); r.Finish("GET", "/app/customers/{id}", 200, 20*time.Millisecond) }()
	}
	wg.Wait()
	snapshot := r.Snapshot()
	if snapshot.Active != 0 || len(snapshot.Samples) != 1 || snapshot.Samples[0].Count != 40 || snapshot.Samples[0].DurationMS != 800 || snapshot.Samples[0].Buckets[0] != 0 || snapshot.Samples[0].Buckets[2] != 40 {
		t.Fatal(snapshot)
	}
	snapshot.Samples[0].Buckets[2] = 0
	if r.Snapshot().Samples[0].Buckets[2] != 40 {
		t.Fatal("mutable metrics snapshot")
	}
}
func TestCardinalityAndMethodNormalization(t *testing.T) {
	r := New()
	for i := 0; i < 1000; i++ {
		r.Start()
		r.Finish(fmt.Sprint(i), fmt.Sprint(i), 200, time.Millisecond)
	}
	snapshot := r.Snapshot()
	if len(snapshot.Samples) > 513 {
		t.Fatal("unbounded labels")
	}
	for _, s := range snapshot.Samples {
		if s.Method != "OTHER" {
			t.Fatal("untrusted method label")
		}
	}
}
