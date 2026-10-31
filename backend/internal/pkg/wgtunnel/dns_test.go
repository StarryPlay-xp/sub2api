package wgtunnel

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

func TestDNSRemainsInsideTunnel(t *testing.T) {
	client, server := pairedRuntimes(t)
	dns, err := server.network.ListenUDPAddrPort(netip.MustParseAddrPort("10.77.0.1:53"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dns.Close() })
	var queries atomic.Int32
	go func() {
		buffer := make([]byte, 2048)
		for {
			n, addr, err := dns.ReadFrom(buffer)
			if err != nil {
				return
			}
			var message dnsmessage.Message
			if message.Unpack(buffer[:n]) != nil || len(message.Questions) != 1 {
				continue
			}
			queries.Add(1)
			question := message.Questions[0]
			answer := dnsmessage.Message{Header: dnsmessage.Header{ID: message.ID, Response: true, RecursionDesired: true, RecursionAvailable: true}, Questions: message.Questions}
			if question.Type == dnsmessage.TypeA {
				answer.Answers = []dnsmessage.Resource{{Header: dnsmessage.ResourceHeader{Name: question.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 60}, Body: &dnsmessage.AResource{A: [4]byte{10, 77, 0, 1}}}}
			}
			packet, err := answer.Pack()
			if err == nil {
				dns.WriteTo(packet, addr)
			}
		}
	}()
	// A literal Endpoint must not bootstrap DNS. Upstream DNS must never use the
	// process resolver, even if the host has a usable direct connection.
	old := net.DefaultResolver
	var hostQueries atomic.Int32
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(context.Context, string, string) (net.Conn, error) {
		hostQueries.Add(1)
		return nil, errors.New("host DNS forbidden")
	}}
	t.Cleanup(func() { net.DefaultResolver = old })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ips, err := client.Lookup(ctx, "wg-target.invalid")
	if err != nil || len(ips) != 1 || ips[0].String() != "10.77.0.1" {
		t.Fatal("tunneled DNS lookup failed", err)
	}
	if queries.Load() == 0 || hostQueries.Load() != 0 {
		t.Fatal("DNS used the wrong network")
	}
	if _, err = client.DialContext(ctx, "tcp6", "wg-target.invalid:443"); err == nil {
		t.Fatal("missing IPv6 route accepted")
	}
}
