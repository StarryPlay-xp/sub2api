package wgtunnel

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

// Evidence is an immutable observation, never a claim that a different
// destination necessarily sees the same source IP.
type Evidence struct {
	IP           string    `json:"ip,omitempty"`
	Source       string    `json:"source"`
	Family       string    `json:"family"`
	ObservedAt   time.Time `json:"observed_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	Route        string    `json:"route"`
	NodeID       int64     `json:"node_id,omitempty"`
	Version      int64     `json:"version,omitempty"`
	RuntimeID    string    `json:"runtime_id,omitempty"`
	ProbeService string    `json:"probe_service"`
	State        string    `json:"state"`
}

type DialContext func(context.Context, string, string) (net.Conn, error)

// Probe is deliberately credential-free and uses a fresh, family-specific
// connection. The supplied dialer is the only possible network path.
func Probe(ctx context.Context, dial DialContext, family string) (Evidence, error) {
	if dial == nil {
		return Evidence{}, errors.New("exit verification requires an explicit route")
	}
	return ProbeWithService(ctx, dial, family, "ipify")
}

// ProbeWithService accepts only fixed HTTPS echo services, never arbitrary URLs.
func ProbeWithService(ctx context.Context, dial DialContext, family, service string) (Evidence, error) {
	if dial == nil {
		return Evidence{}, errors.New("exit verification requires an explicit route")
	}
	target := "https://api.ipify.org?format=json"
	network := "tcp4"
	if family == "ipv6" {
		target = "https://api6.ipify.org?format=json"
		network = "tcp6"
	} else if family != "ipv4" {
		return Evidence{}, errors.New("unsupported verification address family")
	}
	if service == "icanhazip" {
		target = "https://icanhazip.com"
	} else if service != "ipify" {
		return Evidence{}, errors.New("unsupported verification service")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	tr := &http.Transport{Proxy: nil, DisableKeepAlives: true, TLSHandshakeTimeout: 5 * time.Second, DialContext: func(ctx context.Context, _ string, addr string) (net.Conn, error) { return dial(ctx, network, addr) }}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("exit verification redirects are prohibited")
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return Evidence{}, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return Evidence{}, errors.New("exit verification failed on selected route")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Evidence{}, errors.New("exit verification service returned an error")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4097))
	if err != nil || len(data) > 4096 {
		return Evidence{}, errors.New("invalid exit verification response")
	}
	var body struct {
		IP string `json:"ip"`
	}
	if service == "icanhazip" {
		body.IP = strings.TrimSpace(string(data))
	} else if json.Unmarshal(data, &body) != nil {
		return Evidence{}, errors.New("invalid exit verification response")
	}
	ip, err := netip.ParseAddr(body.IP)
	if err != nil || ip.IsPrivate() || ip.IsLoopback() || !ip.IsGlobalUnicast() || ip.Is4In6() || ((family == "ipv4") != ip.Is4()) {
		return Evidence{}, errors.New("verification did not return a matching public address")
	}
	now := time.Now().UTC()
	return Evidence{IP: ip.String(), Source: "route_probe", Family: family, ObservedAt: now, ExpiresAt: now.Add(10 * time.Minute), ProbeService: service, State: "observed"}, nil
}
