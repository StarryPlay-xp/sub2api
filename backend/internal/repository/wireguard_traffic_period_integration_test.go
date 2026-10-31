//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
	"github.com/stretchr/testify/require"
)

func TestWireGuardTrafficPeriodTotalsAndBoundaries(t *testing.T) {
	db := wireguardTestDB(t)
	store := &WireGuardStore{DB: db}
	ctx := context.Background()
	now := time.Date(2026, 10, 31, 6, 20, 35, 0, time.UTC)
	window, err := wgtunnel.ResolveTrafficWindow(wgtunnel.TrafficQuery{Range: "custom", Start: "2026-10-01T00:00:00+08:00", End: "2026-10-02T00:00:00+08:00"}, now)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO proxies(id,name,protocol) VALUES(2,'other','wireguard'); INSERT INTO wireguard_nodes(proxy_id,config_ciphertext,public_summary) VALUES(2,'test','{}')`)
	require.NoError(t, err)
	for _, item := range []struct {
		id     int64
		at     time.Time
		tx, rx string
	}{
		{1, window.Start.Add(-time.Hour), "999", "999"},
		{1, window.Start, "9007199254740993", "3"},
		{1, window.Start.Add(time.Hour), "7", "11"},
		{1, window.End.Add(-time.Hour), "13", "17"},
		{1, window.End, "999", "999"},
		{2, window.Start, "999", "999"},
	} {
		_, err = db.Exec(`INSERT INTO wireguard_traffic_hourly(proxy_id,bucket_start,tx_bytes,rx_bytes) VALUES($1,$2,$3,$4)`, item.id, item.at, item.tx, item.rx)
		require.NoError(t, err)
	}
	result, err := store.TrafficReport(ctx, 1, window)
	require.NoError(t, err)
	require.Equal(t, "9007199254741013", result.TX)
	require.Equal(t, "31", result.RX)
	require.Equal(t, "9007199254741044", result.Total)
	require.Equal(t, 3, result.BucketCount)
	require.True(t, result.HasRecords)
	require.True(t, result.FirstRecordAt.Equal(window.Start))
	require.True(t, result.LastRecordAt.Equal(window.End.Add(-time.Hour)))
	window.Start, window.End = window.End.Add(time.Hour), window.End.Add(2*time.Hour)
	empty, err := store.TrafficReport(ctx, 1, window)
	require.NoError(t, err)
	require.False(t, empty.HasRecords)
	require.Empty(t, empty.History)
	require.Equal(t, "0", empty.Total)
	require.Nil(t, empty.FirstRecordAt)
}

func TestWireGuardTrafficPeriodMinuteAggregatesRuntimeRestarts(t *testing.T) {
	db := wireguardTestDB(t)
	store := &WireGuardStore{DB: db}
	ctx := context.Background()
	now := time.Now().UTC()
	window, err := wgtunnel.ResolveTrafficWindow(wgtunnel.TrafficQuery{Range: "24h"}, now)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO wireguard_traffic_runtimes(runtime_id,proxy_id,config_version) VALUES('first',1,1),('second',1,1)`)
	require.NoError(t, err)
	for _, id := range []string{"first", "second"} {
		_, err = db.Exec(`INSERT INTO wireguard_traffic_buckets(runtime_id,bucket_start,tx_bytes,rx_bytes) VALUES($1,$2,10,20)`, id, window.Start)
		require.NoError(t, err)
	}
	result, err := store.TrafficReport(ctx, 1, window)
	require.NoError(t, err)
	require.Equal(t, 1, result.BucketCount)
	require.Equal(t, "20", result.TX)
	require.Equal(t, "40", result.RX)
}

func TestWireGuardTrafficPeriodPresets(t *testing.T) {
	db := wireguardTestDB(t)
	store := &WireGuardStore{DB: db}
	now := time.Date(2026, 10, 31, 6, 20, 35, 0, time.UTC)
	for _, days := range []int{1, 8, 31, 91} {
		_, err := db.Exec(`INSERT INTO wireguard_traffic_hourly(proxy_id,bucket_start,tx_bytes,rx_bytes) VALUES(1,$1,100,200)`, now.Add(-time.Duration(days)*24*time.Hour).Truncate(time.Hour))
		require.NoError(t, err)
	}
	for _, tc := range []struct {
		period string
		count  int
	}{{"7d", 1}, {"30d", 2}, {"90d", 3}} {
		t.Run(tc.period, func(t *testing.T) {
			window, err := wgtunnel.ResolveTrafficWindow(wgtunnel.TrafficQuery{Range: tc.period}, now)
			require.NoError(t, err)
			report, err := store.TrafficReport(context.Background(), 1, window)
			require.NoError(t, err)
			require.Equal(t, tc.count, report.BucketCount)
		})
	}
}
