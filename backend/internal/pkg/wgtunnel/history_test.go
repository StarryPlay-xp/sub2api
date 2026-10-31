package wgtunnel

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTrafficWindowPresetsAndRetention(t *testing.T) {
	now := time.Date(2026, 10, 31, 6, 20, 35, 0, time.UTC)
	for _, tc := range []struct {
		name, interval  string
		duration, grain time.Duration
	}{
		{"24h", "minute", 24 * time.Hour, time.Minute},
		{"7d", "hour", 7 * 24 * time.Hour, time.Hour},
		{"30d", "hour", 30 * 24 * time.Hour, time.Hour},
		{"90d", "hour", 90 * 24 * time.Hour, time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			window, err := ResolveTrafficWindow(TrafficQuery{Range: tc.name}, now)
			require.NoError(t, err)
			require.Equal(t, tc.interval, window.Interval)
			require.Equal(t, now.Add(-tc.duration), window.RequestedStart)
			require.Equal(t, ceilBucket(window.RequestedStart, tc.grain), window.Start)
			require.Equal(t, now, window.End)
			require.False(t, window.Start.Before(window.RetainedFrom))
		})
	}
	window, err := ResolveTrafficWindow(TrafficQuery{}, now)
	require.NoError(t, err)
	require.Equal(t, "24h", window.Range)
}

func TestTrafficWindowCustomTimezoneAndExclusiveEnd(t *testing.T) {
	now := time.Date(2026, 10, 31, 6, 20, 35, 0, time.UTC)
	window, err := ResolveTrafficWindow(TrafficQuery{Range: "custom", Start: "2026-10-01T00:00:00+08:00", End: "2026-10-16T00:00:00+08:00"}, now)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 9, 30, 16, 0, 0, 0, time.UTC), window.Start)
	require.Equal(t, time.Date(2026, 10, 15, 16, 0, 0, 0, time.UTC), window.End)
	require.Equal(t, "hour", window.Interval)
	throughNow, err := ResolveTrafficWindow(TrafficQuery{Range: "custom", Start: "2026-10-01T00:00:00+08:00"}, now)
	require.NoError(t, err)
	require.Equal(t, now, throughNow.End)
	endOfToday, err := ResolveTrafficWindow(TrafficQuery{Range: "custom", Start: "2026-10-01T00:00:00+08:00", End: "2026-11-01T00:00:00+08:00"}, now)
	require.NoError(t, err)
	require.True(t, endOfToday.EndClamped)
	require.Equal(t, now, endOfToday.End)
}

func TestTrafficWindowRejectsMisleadingRanges(t *testing.T) {
	now := time.Date(2026, 10, 31, 6, 20, 35, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		query  TrafficQuery
		reason string
	}{
		{"unknown", TrafficQuery{Range: "365d"}, "TRAFFIC_RANGE_INVALID"},
		{"interval", TrafficQuery{Interval: "day"}, "TRAFFIC_INTERVAL_INVALID"},
		{"mixed", TrafficQuery{Range: "7d", Start: "2026-10-01T00:00:00Z"}, "TRAFFIC_RANGE_INVALID"},
		{"missing-start", TrafficQuery{Range: "custom"}, "TRAFFIC_DATE_INVALID"},
		{"without-zone", TrafficQuery{Range: "custom", Start: "2026-10-01T00:00:00"}, "TRAFFIC_DATE_INVALID"},
		{"reversed", TrafficQuery{Range: "custom", Start: "2026-10-20T00:00:00Z", End: "2026-10-19T00:00:00Z"}, "TRAFFIC_RANGE_INVALID"},
		{"future", TrafficQuery{Range: "custom", Start: "2026-11-01T00:00:00Z"}, "TRAFFIC_RANGE_INVALID"},
		{"far-future-end", TrafficQuery{Range: "custom", Start: "2026-10-01T00:00:00Z", End: "2027-01-01T00:00:00Z"}, "TRAFFIC_DATE_INVALID"},
		{"unaligned-start", TrafficQuery{Range: "custom", Start: "2026-10-01T00:30:00Z"}, "TRAFFIC_BOUNDARY_UNALIGNED"},
		{"unaligned-end", TrafficQuery{Range: "custom", Start: "2026-10-01T00:00:00Z", End: "2026-10-02T00:30:00Z"}, "TRAFFIC_BOUNDARY_UNALIGNED"},
		{"expired-hourly", TrafficQuery{Range: "custom", Start: "2026-07-01T00:00:00Z"}, "TRAFFIC_RETENTION_EXCEEDED"},
		{"expired-minute", TrafficQuery{Range: "30d", Interval: "minute"}, "TRAFFIC_RETENTION_EXCEEDED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ResolveTrafficWindow(tc.query, now)
			var invalid *TrafficQueryError
			require.ErrorAs(t, err, &invalid)
			require.Equal(t, tc.reason, invalid.Reason)
		})
	}
}

func TestTrafficWindowRetentionBoundary(t *testing.T) {
	now := time.Date(2026, 10, 31, 6, 20, 35, 0, time.UTC)
	for _, interval := range []string{"hour", "minute"} {
		t.Run(interval, func(t *testing.T) {
			retention, grain := TrafficRetention, time.Hour
			if interval == "minute" {
				retention, grain = MinuteTrafficRetention, time.Minute
			}
			cutoff := ceilBucket(now.Add(-retention), grain)
			window, err := ResolveTrafficWindow(TrafficQuery{Range: "custom", Interval: interval, Start: cutoff.Format(time.RFC3339)}, now)
			require.NoError(t, err)
			require.Equal(t, cutoff, window.Start)
			_, err = ResolveTrafficWindow(TrafficQuery{Range: "custom", Interval: interval, Start: cutoff.Add(-grain).Format(time.RFC3339)}, now)
			var invalid *TrafficQueryError
			require.ErrorAs(t, err, &invalid)
			require.Equal(t, "TRAFFIC_RETENTION_EXCEEDED", invalid.Reason)
		})
	}
}
