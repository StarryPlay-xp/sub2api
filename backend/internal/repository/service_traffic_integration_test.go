//go:build integration

package repository

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
	"github.com/stretchr/testify/require"
)

func TestServiceTrafficPersistenceRetryRestartAndPeak(t *testing.T) {
	db := wireguardTestDB(t)
	s := &ServiceTrafficStore{DB: db}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Hour).Add(time.Minute)
	m := traffic.NewMeter("first", now)
	m.Add(traffic.ClientIn, 100)
	m.Add(traffic.UpstreamIn, 200)
	m.Sample(now.Add(time.Second))
	old := m.Snapshot(false)
	require.NoError(t, s.Save(ctx, old))
	require.NoError(t, s.Save(ctx, old))
	m.Add(traffic.ClientOut, 500)
	m.Sample(now.Add(2 * time.Second))
	latest := m.Snapshot(true)
	require.NoError(t, s.Save(ctx, latest))
	require.NoError(t, s.Save(ctx, old))
	m2 := traffic.NewMeter("restart", now.Add(3*time.Second))
	m2.Add(traffic.UpstreamOut, 40)
	m2.Sample(now.Add(4 * time.Second))
	require.NoError(t, s.Save(ctx, m2.Snapshot(true)))
	for _, interval := range []string{"minute", "hour"} {
		w := wgtunnel.TrafficWindow{Interval: interval, Start: now.Truncate(time.Hour), End: now.Add(time.Hour)}
		r, err := s.Report(ctx, w)
		require.NoError(t, err)
		require.Equal(t, "840", r.TotalBytes)
		require.Equal(t, "300", r.InBytes)
		require.Equal(t, "540", r.OutBytes)
		require.Equal(t, 500.0, r.Peaks["total"].BytesPerSecond)
		require.Equal(t, uint64(3), r.Quality.ValidSamples)
		require.False(t, r.ConcurrentRuntimes)
		require.False(t, r.Incomplete)
	}
	// A concurrent replica cannot fabricate an aggregate peak by adding maxima.
	other := traffic.NewMeter("overlap", now)
	other.Add(traffic.ClientOut, 60)
	other.Sample(now.Add(time.Second))
	require.NoError(t, s.Save(ctx, other.Snapshot(true)))
	r, err := s.Report(ctx, wgtunnel.TrafficWindow{Interval: "hour", Start: now.Truncate(time.Hour), End: now.Add(time.Hour)})
	require.NoError(t, err)
	require.True(t, r.ConcurrentRuntimes)
	require.Nil(t, r.Peaks)
	require.True(t, r.Incomplete)
	require.Equal(t, "900", r.TotalBytes)
}
func TestServiceTrafficRetentionAndEmptyWindow(t *testing.T) {
	db := wireguardTestDB(t)
	s := &ServiceTrafficStore{DB: db}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Hour)
	for _, tc := range []struct {
		id  string
		age time.Duration
	}{{"minute-expired", 8 * 24 * time.Hour}, {"all-expired", 91 * 24 * time.Hour}} {
		m := traffic.NewMeter(tc.id, now.Add(-tc.age))
		m.Add(traffic.ClientIn, 5)
		m.Sample(now.Add(-tc.age).Add(time.Second))
		require.NoError(t, s.Save(ctx, m.Snapshot(true)))
	}
	require.NoError(t, s.Prune(ctx, now))
	var count int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM service_traffic_buckets`).Scan(&count))
	require.Equal(t, 1, count)
	r, err := s.Report(ctx, wgtunnel.TrafficWindow{Interval: "minute", Start: now.Add(-time.Hour), End: now})
	require.NoError(t, err)
	require.False(t, r.HasRecords)
	require.Nil(t, r.Peaks)
	require.Empty(t, r.History)
	require.Equal(t, "0", r.TotalBytes)
}
func TestWireGuardPeakUpgradeAndIdempotentRollup(t *testing.T) {
	db := wireguardTestDB(t)
	s := &WireGuardStore{DB: db}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Hour).Add(time.Minute)
	// Legacy records survive the additive migration and remain unknown, not zero.
	_, err := db.Exec(`INSERT INTO wireguard_traffic_hourly(proxy_id,bucket_start,tx_bytes,rx_bytes) VALUES(1,$1,100,200)`, now.Add(-time.Hour).Truncate(time.Hour))
	require.NoError(t, err)
	migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "253_service_traffic_peaks.sql"))
	require.NoError(t, err)
	_, err = db.Exec(string(migration))
	require.NoError(t, err)
	legacy, err := s.TrafficReport(ctx, 1, wgtunnel.TrafficWindow{Interval: "hour", Start: now.Add(-time.Hour).Truncate(time.Hour), End: now.Truncate(time.Hour)})
	require.NoError(t, err)
	require.Equal(t, "300", legacy.Total)
	require.Nil(t, legacy.PeakHistory)
	var c wgtunnel.Counters
	m := wgtunnel.NewMeter(&c, now)
	c.TX.Add(200)
	_, sampleErr := m.Sample(now.Add(time.Second))
	require.NoError(t, sampleErr)
	cp, gap := m.Checkpoint(1, "peak-run", 1, false)
	require.NoError(t, s.SaveCheckpoint(ctx, cp, gap))
	require.NoError(t, s.SaveCheckpoint(ctx, cp, gap))
	c.RX.Add(300)
	_, sampleErr = m.Sample(now.Add(2 * time.Second))
	require.NoError(t, sampleErr)
	cp, gap = m.Checkpoint(1, "peak-run", 1, false)
	require.NoError(t, s.SaveCheckpoint(ctx, cp, gap))
	require.NoError(t, s.SaveCheckpoint(ctx, cp, gap))
	for _, interval := range []string{"minute", "hour"} {
		r, e := s.TrafficReport(ctx, 1, wgtunnel.TrafficWindow{Interval: interval, Start: now.Truncate(time.Hour), End: now.Add(time.Hour)})
		require.NoError(t, e)
		require.Equal(t, "500", r.Total)
		require.NotNil(t, r.PeakHistory)
		require.Equal(t, 200.0, r.PeakHistory.Peaks["tx"].BytesPerSecond)
		require.Equal(t, 300.0, r.PeakHistory.Peaks["rx"].BytesPerSecond)
		require.Equal(t, 300.0, r.PeakHistory.Peaks["total"].BytesPerSecond)
		require.Equal(t, uint64(2), r.PeakHistory.Quality.ValidSamples)
	}
}

func TestTrafficLifecycleFragmentsPersistBytesButNotPeaks(t *testing.T) {
	db := wireguardTestDB(t)
	ctx := context.Background()
	start := time.Now().UTC().Truncate(time.Hour).Add(time.Minute)
	window := wgtunnel.TrafficWindow{Start: start.Truncate(time.Hour), End: start.Add(time.Minute)}
	serviceStore := &ServiceTrafficStore{DB: db}
	wgStore := &WireGuardStore{DB: db}
	serviceMeter := traffic.NewMeter("fragments", start)
	var counts wgtunnel.Counters
	wgMeter := wgtunnel.NewMeter(&counts, start)

	// The first and final bursts would dominate the peak if either fragment
	// were treated as a periodic sample. All three bursts must still be retained
	// in the byte counters, including when checkpoints are retried.
	for i, elapsed := range []time.Duration{533 * time.Millisecond, 1533 * time.Millisecond, 2483 * time.Millisecond} {
		n := 10000
		if i == 1 {
			n = 100
		}
		serviceMeter.Add(traffic.ClientIn, n)
		counts.TX.Add(uint64(n))
		final := i == 2
		if final {
			serviceMeter.SampleFinal(start.Add(elapsed))
			_, err := wgMeter.SampleFinal(start.Add(elapsed))
			require.NoError(t, err)
		} else {
			serviceMeter.Sample(start.Add(elapsed))
			_, err := wgMeter.Sample(start.Add(elapsed))
			require.NoError(t, err)
		}
		snapshot := serviceMeter.Snapshot(final)
		checkpoint, gap := wgMeter.Checkpoint(1, "fragments", 1, final)
		for retry := 0; retry < 2; retry++ {
			require.NoError(t, serviceStore.Save(ctx, snapshot))
			require.NoError(t, wgStore.SaveCheckpoint(ctx, checkpoint, gap))
		}
	}

	for _, interval := range []string{"minute", "hour"} {
		window.Interval = interval
		serviceReport, err := serviceStore.Report(ctx, window)
		require.NoError(t, err)
		require.Equal(t, "20100", serviceReport.TotalBytes)
		require.Equal(t, 100.0, serviceReport.Peaks["total"].BytesPerSecond)
		require.Equal(t, start.Add(1533*time.Millisecond), serviceReport.Peaks["total"].At)
		require.Equal(t, uint64(1), serviceReport.Quality.ValidSamples)
		require.Equal(t, uint64(2), serviceReport.Quality.InvalidSamples)
		require.True(t, serviceReport.Incomplete)

		wgReport, err := wgStore.TrafficReport(ctx, 1, window)
		require.NoError(t, err)
		require.Equal(t, "20100", wgReport.Total)
		require.Equal(t, 100.0, wgReport.PeakHistory.Peaks["total"].BytesPerSecond)
		require.Equal(t, start.Add(1533*time.Millisecond), wgReport.PeakHistory.Peaks["total"].At)
		require.Equal(t, uint64(1), wgReport.PeakHistory.Quality.ValidSamples)
		require.Equal(t, uint64(2), wgReport.PeakHistory.Quality.InvalidSamples)
	}
}
