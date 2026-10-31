package wgtunnel

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"
)

func TestCloseCancelsPendingDNS(t *testing.T) {
	for _, dial := range []bool{false, true} {
		name := "lookup"
		if dial {
			name = "dial"
		}
		t.Run(name, func(t *testing.T) {
			client, server := pairedRuntimes(t)
			dns, err := server.network.ListenUDPAddrPort(netip.MustParseAddrPort("10.77.0.1:53"))
			if err != nil {
				t.Fatal(err)
			}
			defer dns.Close()
			// Receive but never answer: shutdown must cancel real socket I/O,
			// rather than wait for the normal DNS deadline to expire.
			if err := dns.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
				t.Fatal(err)
			}
			operation := make(chan error, 1)
			go func() {
				if dial {
					conn, err := client.DialContext(context.Background(), "tcp", "pending.invalid:443")
					if conn != nil {
						conn.Close()
					}
					operation <- err
				} else {
					_, err := client.Lookup(context.Background(), "pending.invalid")
					operation <- err
				}
			}()
			if _, _, err := dns.ReadFrom(make([]byte, 2048)); err != nil {
				t.Fatal("DNS did not enter the tunnel:", err)
			}
			closed := make(chan struct{})
			go func() { client.Close(); close(closed) }()
			select {
			case <-closed:
			case <-time.After(2 * time.Second):
				t.Fatal("close waited for the DNS deadline instead of canceling I/O")
			}
			if err := <-operation; !errors.Is(err, ErrBlocked) {
				t.Fatalf("pending operation was not revoked: %v", err)
			}
			final := client.Counts.Snapshot()
			if _, err := client.Lookup(context.Background(), "pending.invalid"); !errors.Is(err, ErrBlocked) {
				t.Fatalf("lookup accepted after close: %v", err)
			}
			if final != client.Counts.Snapshot() {
				t.Fatal("closed lookup emitted traffic")
			}
		})
	}
}
