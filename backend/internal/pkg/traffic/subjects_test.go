package traffic

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func subjectBucket(t *testing.T, m *Meter, kind string, id int64) SubjectBucket {
	t.Helper()
	for _, b := range m.Snapshot(false).Subjects {
		if b.Type == kind && b.ID == id {
			return b
		}
	}
	t.Fatalf("missing %s %d", kind, id)
	return SubjectBucket{}
}
func assertSubjectConservation(t *testing.T, m *Meter) {
	t.Helper()
	s := m.Snapshot(false)
	var host, keys, users, unattributed Bytes
	byUser := map[int64]Bytes{}
	userRows := map[int64]Bytes{}
	for _, b := range s.Buckets {
		if b.Interval == "hour" {
			host.Add(b.Bytes)
		}
	}
	for _, b := range s.Subjects {
		switch b.Type {
		case "api_key":
			keys.Add(b.Bytes)
			v := byUser[b.UserID]
			v.Add(b.Bytes)
			byUser[b.UserID] = v
		case "user":
			users.Add(b.Bytes)
			v := userRows[b.UserID]
			v.Add(b.Bytes)
			userRows[b.UserID] = v
		case "unattributed":
			unattributed.Add(b.Bytes)
		}
	}
	require.Equal(t, keys, users)
	require.Equal(t, byUser, userRows)
	keys.Add(unattributed)
	require.Equal(t, host, keys)
}
func TestTrafficSubjectsPeaksConservationAndScopes(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	m := NewMeter("subjects", now)
	a, b := m.Scope(), m.Scope()
	a.Add(ClientIn, 7)
	a.Bind(1, 11, true)
	b.Bind(1, 12, true)
	a.Bind(2, 21, true)
	a.Add(ClientIn, 100)
	b.Add(UpstreamIn, 200)
	m.Sample(now.Add(time.Second))
	assertSubjectConservation(t, m)
	require.Equal(t, 300.0, subjectBucket(t, m, "user", 1).Peaks["total"].BytesPerSecond)
	a.Add(ClientOut, 400)
	m.Sample(now.Add(2 * time.Second))
	b.Add(ClientOut, 500)
	m.Sample(now.Add(3 * time.Second))
	require.Equal(t, 500.0, subjectBucket(t, m, "user", 1).Peaks["total"].BytesPerSecond)
	require.Equal(t, uint64(2), subjectBucket(t, m, "user", 1).Requests)
	require.Equal(t, uint64(3), subjectBucket(t, m, "user", 1).ActiveSeconds)
	require.Equal(t, int64(2), m.Live("user").Items[0].InFlight)
	a.CloseScope()
	a.CloseScope()
	b.CloseScope()
	a.Add(ClientOut, 13)
	ctx, closeBackground := BackgroundScope(WithMeter(context.Background(), m), 1, 11)
	FromContext(ctx).Add(UpstreamOut, 19)
	closeBackground()
	m.Sample(now.Add(4 * time.Second))
	assertSubjectConservation(t, m)
	require.Equal(t, uint64(2), subjectBucket(t, m, "user", 1).Requests)
	require.Equal(t, uint64(20), subjectBucket(t, m, "unattributed", 0).Bytes.Vector()[6])
	require.Equal(t, int64(0), m.Live("user").Items[0].InFlight)
}
func TestTrafficSubjectsFragmentsOverflowEvictionAndReentry(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	m := NewMeter("limit", now)
	m.subjectLimit = 1
	a, b := m.Scope(), m.Scope()
	a.Bind(1, 11, true)
	b.Bind(2, 22, true)
	a.Add(ClientIn, 1000)
	b.Add(ClientIn, 20)
	m.Sample(now.Add(999 * time.Millisecond))
	require.Empty(t, subjectBucket(t, m, "user", 1).Peaks)
	a.Add(ClientIn, 999)
	m.Sample(now.Add(1998 * time.Millisecond))
	require.InDelta(t, 1000, subjectBucket(t, m, "user", 1).Peaks["in"].BytesPerSecond, 1e-9)
	a.Add(ClientIn, 5000)
	m.SampleFinal(now.Add(2997 * time.Millisecond))
	require.InDelta(t, 1000, subjectBucket(t, m, "user", 1).Peaks["in"].BytesPerSecond, 1e-9)
	require.True(t, subjectBucket(t, m, "unattributed", 0).Overflow)
	require.Equal(t, uint64(20), subjectBucket(t, m, "unattributed", 0).Bytes.ClientIn)
	a.CloseScope()
	b.CloseScope()
	m.Sample(now.Add(64 * time.Second))
	require.Empty(t, m.subjects)
	require.Empty(t, m.Live("user").Items)
	c := m.Scope()
	c.Bind(1, 11, true)
	c.Add(ClientIn, 1)
	m.Sample(now.Add(65 * time.Second))
	c.CloseScope()
	require.Equal(t, uint64(7000), subjectBucket(t, m, "api_key", 11).Bytes.ClientIn)
	assertSubjectConservation(t, m)
}
func TestTrafficSubjectsConcurrentSamplingAndSwitch(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	m := NewMeter("race", now)
	a, b := m.Scope(), m.Scope()
	a.Bind(1, 11, true)
	b.Bind(2, 22, true)
	switched := a.Switch()
	switched.Add(ClientIn, 5)
	switched.SetSource(b)
	switched.Add(ClientOut, 9)
	switched.SetSource(nil)
	switched.Add(UpstreamIn, 3)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10000; j++ {
				a.Add(UpstreamIn, 1)
				b.Add(ClientOut, 2)
			}
		}()
	}
	for i := 1; i <= 60; i++ {
		m.Sample(now.Add(time.Duration(i) * time.Second))
		assertSubjectConservation(t, m)
	}
	wg.Wait()
	m.Sample(now.Add(61 * time.Second))
	assertSubjectConservation(t, m)
	require.Equal(t, uint64(80005), subjectBucket(t, m, "api_key", 11).Bytes.Vector()[6])
	require.Equal(t, uint64(160009), subjectBucket(t, m, "api_key", 22).Bytes.Vector()[6])
	a.CloseScope()
	b.CloseScope()
}
func TestTrafficSubjectsRollingAverageIncludesIdleTime(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	m := NewMeter("average", now)
	for i := 1; i < 60; i++ {
		m.Sample(now.Add(time.Duration(i) * time.Second))
	}
	a := m.Scope()
	a.Bind(1, 11, true)
	a.Add(ClientIn, 600)
	m.Sample(now.Add(60 * time.Second))
	v := m.Live("user").Items[0]
	require.Equal(t, 600.0, v.Rate.Total)
	require.Equal(t, 10.0, v.Average.Total)
	require.Equal(t, 60.0, v.AverageSeconds)
	a.CloseScope()
	for i := 61; i <= 120; i++ {
		m.Sample(now.Add(time.Duration(i) * time.Second))
	}
	require.Empty(t, m.Live("user").Items)
}
func BenchmarkTrafficCounter(b *testing.B) {
	b.Run("legacy_add", func(b *testing.B) {
		var m trafficLegacyMeter
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			m.Add(ClientIn, 1024)
		}
	})
	m := NewMeter("bench", time.Now())
	s := m.Scope()
	s.Bind(1, 11, true)
	defer s.CloseScope()
	ws := s.Switch()
	for _, tc := range []struct {
		name  string
		meter *Meter
	}{{"unattributed", m}, {"attributed", s}, {"ws_switch", ws}} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				tc.meter.Add(ClientIn, 1024)
			}
		})
	}
}

func TestTrafficSubjectsAckOnlyDirtyHoursAndNoReset(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	m := NewMeter("ack", now)
	a := m.Scope()
	a.Bind(1, 11, true)
	a.Add(ClientIn, 100)
	a.CloseScope()
	m.Sample(now.Add(time.Second))
	snapshot := m.Snapshot(false)
	m.Ack(snapshot)
	require.Empty(t, m.Snapshot(false).Subjects)
	m.Sample(now.Add(2 * time.Second))
	require.Len(t, m.Snapshot(false).Subjects, 1) // only unattributed coverage changed
	b := m.Scope()
	b.Bind(1, 11, true)
	b.Add(ClientIn, 50)
	m.Sample(now.Add(3 * time.Second))
	m.Ack(snapshot)
	require.Equal(t, uint64(150), subjectBucket(t, m, "api_key", 11).Bytes.ClientIn)
	b.CloseScope()
}

type trafficLegacyMeter struct{ counters [4]atomic.Uint64 }

func (m *trafficLegacyMeter) Add(direction, n int) {
	if m != nil && direction >= 0 && direction < 4 && n > 0 {
		m.counters[direction].Add(uint64(n))
	}
}
