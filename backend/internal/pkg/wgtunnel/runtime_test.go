package wgtunnel

import (
	"context"
	"crypto/rand"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"testing"
	"time"

	"golang.org/x/crypto/curve25519"
)

func freeUDPPort(t *testing.T) int {
	t.Helper()
	s, e := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if e != nil {
		t.Fatal(e)
	}
	addr, ok := s.LocalAddr().(*net.UDPAddr)
	if !ok {
		t.Fatal("unexpected UDP local address")
	}
	p := addr.Port
	_ = s.Close()
	return p
}

func runtimePort(t *testing.T, r *Runtime) string {
	t.Helper()
	raw, e := r.dev.IpcGet()
	if e != nil {
		t.Fatal("cannot inspect test device")
	}
	// Never print the raw IPC response: it includes private keys.
	for _, line := range stringsSplit(raw) {
		if len(line) > 12 && line[:12] == "listen_port=" {
			return line[12:]
		}
	}
	t.Fatal("test device did not bind")
	return ""
}

func stringsSplit(s string) []string {
	var out []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return out
}

func pairedRuntimes(t *testing.T) (*Runtime, *Runtime) {
	t.Helper()
	var a, b Config
	if _, e := rand.Read(a.privateKey[:]); e != nil {
		t.Fatal(e)
	}
	if _, e := rand.Read(b.privateKey[:]); e != nil {
		t.Fatal(e)
	}
	pa, e := curve25519.X25519(a.privateKey[:], curve25519.Basepoint)
	if e != nil {
		t.Fatal(e)
	}
	pb, e := curve25519.X25519(b.privateKey[:], curve25519.Basepoint)
	if e != nil {
		t.Fatal(e)
	}
	copy(a.publicKey[:], pb)
	copy(b.publicKey[:], pa)
	a.Addresses = []netip.Addr{netip.MustParseAddr("10.77.0.2")}
	b.Addresses = []netip.Addr{netip.MustParseAddr("10.77.0.1")}
	a.DNS = b.Addresses
	b.DNS = a.Addresses
	a.MTU = 1280
	b.MTU = 1280
	a.AllowedIPs = []netip.Prefix{netip.MustParsePrefix("10.77.0.0/24")}
	b.AllowedIPs = a.AllowedIPs
	b.Endpoint = net.JoinHostPort("127.0.0.1", strconv.Itoa(freeUDPPort(t)))
	server, e := Start(context.Background(), b)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(server.Close)
	a.Endpoint = net.JoinHostPort("127.0.0.1", runtimePort(t, server))
	client, e := Start(context.Background(), a)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(client.Close)
	if e = server.dev.IpcSet("public_key=" + hexKey(b.publicKey) + "\nendpoint=127.0.0.1:" + runtimePort(t, client) + "\n"); e != nil {
		t.Fatal("could not pair test peers")
	}
	return client, server
}

func TestRealUserspaceTunnelAndRevocation(t *testing.T) {
	client, server := pairedRuntimes(t)
	l, e := server.network.ListenTCPAddrPort(netip.MustParseAddrPort("10.77.0.1:8080"))
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = l.Close() }()
	h := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("tunneled")) })}
	defer func() { _ = h.Close() }()
	go func() { _ = h.Serve(l) }()
	tr := &http.Transport{DialContext: client.DialContext}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr, Timeout: 5 * time.Second}
	resp, e := hc.Get("http://10.77.0.1:8080")
	if e != nil {
		t.Fatal(e)
	}
	data, e := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if e != nil || string(data) != "tunneled" {
		t.Fatal("unexpected tunnel response")
	}
	s := client.Counts.Snapshot()
	if s.TX == 0 || s.RX == 0 {
		t.Fatal("missing UDP accounting")
	}
	// No public route exists in this configuration; rejection precedes dialing.
	if _, e = client.DialContext(context.Background(), "tcp", "192.0.2.1:80"); e == nil {
		t.Fatal("uncovered route accepted")
	}
	client.Close()
	if _, e = hc.Get("http://10.77.0.1:8080"); e == nil {
		t.Fatal("revoked pooled connection survived")
	}
}

func hexKey(k [32]byte) string {
	const digits = "0123456789abcdef"
	b := make([]byte, 64)
	for i, v := range k {
		b[i*2] = digits[v>>4]
		b[i*2+1] = digits[v&15]
	}
	return string(b)
}
