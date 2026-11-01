//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/egress"
	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

type trafficDirectRouteStore struct{}

func (trafficDirectRouteStore) ResolveReference(_ context.Context, r egress.Reference) (egress.Record, error) {
	return egress.Record{Kind: egress.Direct, AccountID: r.AccountID}, nil
}
func (trafficDirectRouteStore) LoadNode(context.Context, int64) (wgtunnel.Node, error) {
	return wgtunnel.Node{}, egress.ErrBlocked
}
func (trafficDirectRouteStore) SaveCheckpoint(context.Context, wgtunnel.Checkpoint, bool) error {
	return nil
}
func trafficTestRoute(t *testing.T) *egress.Route {
	t.Helper()
	store := trafficDirectRouteStore{}
	manager := wgtunnel.NewManager(store, nil)
	t.Cleanup(manager.Close)
	router, err := egress.NewRouter(store, manager)
	require.NoError(t, err)
	route, err := router.Resolve(context.Background(), egress.Reference{AccountID: 123})
	require.NoError(t, err)
	return route
}
func trafficRouteBucket(t *testing.T, m *traffic.Meter, account int64) traffic.EgressBucket {
	t.Helper()
	for _, b := range m.Snapshot(false).Egress {
		if b.Type == "direct" && b.AccountID == account {
			return b
		}
	}
	t.Fatal("missing route")
	return traffic.EgressBucket{}
}
func TestTrafficEgressWebSocketPoolRealReuseAndIdle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := coderws.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer ws.CloseNow()
		for {
			kind, data, e := ws.Read(r.Context())
			if e != nil {
				return
			}
			if ws.Write(r.Context(), kind, data) != nil {
				return
			}
		}
	}))
	defer server.Close()
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	pool := newOpenAIWSConnPool(cfg)
	defer pool.Close()
	now := time.Now().UTC().Truncate(time.Hour)
	root := traffic.NewMeter("ws-egress", now)
	route := trafficTestRoute(t)
	request := openAIWSAcquireRequest{Account: &Account{ID: 123, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, WSURL: "ws" + strings.TrimPrefix(server.URL, "http"), Route: route}
	var connectionID string
	for i := int64(1); i <= 2; i++ {
		user := root.Scope()
		user.Bind(i, 10+i, true)
		ctx, cancel := context.WithTimeout(traffic.WithMeter(egress.WithRoute(context.Background(), route), user), 5*time.Second)
		lease, err := pool.Acquire(ctx, request)
		require.NoError(t, err)
		if i == 1 {
			connectionID = lease.ConnID()
		} else {
			require.Equal(t, connectionID, lease.ConnID())
		}
		// Exercise coder conn's original handle, not a newly attached test handle.
		require.NoError(t, lease.WriteJSONContext(ctx, map[string]string{"type": "traffic"}))
		data, err := lease.ReadMessageContext(ctx)
		require.NoError(t, err)
		require.Contains(t, string(data), "traffic")
		handle := lease.conn.trafficMeter
		lease.Release()
		user.CloseScope()
		cancel()
		handle.Add(traffic.UpstreamIn, 7) // an idle message retains the physical route
	}
	root.Sample(now.Add(time.Second))
	routeBucket := trafficRouteBucket(t, root, 0)
	accountBucket := trafficRouteBucket(t, root, 123)
	require.Equal(t, uint64(2), routeBucket.Attempts)
	require.Equal(t, routeBucket.Bytes, accountBucket.Bytes)
	var host, userTotal uint64
	for _, b := range root.Snapshot(false).Subjects {
		if b.Type == "api_key" {
			require.Positive(t, b.Bytes.UpstreamOut)
			require.Equal(t, b.Bytes.UpstreamOut, b.Bytes.UpstreamIn)
			userTotal += b.Bytes.UpstreamOut + b.Bytes.UpstreamIn
		}
		if b.Type == "unattributed" {
			host = b.Bytes.UpstreamIn
		}
	}
	require.Equal(t, uint64(14), host)
	require.Equal(t, userTotal+14, routeBucket.Bytes.Total())
}
func TestTrafficEgressWebSocketNonPooledFailureAndPrewarm(t *testing.T) {
	route := trafficTestRoute(t)
	now := time.Now().UTC().Truncate(time.Hour)
	root := traffic.NewMeter("ws-failure", now)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("failure"))
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(traffic.WithMeter(egress.WithRoute(context.Background(), route), root), 5*time.Second)
	defer cancel()
	conn, status, _, err := newDefaultOpenAIWSClientDialer().Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil, "")
	require.Error(t, err)
	require.Nil(t, conn)
	require.Equal(t, http.StatusBadGateway, status)
	root.Sample(now.Add(time.Second))
	b := trafficRouteBucket(t, root, 0)
	require.Equal(t, uint64(1), b.Attempts)
	require.Equal(t, uint64(7), b.Bytes.In)
	pool := newOpenAIWSConnPool(&config.Config{})
	defer pool.Close()
	pool.setClientDialerForTest(&openAIWSCountingDialer{})
	// Prewarming starts from the root meter, outside any user request.
	request := openAIWSAcquireRequest{Account: &Account{ID: 123}, WSURL: "wss://example.invalid", Route: route, TrafficMeter: root}
	warm, err := pool.dialConn(context.Background(), request)
	require.NoError(t, err)
	warm.trafficMeter.Add(traffic.UpstreamIn, 9)
	warm.close()
	root.Sample(now.Add(2 * time.Second))
	b = trafficRouteBucket(t, root, 0)
	require.Equal(t, uint64(2), b.Attempts)
	require.Equal(t, uint64(16), b.Bytes.In)
}

type trafficImmediateCloseWS struct {
	openAIWSFakeConn
	done chan struct{}
}

func (*trafficImmediateCloseWS) RequiresReaderLoop() bool                    { return true }
func (*trafficImmediateCloseWS) ReadMessage(context.Context) ([]byte, error) { return nil, io.EOF }
func (c *trafficImmediateCloseWS) Close() error                              { close(c.done); return nil }
func TestTrafficEgressWebSocketImmediatePeerCloseReleasesIdle(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	root := traffic.NewMeter("immediate-close", now)
	for i := 0; i < 100; i++ {
		idle := root.EgressIdleScope(traffic.EgressIdentity{Type: "direct", AccountID: 123})
		ws := &trafficImmediateCloseWS{done: make(chan struct{})}
		conn := newOpenAIWSConnWithTraffic("immediate-close", ws, nil, idle.Switch(), idle)
		select {
		case <-ws.done:
		case <-time.After(time.Second):
			t.Fatal("reader did not close connection")
		}
		conn.close() // synchronize completion of closeOnce, including idle scope cleanup
	}
	root.Sample(now.Add(time.Second))
	root.Ack(root.Snapshot(false))
	root.Sample(now.Add(64 * time.Second))
	for _, bucket := range root.Snapshot(false).Egress {
		require.Equal(t, "unidentified", bucket.Type, "closed idle scope must not remain active")
	}
}
