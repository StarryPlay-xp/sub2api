package egress

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
)

type routeStore struct {
	mu     sync.Mutex
	record Record
	failed bool
}

func (s *routeStore) ResolveReference(context.Context, Reference) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failed {
		return Record{}, errors.New("unavailable")
	}
	return s.record, nil
}
func (s *routeStore) LoadNode(context.Context, int64) (wgtunnel.Node, error) {
	return wgtunnel.Node{}, ErrBlocked
}
func (s *routeStore) SaveCheckpoint(context.Context, wgtunnel.Checkpoint, bool) error { return nil }
func testRouter(t *testing.T, record Record) (*Router, *routeStore) {
	t.Helper()
	store := &routeStore{record: record}
	manager := wgtunnel.NewManager(store, nil)
	t.Cleanup(manager.Close)
	router, err := NewRouter(store, manager)
	if err != nil {
		t.Fatal(err)
	}
	return router, store
}
func TestPooledDirectConnectionDoesNotSurviveBindingChange(t *testing.T) {
	router, store := testRouter(t, Record{Kind: Direct, AccountID: 1, Revision: 1})
	route, err := router.Resolve(context.Background(), Reference{AccountID: 1})
	if err != nil {
		t.Fatal(err)
	}
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { count.Add(1); _, _ = w.Write([]byte("ok")) }))
	defer server.Close()
	tr := &http.Transport{}
	if err = route.ConfigureHTTP(tr); err != nil {
		t.Fatal(err)
	}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: route.Wrap(tr)}
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	store.mu.Lock()
	store.record = Record{Kind: WireGuard, NodeID: 10, AccountID: 1, Revision: 2}
	store.mu.Unlock()
	if _, err = client.Get(server.URL); !errors.Is(err, ErrBlocked) {
		t.Fatal("pooled connection ignored changed binding")
	}
	if count.Load() != 1 {
		t.Fatal("request escaped after binding changed")
	}
}
func TestInvalidReferencesNeverSelectDirect(t *testing.T) {
	router, store := testRouter(t, Record{Kind: Direct})
	for _, ref := range []Reference{{}, {AccountID: 1, ExplicitDirect: true}, {AccountID: -1}, {NodeID: 1, AccountID: 2}} {
		if _, err := router.Resolve(context.Background(), ref); !errors.Is(err, ErrBlocked) {
			t.Fatal("ambiguous reference accepted")
		}
	}
	store.failed = true
	if _, err := router.Resolve(context.Background(), Reference{ExplicitDirect: true}); !errors.Is(err, ErrBlocked) {
		t.Fatal("resolver failure became direct")
	}
}
func TestEvidenceMatchesActualConnectionFamilyAndIsImmutable(t *testing.T) {
	router, _ := testRouter(t, Record{Kind: Direct})
	route, err := router.Resolve(context.Background(), Reference{ExplicitDirect: true})
	if err != nil {
		t.Fatal(err)
	}
	observer := NewObserver(router, nil)
	defer observer.Close()
	now := time.Now()
	observer.evidence[route.EvidenceKey()+":ipv4"] = wgtunnel.Evidence{IP: "192.0.2.1", Family: "ipv4", Source: "route_probe", State: "observed", ObservedAt: now, ExpiresAt: now.Add(time.Minute)}
	snapshot := observer.Snapshot(route, &net.TCPAddr{IP: net.ParseIP("198.51.100.1"), Port: 443}, true)
	if snapshot.Family != "ipv4" || snapshot.Evidence.IP != "192.0.2.1" || !snapshot.ConnectionReused {
		t.Fatal("missing family-specific evidence")
	}
	ipv6 := observer.Snapshot(route, &net.TCPAddr{IP: net.ParseIP("2001:db8::1"), Port: 443}, false)
	if ipv6.Evidence.State != "unobserved" || ipv6.Evidence.IP != "" {
		t.Fatal("IPv4 evidence was applied to IPv6")
	}
	observer.evidence[route.EvidenceKey()+":ipv4"] = wgtunnel.Evidence{IP: "192.0.2.2", ExpiresAt: now.Add(-time.Minute)}
	if snapshot.Evidence.IP != "192.0.2.1" {
		t.Fatal("historical snapshot mutated")
	}
	stale := observer.Snapshot(route, &net.TCPAddr{IP: net.ParseIP("198.51.100.1"), Port: 443}, false)
	if stale.Evidence.State != "expired" {
		t.Fatal("expired evidence claimed freshness")
	}
}

func TestRequestScopeNeverChangesRouteAfterWireGuardSelection(t *testing.T) {
	ctx := WithScope(context.Background())
	scope := ScopeFromContext(ctx)
	if WithScope(ctx) != ctx {
		t.Fatal("nested call replaced shared scope")
	}
	direct := &Route{record: Record{Kind: Direct, AccountID: 1}}
	if err := scope.Pin(direct); err != nil || scope.BlocksFailover() {
		t.Fatal("direct-only request was pinned")
	}
	wg := &Route{record: Record{Kind: WireGuard, AccountID: 2, NodeID: 10}, version: 1, runtimeID: "first"}
	if err := scope.Pin(wg); err != nil || !scope.BlocksFailover() {
		t.Fatal("WG selection was not pinned")
	}
	if err := scope.Pin(wg); err != nil {
		t.Fatal("same WG attempt rejected")
	}
	for _, changed := range []*Route{direct, {record: Record{Kind: WireGuard, AccountID: 3, NodeID: 10}, version: 1, runtimeID: "first"}, {record: wg.record, version: 2, runtimeID: "second"}} {
		if err := scope.Pin(changed); !errors.Is(err, ErrBlocked) {
			t.Fatal("request switched its route")
		}
	}
}

func TestUnavailableWGStillBlocksAccountFailover(t *testing.T) {
	router, store := testRouter(t, Record{Kind: WireGuard, AccountID: 1, NodeID: 10, Revision: 1})
	ctx := WithScope(context.Background())
	if _, err := router.Resolve(ctx, Reference{AccountID: 1}); !errors.Is(err, ErrBlocked) {
		t.Fatal("missing node unexpectedly usable")
	}
	if !ScopeFromContext(ctx).BlocksFailover() {
		t.Fatal("unavailable WG did not pin request")
	}
	store.mu.Lock()
	store.record = Record{Kind: Direct, AccountID: 2, Revision: 1}
	store.mu.Unlock()
	if _, err := router.Resolve(ctx, Reference{AccountID: 2}); !errors.Is(err, ErrBlocked) {
		t.Fatal("unavailable WG fell back to a direct account")
	}
}
func TestUnverifiableRoutePoisonsOnlyCurrentRequest(t *testing.T) {
	router, store := testRouter(t, Record{Kind: Direct, AccountID: 1})
	ctx := WithScope(context.Background())
	store.failed = true
	if _, err := router.Resolve(ctx, Reference{AccountID: 1}); !errors.Is(err, ErrBlocked) {
		t.Fatal("failed resolution accepted")
	}
	store.failed = false
	if _, err := router.Resolve(ctx, Reference{AccountID: 1}); !errors.Is(err, ErrBlocked) {
		t.Fatal("failed request silently resumed with guessed routing")
	}
	if _, err := router.Resolve(WithScope(context.Background()), Reference{AccountID: 1}); err != nil {
		t.Fatal("unrelated new direct request was blocked")
	}
}

func TestActiveConnectionClosesAfterAccountEdit(t *testing.T) {
	router, store := testRouter(t, Record{Kind: Direct, AccountID: 1, Revision: 1})
	route, err := router.Resolve(context.Background(), Reference{AccountID: 1})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	accepted := make(chan net.Conn, 1)
	go func() { c, _ := listener.Accept(); accepted <- c }()
	conn, err := route.DialContext(context.Background(), "tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	peer := <-accepted
	defer func() { _ = peer.Close() }()
	store.mu.Lock()
	store.record.Revision++
	store.mu.Unlock()
	_ = conn.SetReadDeadline(time.Now().Add(4 * time.Second))
	_, err = conn.Read(make([]byte, 1))
	if err == nil {
		t.Fatal("active stream survived account edit")
	}
	if e, ok := err.(net.Error); ok && e.Timeout() {
		t.Fatal("connection only stopped at client timeout")
	}
}
