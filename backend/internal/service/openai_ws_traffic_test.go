package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWSTrafficCountsFailedUpgradeBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("unavailable"))
	}))
	defer server.Close()
	now := time.Now()
	meter := traffic.NewMeter("upgrade", now)
	dialer := newDefaultOpenAIWSClientDialer()
	_, status, _, err := dialer.Dial(traffic.WithMeter(context.Background(), meter), "ws"+strings.TrimPrefix(server.URL, "http"), nil, "")
	require.Error(t, err)
	require.Equal(t, http.StatusServiceUnavailable, status)
	meter.Sample(now.Add(time.Second))
	require.Equal(t, float64(len("unavailable")), meter.Current().Rates["upstream_in"])
}

func TestOpenAIWSTrafficRetainsMeterForBackgroundConnectionIO(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = c.CloseNow() }()
		kind, payload, err := c.Read(r.Context())
		if err == nil {
			_ = c.Write(r.Context(), kind, payload)
		}
	}))
	defer server.Close()
	now := time.Now()
	meter := traffic.NewMeter("pooled", now)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, _, err := newDefaultOpenAIWSClientDialer().Dial(traffic.WithMeter(ctx, meter), "ws"+strings.TrimPrefix(server.URL, "http"), nil, "")
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	// A pooled reader or prewarmed connection uses a new context without the meter.
	require.NoError(t, conn.WriteJSON(ctx, map[string]string{"type": "test"}))
	payload, err := conn.ReadMessage(ctx)
	require.NoError(t, err)
	meter.Sample(now.Add(time.Second))
	require.Equal(t, float64(len(payload)), meter.Current().Rates["upstream_in"])
	require.Equal(t, float64(len(payload)), meter.Current().Rates["upstream_out"])
}
