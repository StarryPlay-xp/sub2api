package wgtunnel

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMeterStartupFragmentCannotSetPeak(t *testing.T) {
	for _, initial := range []time.Duration{533 * time.Millisecond, 999 * time.Millisecond} {
		t.Run(initial.String(), func(t *testing.T) {
			var counts Counters
			start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
			m := NewMeter(&counts, start)
			counts.TX.Add(10000)
			counts.RX.Add(20000)
			rate, err := m.Sample(start.Add(initial))
			require.NoError(t, err)
			require.Equal(t, "invalid_interval", rate.State)
			c, gap := m.Checkpoint(1, "startup", 1, false)
			require.False(t, gap)
			require.Equal(t, uint64(10000), c.TX)
			require.Equal(t, uint64(20000), c.RX)
			require.Empty(t, c.Buckets[0].PeakHistory.Peaks)
			require.Equal(t, uint64(1), c.Buckets[0].PeakHistory.Quality.InvalidSamples)

			counts.TX.Add(999)
			counts.RX.Add(1998)
			_, err = m.Sample(start.Add(initial + 999*time.Millisecond))
			require.NoError(t, err)
			c, _ = m.Checkpoint(1, "startup", 1, false)
			require.InDelta(t, 1000, c.Buckets[0].PeakHistory.Peaks["tx"].BytesPerSecond, 1e-9)
			require.InDelta(t, 3000, c.Buckets[0].PeakHistory.Peaks["total"].BytesPerSecond, 1e-9)
			require.Equal(t, uint64(10999), c.TX)
			require.Equal(t, uint64(21998), c.RX)
		})
	}
}

func TestMeterFinalSamplePreservesBytesWithoutChangingPeak(t *testing.T) {
	for _, tail := range []time.Duration{533 * time.Millisecond, 999 * time.Millisecond, time.Second} {
		t.Run(tail.String(), func(t *testing.T) {
			var counts Counters
			start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
			m := NewMeter(&counts, start)
			counts.TX.Add(100)
			counts.RX.Add(200)
			_, err := m.Sample(start.Add(time.Second))
			require.NoError(t, err)
			counts.TX.Add(10000)
			counts.RX.Add(20000)
			rate, err := m.SampleFinal(start.Add(time.Second + tail))
			require.NoError(t, err)
			require.Equal(t, "invalid_interval", rate.State)
			c, gap := m.Checkpoint(1, "shutdown", 1, true)
			require.False(t, gap)
			require.Equal(t, uint64(10100), c.TX)
			require.Equal(t, uint64(20200), c.RX)
			require.Equal(t, c.TX, c.Buckets[0].TX)
			require.Equal(t, c.RX, c.Buckets[0].RX)
			h := c.Buckets[0].PeakHistory
			require.Equal(t, 100.0, h.Peaks["tx"].BytesPerSecond)
			require.Equal(t, 300.0, h.Peaks["total"].BytesPerSecond)
			require.Equal(t, start.Add(time.Second), h.Peaks["total"].At)
			require.Equal(t, uint64(1), h.Quality.ValidSamples)
			require.Equal(t, uint64(1), h.Quality.InvalidSamples)
		})
	}
}

func TestMeterCountsBothDirectionsOnce(t *testing.T) {
	var counters Counters
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := NewMeter(&counters, now)
	counters.TX.Add(100)
	counters.RX.Add(200)
	r, e := m.Sample(now.Add(time.Second))
	if e != nil || r.TX != 100 || r.RX != 200 {
		t.Fatal("wrong directional rate", r, e)
	}
	c, gap := m.Checkpoint(1, "run", 1, false)
	if gap || c.TX+c.RX != 300 || len(c.Buckets) != 1 {
		t.Fatal("wrong checkpoint")
	}
	// Reading twice must not add the same counters to the minute bucket twice.
	c2, _ := m.Checkpoint(1, "run", 1, false)
	if c2.Seq != c.Seq || c2.Buckets[0].TX != 100 {
		t.Fatal("checkpoint changed counters")
	}
	m.Ack(c)
	counters.TX.Add(50)
	if _, err := m.Sample(now.Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	c3, _ := m.Checkpoint(1, "run", 1, false)
	if c3.Buckets[0].TX != 150 {
		t.Fatal("open bucket lost cumulative identity")
	}
}

func TestMeterMarksResetAndMissingHistory(t *testing.T) {
	var c Counters
	now := time.Now()
	m := NewMeter(&c, now)
	c.TX.Add(10)
	if _, err := m.Sample(now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	c.TX.Store(0)
	if _, e := m.Sample(now.Add(2 * time.Second)); e == nil {
		t.Fatal("counter reset silently accepted")
	}
	_, gap := m.Checkpoint(1, "run", 1, false)
	if !gap {
		t.Fatal("reset not marked")
	}
}

func TestMeterAckPreservesIdleSamplesAddedDuringSave(t *testing.T) {
	var counts Counters
	start := time.Date(2026, 1, 1, 0, 0, 57, 0, time.UTC)
	m := NewMeter(&counts, start)
	if _, err := m.Sample(start.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	saved, _ := m.Checkpoint(1, "run", 1, false)
	if _, err := m.Sample(start.Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Sample(start.Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	m.Ack(saved)
	next, _ := m.Checkpoint(1, "run", 1, false)
	if len(next.Buckets) != 2 {
		t.Fatal("unsaved idle sample was discarded")
	}
	m.Ack(next)
	final, _ := m.Checkpoint(1, "run", 1, false)
	if len(final.Buckets) != 1 {
		t.Fatal("persisted completed bucket was not retired")
	}
}
