package traffic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

func TestTrafficWebSocketCountsUncompressedPayloadBothDirections(t *testing.T) {
	now := time.Now()
	m := NewMeter("ws", now)
	done := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, e := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		if e != nil {
			done <- e
			return
		}
		defer func() { _ = c.CloseNow() }()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		kind, p, e := ReadWS(ctx, c, m, ClientIn)
		if e == nil {
			e = WriteWS(ctx, c, m, ClientOut, kind, p)
		}
		done <- e
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), &coderws.DialOptions{CompressionMode: coderws.CompressionContextTakeover})
	require.NoError(t, err)
	defer func() { _ = conn.CloseNow() }()
	p := []byte(strings.Repeat("a", 12000))
	require.NoError(t, WriteWS(ctx, conn, m, UpstreamOut, coderws.MessageText, p))
	_, got, err := ReadWS(ctx, conn, m, UpstreamIn)
	require.NoError(t, err)
	require.Equal(t, p, got)
	require.NoError(t, <-done)
	m.Sample(now.Add(time.Second))
	for _, b := range m.Snapshot(false).Buckets {
		require.Equal(t, [7]uint64{12000, 12000, 12000, 12000, 24000, 24000, 48000}, b.Bytes.Vector())
	}
}
