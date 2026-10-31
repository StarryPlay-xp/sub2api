package egress

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
)

// Snapshot records the evidence available to one connection/attempt. It does not
// overwrite the caller's client IP and is never upgraded after a later probe.
type Snapshot struct {
	Route            Kind              `json:"route"`
	NodeID           int64             `json:"node_id,omitempty"`
	Version          int64             `json:"config_version,omitempty"`
	RuntimeID        string            `json:"runtime_id,omitempty"`
	Family           string            `json:"family"`
	ConnectionReused bool              `json:"connection_reused"`
	Evidence         wgtunnel.Evidence `json:"evidence"`
}

type EvidenceStore interface {
	SaveObservation(context.Context, string, wgtunnel.Evidence) error
	LatestObservation(context.Context, string, string) (wgtunnel.Evidence, error)
}

type watchedRoute struct {
	route  *Route
	seen   time.Time
	probed time.Time
}

// Observer performs no probe on the request's critical path. WG and explicit
// direct observations use separate network paths and runtime-scoped cache keys.
type Observer struct {
	mu       sync.Mutex
	router   *Router
	store    EvidenceStore
	routes   map[string]watchedRoute
	evidence map[string]wgtunnel.Evidence
	busy     map[string]bool
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
	workers  sync.WaitGroup
	slots    chan struct{}
}

func NewObserver(router *Router, store EvidenceStore) *Observer {
	ctx, cancel := context.WithCancel(context.Background())
	o := &Observer{router: router, store: store, routes: map[string]watchedRoute{}, evidence: map[string]wgtunnel.Evidence{}, busy: map[string]bool{}, ctx: ctx, cancel: cancel, done: make(chan struct{}), slots: make(chan struct{}, 4)}
	go o.loop()
	return o
}
func (o *Observer) Watch(route *Route) {
	if route == nil || route.Kind() == Proxy {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.ctx.Err() != nil {
		return
	}
	key := route.EvidenceKey()
	w := o.routes[key]
	w.route = route
	w.seen = time.Now()
	o.routes[key] = w
	if w.probed.IsZero() {
		o.scheduleLocked(key, w)
	}
}
func (o *Observer) scheduleLocked(key string, w watchedRoute) {
	if o.busy[key] || o.ctx.Err() != nil {
		return
	}
	select {
	case o.slots <- struct{}{}:
	default:
		return
	}
	o.busy[key] = true
	w.probed = time.Now()
	o.routes[key] = w
	o.workers.Add(1)
	go func() {
		defer o.workers.Done()
		defer func() { <-o.slots; o.mu.Lock(); delete(o.busy, key); o.mu.Unlock() }()
		ref := Reference{ExplicitDirect: true}
		if w.route.Kind() == WireGuard {
			ref = Reference{NodeID: w.route.NodeID()}
		}
		route, err := o.router.Resolve(o.ctx, ref)
		if err != nil || route.EvidenceKey() != key {
			return
		}
		for _, family := range []string{"ipv4", "ipv6"} {
			started := time.Now()
			observation, err := wgtunnel.ProbeWithService(o.ctx, route.DialContext, family, "icanhazip")
			if err != nil {
				route.confirmTunnelFailure(o.ctx, started)
				continue
			}
			observation.Route = string(route.Kind())
			observation.NodeID = route.NodeID()
			observation.Version = route.Version()
			observation.RuntimeID = route.RuntimeID()
			o.mu.Lock()
			o.evidence[key+":"+family] = observation
			o.mu.Unlock()
			if o.store != nil {
				ctx, cancel := context.WithTimeout(o.ctx, 3*time.Second)
				_ = o.store.SaveObservation(ctx, key, observation)
				cancel()
			}
		}
	}()
}
func (o *Observer) Snapshot(route *Route, remote net.Addr, reused bool) Snapshot {
	result := Snapshot{Route: route.Kind(), NodeID: route.NodeID(), Version: route.Version(), RuntimeID: route.RuntimeID(), Family: "unknown", ConnectionReused: reused, Evidence: wgtunnel.Evidence{State: "unobserved", Source: "none"}}
	// A proxy TCP peer describes the proxy-facing hop, not the target family.
	if remote == nil || route.Kind() == Proxy {
		return result
	}
	host, _, err := net.SplitHostPort(remote.String())
	if err != nil {
		return result
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return result
	}
	family := "ipv6"
	if ip.Unmap().Is4() {
		family = "ipv4"
	}
	result.Family = family
	o.mu.Lock()
	e, ok := o.evidence[route.EvidenceKey()+":"+family]
	o.mu.Unlock()
	if !ok {
		return result
	}
	if !e.ExpiresAt.After(time.Now()) {
		e.State = "expired"
	}
	result.Evidence = e
	return result
}
func (o *Observer) Close() { o.mu.Lock(); o.cancel(); o.mu.Unlock(); <-o.done; o.workers.Wait() }
func (o *Observer) loop() {
	defer close(o.done)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-o.ctx.Done():
			return
		case now := <-ticker.C:
			o.mu.Lock()
			for key, w := range o.routes {
				if now.Sub(w.seen) > 10*time.Minute {
					delete(o.routes, key)
					delete(o.evidence, key+":ipv4")
					delete(o.evidence, key+":ipv6")
					continue
				}
				if now.Sub(w.probed) >= 5*time.Minute {
					o.scheduleLocked(key, w)
				}
			}
			o.mu.Unlock()
		}
	}
}

func (o *Observer) ProbeNow(ctx context.Context, route *Route, family string) (wgtunnel.Evidence, error) {
	started := time.Now()
	evidence, err := wgtunnel.ProbeWithService(ctx, route.DialContext, family, "icanhazip")
	if err != nil {
		route.confirmTunnelFailure(ctx, started)
		return evidence, err
	}
	if err = route.Check(ctx); err != nil {
		return wgtunnel.Evidence{}, err
	}
	evidence.Route = string(route.Kind())
	evidence.NodeID = route.NodeID()
	evidence.Version = route.Version()
	evidence.RuntimeID = route.RuntimeID()
	o.mu.Lock()
	o.evidence[route.EvidenceKey()+":"+family] = evidence
	o.mu.Unlock()
	if o.store != nil {
		if err = o.store.SaveObservation(ctx, route.EvidenceKey(), evidence); err != nil {
			return evidence, err
		}
	}
	o.Watch(route)
	return evidence, nil
}
