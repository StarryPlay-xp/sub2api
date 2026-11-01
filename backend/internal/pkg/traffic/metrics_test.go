package traffic

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTrafficPeakUsesOneSecondAndSimultaneousTotal(t *testing.T) {
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	m := NewMeter("test", now)
	m.Add(ClientIn, 100)
	m.Add(UpstreamIn, 200)
	m.Sample(now.Add(time.Second))
	m.Add(ClientOut, 400)
	m.Add(UpstreamOut, 500)
	m.Sample(now.Add(2 * time.Second))
	s := m.Snapshot(false)
	require.Len(t, s.Buckets, 2)
	for _, b := range s.Buckets {
		require.Equal(t, uint64(1200), b.Bytes.Vector()[6])
		require.Equal(t, 300.0, b.Peaks["in"].BytesPerSecond)
		require.Equal(t, 900.0, b.Peaks["out"].BytesPerSecond)
		require.Equal(t, 900.0, b.Peaks["total"].BytesPerSecond)
		require.Equal(t, now.Add(2*time.Second), b.Peaks["total"].At)
	}
	m.Sample(now.Add(3 * time.Second))
	require.Zero(t, m.Current().Rates["total"])
	require.Equal(t, 900.0, m.Snapshot(false).Buckets[0].Peaks["total"].BytesPerSecond)
}
func TestTrafficDelayedSampleDoesNotFabricatePeak(t *testing.T) {
	now := time.Now()
	m := NewMeter("delay", now)
	m.Add(ClientIn, 1000)
	m.Sample(now.Add(5 * time.Second))
	require.Equal(t, "invalid_interval", m.Current().State)
	require.Equal(t, 200.0, m.Current().Rates["in"])
	for _, b := range m.Snapshot(false).Buckets {
		require.Empty(t, b.Peaks)
		require.Equal(t, uint64(1000), b.Bytes.ClientIn)
		require.Equal(t, uint64(1), b.Quality.InvalidSamples)
	}
	m.Add(ClientIn, 50)
	m.Sample(now.Add(6 * time.Second))
	require.Equal(t, 50.0, m.Current().Rates["in"])
}
func TestTrafficConcurrentCountsAndAckDoesNotLoseNewData(t *testing.T) {
	now := time.Date(2026, 11, 1, 0, 0, 58, 0, time.UTC)
	m := NewMeter("parallel", now)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				m.Add(ClientIn, 10)
			}
		}()
	}
	wg.Wait()
	m.Sample(now.Add(time.Second))
	old := m.Snapshot(false)
	m.Add(ClientIn, 7)
	m.Sample(now.Add(2 * time.Second))
	m.Ack(old)
	s := m.Snapshot(false)
	var hour Bucket
	for _, b := range s.Buckets {
		if b.Interval == "hour" {
			hour = b
		}
	}
	require.Equal(t, uint64(20007), hour.Bytes.ClientIn)
	// Snapshots do not alias mutable peak maps.
	old.Buckets[0].Peaks["in"] = Peak{BytesPerSecond: 999999}
	require.NotEqual(t, 999999.0, m.Snapshot(false).Buckets[0].Peaks["in"].BytesPerSecond)
}
func TestTrafficMeasuredElapsedTimeAndEqualPeakKeepsFirst(t *testing.T) {
	now := time.Now()
	m := NewMeter("clock", now)
	m.Add(ClientOut, 1100)
	m.Sample(now.Add(1100 * time.Millisecond))
	require.InDelta(t, 1000.0, m.Current().Rates["out"], 1e-9)
	m.Sample(now)
	require.Equal(t, now.Add(1100*time.Millisecond), m.Current().At)
	p := Peaks{}
	p.Merge(Peaks{"out": {1000, now, 1}})
	p.Merge(Peaks{"out": {1000, now.Add(time.Second), 1}})
	require.Equal(t, now, p["out"].At)
}
func BenchmarkTrafficCount(b *testing.B) {
	m := NewMeter("bench", time.Now())
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			m.Add(ClientIn, 16384)
		}
	})
}
