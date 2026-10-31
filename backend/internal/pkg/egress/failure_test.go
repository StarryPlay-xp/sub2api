package egress

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
)

type failureStore struct{ node wgtunnel.Node }

func (s *failureStore) LoadNode(context.Context, int64) (wgtunnel.Node, error)        { return s.node, nil }
func (*failureStore) SaveCheckpoint(context.Context, wgtunnel.Checkpoint, bool) error { return nil }
func (*failureStore) ResolveReference(context.Context, Reference) (Record, error) {
	return Record{Kind: WireGuard, NodeID: 1}, nil
}
func TestProbeInternalDeadlineLatchesFailureButUserCancellationDoesNot(t *testing.T) {
	box, _ := wgtunnel.NewSecretBox([]byte(strings.Repeat("k", 32)))
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	raw := "[Interface]\nPrivateKey = " + key + "\nAddress = 10.253.240.2/32\nDNS = 10.253.240.1\n[Peer]\nPublicKey = " + key + "\nEndpoint = 127.0.0.1:9\nAllowedIPs = 0.0.0.0/0\n"
	cipher, err := box.Seal(1, raw)
	if err != nil {
		t.Fatal(err)
	}
	store := &failureStore{node: wgtunnel.Node{ID: 1, Version: 1, Ciphertext: cipher, Active: true}}
	manager := wgtunnel.NewManager(store, box)
	defer manager.Close()
	router, _ := NewRouter(store, manager)
	route, err := router.Resolve(context.Background(), Reference{NodeID: 1})
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	route.confirmTunnelFailure(canceled, time.Now().Add(-10*time.Second))
	if err = route.Check(context.Background()); err != nil {
		t.Fatal("user cancellation paused a shared tunnel", err)
	}
	observer := NewObserver(router, nil)
	defer observer.Close()
	if _, err = observer.ProbeNow(context.Background(), route, "ipv4"); err == nil {
		t.Fatal("unreachable test endpoint succeeded")
	}
	if err = route.Check(context.Background()); !errors.Is(err, ErrBlocked) {
		t.Fatal("internal probe deadline did not latch failure", err)
	}
}
