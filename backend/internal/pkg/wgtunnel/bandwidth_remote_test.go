package wgtunnel

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/stretchr/testify/require"
)

// Explicit opt-in against an authorized, bounded synthetic upload/download
// fixture. No vendor credentials, production peer or public exit probe is used.
func TestAuthorizedRemoteBandwidth(t *testing.T) {
	path, base := os.Getenv("SUB2API_WG_BANDWIDTH_TEST_CONFIG"), os.Getenv("SUB2API_WG_BANDWIDTH_TEST_URL")
	if path == "" || base == "" {
		t.Skip("authorized bandwidth fixture not supplied")
	}
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Zero(t, info.Mode().Perm()&0077)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	cfg, err := Parse(string(raw))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	r, err := Start(ctx, cfg)
	require.NoError(t, err)
	defer r.Close()
	now := time.Now()
	meter := NewMeter(&r.Counts, now)
	application := traffic.NewMeter("remote-bandwidth-test", now)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				at := time.Now()
				_, _ = meter.Sample(at)
				application.Sample(at)
			case <-stop:
				return
			}
		}
	}()
	defer func() { close(stop); wg.Wait() }()
	tr := &http.Transport{DialContext: r.DialContext, Proxy: nil}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 35 * time.Second}
	requestCtx := traffic.WithMeter(ctx, application)
	req, err := http.NewRequestWithContext(requestCtx, "GET", strings.TrimRight(base, "/")+"/download", nil)
	require.NoError(t, err)
	resp, err := traffic.Do(client, req)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	n, err := io.Copy(io.Discard, io.LimitReader(resp.Body, 5<<20))
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, int64(4<<20), n)
	req, err = http.NewRequestWithContext(requestCtx, "POST", strings.TrimRight(base, "/")+"/upload", bytes.NewReader(bytes.Repeat([]byte{'x'}, 4<<20)))
	require.NoError(t, err)
	resp, err = traffic.Do(client, req)
	require.NoError(t, err)
	require.Equal(t, 204, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	// Wait for one normal sampling tick after transfers (not a synthetic rate).
	select {
	case <-time.After(1100 * time.Millisecond):
	case <-ctx.Done():
		t.Fatal("fixture deadline")
	}
	cp, gap := meter.Checkpoint(1, "remote", 1, false)
	require.False(t, gap)
	require.GreaterOrEqual(t, cp.TX, uint64(4<<20))
	require.GreaterOrEqual(t, cp.RX, uint64(4<<20))
	var peaks traffic.PeakHistory
	for _, b := range cp.Buckets {
		peaks.Merge(b.PeakHistory)
	}
	require.Greater(t, peaks.Peaks["tx"].BytesPerSecond, 0.0)
	require.Greater(t, peaks.Peaks["rx"].BytesPerSecond, 0.0)
	require.Greater(t, peaks.Quality.ValidSamples, uint64(1))
	snapshot := application.Snapshot(false)
	for _, b := range snapshot.Buckets {
		if b.Interval == "hour" {
			require.Equal(t, uint64(4<<20), b.Bytes.UpstreamOut)
			require.Equal(t, uint64(4<<20), b.Bytes.UpstreamIn)
		}
	}
	t.Logf("Synthetic transfer verified: application upload/download = 4 MiB each; WG tx=%d rx=%d; 1-second peaks tx=%.0f B/s rx=%.0f B/s; valid samples=%d", cp.TX, cp.RX, peaks.Peaks["tx"].BytesPerSecond, peaks.Peaks["rx"].BytesPerSecond, peaks.Quality.ValidSamples)
}
