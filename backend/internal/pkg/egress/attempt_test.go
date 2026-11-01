package egress

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

type attemptMemoryStore struct {
	mu       sync.Mutex
	attempts []Attempt
}

func (s *attemptMemoryStore) SaveAttempt(_ context.Context, a Attempt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts = append(s.attempts, a)
	return nil
}

type attemptRoundTripper func(*http.Request) (*http.Response, error)

func (f attemptRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type attemptConn struct {
	net.Conn
	remote net.Addr
}

func (c attemptConn) RemoteAddr() net.Addr { return c.remote }

func TestInternalConnectionRetryKeepsSeparateImmutableEvidence(t *testing.T) {
	router, _ := testRouter(t, Record{Kind: Direct, AccountID: 1, Revision: 1})
	route, err := router.Resolve(context.Background(), Reference{AccountID: 1})
	if err != nil {
		t.Fatal(err)
	}
	observer := NewObserver(router, nil)
	observer.Close() // Keep this test entirely offline.
	store := &attemptMemoryStore{}
	recorder := NewRecorder(store)
	router.SetTelemetry(observer, recorder)
	ctx := context.WithValue(context.Background(), ctxkey.ClientRequestID, "client-id")
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://example.invalid", nil)
	res, err := route.observeRoundTrip(req, attemptRoundTripper(func(req *http.Request) (*http.Response, error) {
		trace := httptrace.ContextClientTrace(req.Context())
		trace.GotConn(httptrace.GotConnInfo{Conn: attemptConn{remote: &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 443}}, Reused: true})
		trace.GotConn(httptrace.GotConnInfo{Conn: attemptConn{remote: &net.TCPAddr{IP: net.ParseIP("2001:db8::1"), Port: 443}}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, res.Body); err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	recorder.Close()
	if len(store.attempts) != 2 {
		t.Fatalf("got %d attempts", len(store.attempts))
	}
	first, last := store.attempts[0], store.attempts[1]
	if first.ID == last.ID || first.Outcome != "connection_retry" || last.Outcome != "complete" {
		t.Fatal("retry replaced the first attempt")
	}
	if first.Snapshot.Family != "ipv4" || !first.Snapshot.ConnectionReused || last.Snapshot.Family != "ipv6" || last.Snapshot.ConnectionReused {
		t.Fatal("connection metadata was overwritten")
	}
	if first.RequestID != "client-id" || last.RequestID != "client-id" {
		t.Fatal("lost request association")
	}
}
