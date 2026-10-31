package wgtunnel

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

var ErrBlocked = errors.New("WireGuard route unavailable; direct fallback prohibited")

type Runtime struct {
	cfg         Config
	dev         *device.Device
	network     *netstack.Net
	Counts      Counters
	mu          sync.Mutex
	closed      bool
	connections map[*trackedConn]struct{}
	ctx         context.Context
	cancel      context.CancelFunc
}

// Start resolves only the tunnel endpoint on the host network. All target/DNS
// traffic is subsequently confined to the in-process stack.
func Start(ctx context.Context, cfg Config) (*Runtime, error) {
	if ctx.Err() != nil {
		return nil, ErrBlocked
	}
	host, port, _ := net.SplitHostPort(cfg.Endpoint)
	ip, err := netip.ParseAddr(host)
	if err != nil {
		lookup, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		ips, e := net.DefaultResolver.LookupNetIP(lookup, "ip", host)
		if e != nil || len(ips) == 0 {
			return nil, errors.New("WireGuard endpoint resolution failed")
		}
		ip = ips[0].Unmap()
	}
	if !ip.IsValid() || ip.IsUnspecified() || ip.IsMulticast() {
		return nil, errors.New("invalid WireGuard endpoint address")
	}
	tun, network, err := netstack.CreateNetTUN(cfg.Addresses, cfg.DNS, cfg.MTU)
	if err != nil {
		return nil, errors.New("WireGuard network creation failed")
	}
	r := &Runtime{cfg: cfg, network: network, connections: make(map[*trackedConn]struct{})}
	r.ctx, r.cancel = context.WithCancel(context.Background())
	family := "udp4"
	if ip.Is6() {
		family = "udp6"
	}
	// Library log messages are intentionally discarded: configuration errors
	// must not expose IPC payloads, keys, peer identifiers or endpoint details.
	r.dev = device.NewDevice(tun, NewMeteredBind(family, &r.Counts), device.NewLogger(device.LogLevelSilent, ""))
	var ipc strings.Builder
	fmt.Fprintf(&ipc, "private_key=%s\npublic_key=%s\nendpoint=%s\npersistent_keepalive_interval=%d\n", hex.EncodeToString(cfg.privateKey[:]), hex.EncodeToString(cfg.publicKey[:]), net.JoinHostPort(ip.String(), port), cfg.Keepalive)
	if cfg.HasPresharedKey {
		fmt.Fprintf(&ipc, "preshared_key=%s\n", hex.EncodeToString(cfg.presharedKey[:]))
	}
	for _, p := range cfg.AllowedIPs {
		fmt.Fprintf(&ipc, "allowed_ip=%s\n", p)
	}
	if err = r.dev.IpcSet(ipc.String()); err != nil {
		r.Close()
		return nil, errors.New("WireGuard device configuration failed")
	}
	if err = r.dev.Up(); err != nil {
		r.Close()
		return nil, errors.New("WireGuard UDP endpoint could not be opened")
	}
	return r, nil
}

func (r *Runtime) Lookup(ctx context.Context, host string) ([]netip.Addr, error) {
	if r.ctx.Err() != nil {
		return nil, ErrBlocked
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	stop := context.AfterFunc(r.ctx, cancel)
	defer stop()
	if ip, err := netip.ParseAddr(host); err == nil {
		if r.cfg.Allows(ip) {
			return []netip.Addr{ip}, nil
		}
		return nil, ErrBlocked
	}
	values, err := r.network.LookupContextHost(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("WireGuard DNS failed: %w", ErrBlocked)
	}
	var result []netip.Addr
	for _, v := range values {
		a, e := netip.ParseAddr(v)
		if e == nil && r.cfg.Allows(a) {
			result = append(result, a)
		}
	}
	if len(result) == 0 {
		return nil, ErrBlocked
	}
	return result, nil
}

func (r *Runtime) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, errors.New("unsupported WireGuard application network")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	stop := context.AfterFunc(r.ctx, cancel)
	defer stop()
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, errors.New("invalid upstream address")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return nil, errors.New("invalid upstream port")
	}
	addresses, err := r.Lookup(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, a := range addresses {
		if network == "tcp4" && !a.Is4() || network == "tcp6" && !a.Is6() {
			continue
		}
		c, e := r.network.DialContextTCPAddrPort(ctx, netip.AddrPortFrom(a, uint16(n)))
		if e != nil {
			continue
		}
		t := &trackedConn{Conn: c, owner: r}
		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			c.Close()
			return nil, ErrBlocked
		}
		r.connections[t] = struct{}{}
		r.mu.Unlock()
		return t, nil
	}
	return nil, ErrBlocked
}

func (r *Runtime) Close() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	r.cancel()
	connections := make([]*trackedConn, 0, len(r.connections))
	for c := range r.connections {
		connections = append(connections, c)
	}
	r.mu.Unlock()
	for _, c := range connections {
		c.Close()
	}
	r.dev.Close()
}

type trackedConn struct {
	net.Conn
	owner *Runtime
	once  sync.Once
}

func (c *trackedConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { c.owner.mu.Lock(); delete(c.owner.connections, c); c.owner.mu.Unlock() })
	return err
}

// HandshakeRecent exposes only peer liveness, never the IPC configuration.
func (r *Runtime) HandshakeRecent(now time.Time) bool {
	state, err := r.dev.IpcGet()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(state, "\n") {
		if raw, ok := strings.CutPrefix(line, "last_handshake_time_sec="); ok {
			seconds, err := strconv.ParseInt(raw, 10, 64)
			return err == nil && seconds > 0 && now.Sub(time.Unix(seconds, 0)) < 3*time.Minute
		}
	}
	return false
}
