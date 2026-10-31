//go:build unit

package service

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/egress"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
	coderws "github.com/coder/websocket"
)

type wsAttemptStore struct {
	mu       sync.Mutex
	attempts []egress.Attempt
}

func (s *wsAttemptStore) SaveAttempt(_ context.Context, a egress.Attempt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts = append(s.attempts, a)
	return nil
}
func TestWebSocketTypedRoutePreservesUpgradeAndActualConnectionEvidence(t *testing.T) {
	store := &previewRouteStore{}
	manager := wgtunnel.NewManager(store, nil)
	defer manager.Close()
	router, _ := egress.NewRouter(store, manager)
	observer := egress.NewObserver(router, nil)
	observer.Close() // Do not probe public services in this local test.
	attempts := &wsAttemptStore{}
	recorder := egress.NewRecorder(attempts)
	defer recorder.Close()
	router.SetTelemetry(observer, recorder)
	route, err := router.Resolve(context.Background(), egress.Reference{ExplicitDirect: true})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, e := coderws.Accept(w, r, nil)
		if e != nil {
			return
		}
		defer ws.CloseNow()
		kind, payload, e := ws.Read(r.Context())
		if e == nil {
			_ = ws.Write(r.Context(), kind, payload)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(egress.WithRoute(context.Background(), route), 5*time.Second)
	defer cancel()
	conn, _, _, err := newDefaultOpenAIWSClientDialer().Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil, "http://127.0.0.1:9")
	if err != nil {
		t.Fatal("typed route did not override legacy proxy or lost upgrade", err)
	}
	defer conn.Close()
	if err = conn.WriteJSON(ctx, map[string]string{"type": "synthetic"}); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.ReadMessage(ctx); err != nil {
		t.Fatal(err)
	}
	remote := conn.(interface{ EgressRemoteAddr() net.Addr }).EgressRemoteAddr()
	if remote == nil {
		t.Fatal("actual WS connection address missing")
	}
	route.RecordConnectionAttempt(ctx, remote, true, "websocket_acquired")
	recorder.Close()
	if len(attempts.attempts) != 2 {
		t.Fatalf("expected handshake and reuse evidence, got %d", len(attempts.attempts))
	}
	if attempts.attempts[0].Snapshot.Family != "ipv4" || !attempts.attempts[1].Snapshot.ConnectionReused {
		t.Fatal("WS evidence lost family/reuse")
	}
}
