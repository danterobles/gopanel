package events

import (
	"fmt"
	"sync"
	"testing"
)

func TestBrokerIsolationLimitsOverflowAndCleanup(t *testing.T) {
	b := New()
	defer b.Close()
	a, _ := b.Subscribe("north", "user")
	s, _ := b.Subscribe("south", "other")
	defer a.Cancel()
	defer s.Cancel()
	b.Publish("north", "customers")
	select {
	case e := <-a.Events:
		if e.Type != "invalidate" || e.Resource != "customers" || e.Version == "" {
			t.Fatal(e)
		}
	default:
		t.Fatal("missing event")
	}
	select {
	case <-s.Events:
		t.Fatal("cross tenant event")
	default:
	}
	for i := 0; i < BufferSize+1; i++ {
		b.Publish("north", "products")
	}
	select {
	case <-a.Done:
	default:
		t.Fatal("slow subscriber not removed")
	}
	if b.count != 2 {
		t.Fatal("slow connection released admission before handler exit")
	}
	a.Cancel()
	for i := 1; i < MaxUserConnections; i++ {
		if _, err := b.Subscribe("south", "other"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := b.Subscribe("north", "other"); err != ErrCapacity {
		t.Fatal("per-user admission")
	}
	b.Close()
	if b.count != 0 || len(b.tenants) != 0 {
		t.Fatal("leaked subscriptions")
	}
	if _, err := b.Subscribe("north", "new"); err != ErrClosed {
		t.Fatal(err)
	}
}
func TestBrokerTenantAndGlobalCapacity(t *testing.T) {
	b := New()
	defer b.Close()
	for tenant := 0; tenant < MaxConnections/MaxTenantConnections; tenant++ {
		for user := 0; user < MaxTenantConnections; user++ {
			if _, err := b.Subscribe(fmt.Sprint(tenant), fmt.Sprintf("%d-%d", tenant, user)); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := b.Subscribe(fmt.Sprint(tenant), "extra"); err != ErrCapacity {
			t.Fatal("tenant limit")
		}
	}
	if _, err := b.Subscribe("extra", "extra"); err != ErrCapacity {
		t.Fatal("global limit")
	}
}
func TestBrokerConcurrentPublishSubscribeCancelClose(t *testing.T) {
	b := New()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				s, err := b.Subscribe("north", fmt.Sprint(i))
				b.Publish("north", "customers")
				if err == nil {
					s.Cancel()
					s.Cancel()
				}
			}
		}(i)
	}
	wg.Add(1)
	go func() { defer wg.Done(); b.Close() }()
	wg.Wait()
	b.Close()
	if b.count != 0 {
		t.Fatal("connections leaked")
	}
}
