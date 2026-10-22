// Package events carries ephemeral invalidation signals, never business data.
package events

import (
	"errors"
	"sync"

	"gopanel/internal/platform/id"
)

const (
	MaxConnections       = 256
	MaxTenantConnections = 32
	MaxUserConnections   = 4
	BufferSize           = 16
)

var ErrCapacity = errors.New("event subscriber capacity reached")
var ErrClosed = errors.New("event broker closed")

type Event struct {
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Version  string `json:"version"`
}
type Publisher interface{ Publish(tenant, resource string) }
type Subscription struct {
	Events <-chan Event
	Done   <-chan struct{}
	cancel func()
}

func (s *Subscription) Cancel() { s.cancel() }

type subscriber struct {
	user    string
	events  chan Event
	done    chan struct{}
	stopped bool
}
type Broker struct {
	mu      sync.Mutex
	tenants map[string]map[*subscriber]struct{}
	count   int
	closed  bool
}

func New() *Broker { return &Broker{tenants: make(map[string]map[*subscriber]struct{})} }
func (b *Broker) Subscribe(tenant, user string) (*Subscription, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, ErrClosed
	}
	if tenant == "" || user == "" {
		return nil, ErrCapacity
	}
	userCount := 0
	for _, bucket := range b.tenants {
		for s := range bucket {
			if s.user == user {
				userCount++
			}
		}
	}
	if b.count >= MaxConnections || len(b.tenants[tenant]) >= MaxTenantConnections || userCount >= MaxUserConnections {
		return nil, ErrCapacity
	}
	s := &subscriber{user: user, events: make(chan Event, BufferSize), done: make(chan struct{})}
	if b.tenants[tenant] == nil {
		b.tenants[tenant] = make(map[*subscriber]struct{})
	}
	b.tenants[tenant][s] = struct{}{}
	b.count++
	return &Subscription{Events: s.events, Done: s.done, cancel: func() { b.mu.Lock(); defer b.mu.Unlock(); b.remove(tenant, s) }}, nil
}
func (b *Broker) remove(tenant string, s *subscriber) {
	if _, ok := b.tenants[tenant][s]; !ok {
		return
	}
	delete(b.tenants[tenant], s)
	b.count--
	if !s.stopped {
		s.stopped = true
		close(s.done)
	}
	if len(b.tenants[tenant]) == 0 {
		delete(b.tenants, tenant)
	}
}
func (b *Broker) Publish(tenant, resource string) {
	if resource != "customers" && resource != "products" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	event := Event{Type: "invalidate", Resource: resource, Version: id.New()}
	for s := range b.tenants[tenant] {
		if s.stopped {
			continue
		}
		select {
		case s.events <- event:
		default:
			s.stopped = true
			close(s.done) // Hold admission until the handler releases the connection.
		}
	}
}
func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for tenant, bucket := range b.tenants {
		for s := range bucket {
			b.remove(tenant, s)
		}
	}
}
