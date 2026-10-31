// Package egress describes upstream routing independently of proxy URLs. A
// WireGuard reference is a node identity, never a pretend HTTP/SOCKS address.
package egress

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyutil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
	"github.com/imroc/req/v3"
)

type Kind string

const (
	Direct    Kind = "direct"
	Proxy     Kind = "proxy"
	WireGuard Kind = "wireguard"
)

var ErrBlocked = wgtunnel.ErrBlocked

type Reference struct {
	AccountID      int64
	NodeID         int64
	ExplicitDirect bool
}

func (r Reference) Valid() bool {
	n := 0
	if r.AccountID > 0 {
		n++
	}
	if r.NodeID > 0 {
		n++
	}
	if r.ExplicitDirect {
		n++
	}
	return n == 1 && r.AccountID >= 0 && r.NodeID >= 0
}

type Record struct {
	Kind            Kind
	ParentAccountID int64
	AccountID       int64
	NodeID          int64
	Revision        int64
	ProxyURL        string `json:"-"`
}

func (Record) String() string   { return "[egress record redacted]" }
func (Record) GoString() string { return "[egress record redacted]" }

type Store interface {
	ResolveReference(context.Context, Reference) (Record, error)
}

type Router struct {
	connMu      sync.Mutex
	connections map[string]*connectionGroup
	store       Store
	manager     *wgtunnel.Manager
	instance    string
	observer    *Observer
	recorder    *Recorder
}

func NewRouter(store Store, manager *wgtunnel.Manager) (*Router, error) {
	if store == nil || manager == nil {
		return nil, errors.New("egress dependencies are required")
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, err
	}
	return &Router{store: store, manager: manager, instance: hex.EncodeToString(id[:])}, nil
}

type Route struct {
	router     *Router
	ref        Reference
	record     Record
	runtime    *wgtunnel.Runtime
	version    int64
	runtimeID  string
	proxy      *url.URL
	key        string
	publicOnly bool
}

func (r *Route) Kind() Kind        { return r.record.Kind }
func (r *Route) NodeID() int64     { return r.record.NodeID }
func (r *Route) AccountID() int64  { return r.record.AccountID }
func (r *Route) Version() int64    { return r.version }
func (r *Route) RuntimeID() string { return r.runtimeID }
func (r *Route) PoolKey() string   { return r.key }
func (r *Route) EvidenceKey() string {
	if r.Kind() == WireGuard {
		return fmt.Sprintf("wireguard:%d:%d:%s", r.NodeID(), r.version, r.runtimeID)
	}
	if r.Kind() == Direct {
		return "direct:" + r.router.instance
	}
	digest := sha256.Sum256([]byte(r.record.ProxyURL))
	return fmt.Sprintf("proxy:%d:%x:%s", r.NodeID(), digest[:12], r.router.instance)
}

func (r *Router) Resolve(ctx context.Context, ref Reference) (*Route, error) {
	if !ref.Valid() {
		return nil, ErrBlocked
	}
	record, err := r.store.ResolveReference(ctx, ref)
	if err != nil {
		ScopeFromContext(ctx).Block()
		return nil, ErrBlocked
	}
	if err = ScopeFromContext(ctx).Select(record); err != nil {
		return nil, err
	}
	if (ref.AccountID > 0 && record.AccountID != ref.AccountID) || (ref.NodeID > 0 && record.NodeID != ref.NodeID) || (ref.ExplicitDirect && record.Kind != Direct) {
		return nil, ErrBlocked
	}
	route := &Route{router: r, ref: ref, record: record}
	switch record.Kind {
	case WireGuard:
		if record.NodeID <= 0 {
			return nil, ErrBlocked
		}
		route.runtime, route.version, route.runtimeID, err = r.manager.Acquire(ctx, record.NodeID)
		if err != nil {
			return nil, ErrBlocked
		}
	case Proxy:
		_, route.proxy, err = proxyurl.Parse(record.ProxyURL)
		if err != nil || route.proxy == nil {
			return nil, ErrBlocked
		}
	case Direct:
		if record.NodeID != 0 || record.ProxyURL != "" {
			return nil, ErrBlocked
		}
	default:
		return nil, ErrBlocked
	}
	if err := ScopeFromContext(ctx).Pin(route); err != nil {
		return nil, err
	}
	route.key = route.EvidenceKey() + ":account:" + strconv.FormatInt(record.AccountID, 10) + ":revision:" + strconv.FormatInt(record.Revision, 10)
	return route, nil
}

// Check pins the route to one attempt. A configuration or binding change fails
// the old attempt; it never selects the new route or a different account.
func (r *Route) Check(ctx context.Context) error {
	current, err := r.router.store.ResolveReference(ctx, r.ref)
	if err != nil || current != r.record {
		return ErrBlocked
	}
	if r.runtime != nil {
		runtime, version, id, err := r.router.manager.Acquire(ctx, r.NodeID())
		if err != nil || runtime != r.runtime || version != r.version || id != r.runtimeID {
			return ErrBlocked
		}
	}
	return nil
}
func (r *Route) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	started := time.Now()
	c, err := r.dialContext(ctx, network, address)
	if err != nil {
		r.confirmTunnelFailure(ctx, started)
		return nil, err
	}
	return r.guardConnection(c), nil
}

// Check the owning operation's context: an internal HTTP probe deadline must
// not be mistaken for the user canceling their request.
func (r *Route) confirmTunnelFailure(ctx context.Context, started time.Time) {
	if r.runtime != nil && ctx.Err() == nil && time.Since(started) >= 8*time.Second && !r.runtime.HandshakeRecent(time.Now()) {
		r.router.manager.Fail(r.NodeID(), r.Version())
	}
}

func (r *Route) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if err := r.Check(ctx); err != nil {
		return nil, err
	}
	if r.runtime != nil {
		if !r.publicOnly {
			return r.runtime.DialContext(ctx, network, address)
		}
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, ErrBlocked
		}
		ips, err := r.runtime.Lookup(ctx, host)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			if !publicAddress(ip) {
				return nil, ErrBlocked
			}
		}
		for _, ip := range ips {
			c, err := r.runtime.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return c, nil
			}
		}
		return nil, ErrBlocked
	}
	if r.Kind() != Direct {
		return nil, errors.New("proxy route requires its proxy transport")
	}
	return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, address)
}

// ConfigureHTTP must run before a transport is shared. A caller must additionally
// Check before every RoundTrip, since pooled connections bypass DialContext.
func (r *Route) ConfigureHTTP(tr *http.Transport) error {
	if tr == nil {
		return errors.New("HTTP transport is required")
	}
	tr.Proxy = nil
	tr.DialTLSContext = nil
	tr.DialContext = r.DialContext
	if r.Kind() == Proxy {
		tr.DialContext = (&net.Dialer{Timeout: 10 * time.Second}).DialContext
		return proxyutil.ConfigureTransportProxy(tr, r.proxy)
	}
	return nil
}
func (r *Route) ConfigureReq(c *req.Client) error {
	if c == nil {
		return errors.New("req client is required")
	}
	c.DisableHTTP3()
	if r.Kind() == Proxy {
		// Existing HTTP/SOCKS support is retained. Forced HTTP/2's independent
		// dial path is prohibited for proxies until it can honor CONNECT/SOCKS.
		c.GetTransport().DisableForceHttpVersion()
		c.SetProxyURL(r.record.ProxyURL)
	} else if err := wgtunnel.ConfigureReqClient(c, r.DialContext); err != nil {
		return err
	}
	c.GetTransport().WrapRoundTripFunc(func(next http.RoundTripper) req.HttpRoundTripFunc {
		return func(request *http.Request) (*http.Response, error) {
			if err := r.Check(request.Context()); err != nil {
				return nil, err
			}
			return r.observeRoundTrip(request, next)
		}
	})
	return nil
}

type checkedTransport struct {
	route *Route
	next  http.RoundTripper
}

func (r *Route) Wrap(next http.RoundTripper) http.RoundTripper { return &checkedTransport{r, next} }
func (t *checkedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if err := t.route.Check(request.Context()); err != nil {
		return nil, err
	}
	return t.route.observeRoundTrip(request, t.next)
}

func (r *Route) RequirePublicTargets() *Route {
	copy := *r
	copy.publicOnly = true
	copy.key += ":public"
	return &copy
}
func publicAddress(ip netip.Addr) bool {
	return ip.IsValid() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsMulticast() && !ip.IsUnspecified()
}
func (r *Route) ValidateHost(ctx context.Context, host string) error {
	if r.runtime == nil {
		return errors.New("route does not expose a tunnel resolver")
	}
	ips, err := r.runtime.Lookup(ctx, host)
	if err != nil {
		return err
	}
	for _, ip := range ips {
		if !publicAddress(ip) {
			return ErrBlocked
		}
	}
	return nil
}
func (t *checkedTransport) CloseIdleConnections() {
	if c, ok := t.next.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}

func (r *Route) CredentialParentID() int64 { return r.record.ParentAccountID }
