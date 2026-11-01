package traffic

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func egressBucket(t *testing.T, m *Meter, id EgressIdentity) EgressBucket {
	t.Helper()
	for _, b := range m.Snapshot(false).Egress {
		if b.EgressIdentity == id {
			return b
		}
	}
	t.Fatalf("missing egress %+v", id)
	return EgressBucket{}
}
func assertEgressConservation(t *testing.T, m *Meter) {
	t.Helper()
	s := m.Snapshot(false)
	var host, routes EgressBytes
	totals, accounts := map[EgressIdentity]EgressBytes{}, map[EgressIdentity]EgressBytes{}
	for _, b := range s.Buckets {
		if b.Interval == "hour" {
			host.Add(EgressBytes{b.Bytes.UpstreamOut, b.Bytes.UpstreamIn})
		}
	}
	for _, b := range s.Egress {
		key := EgressIdentity{Type: b.Type, ID: b.ID}
		if b.AccountID == 0 {
			routes.Add(b.Bytes)
			v := totals[key]
			v.Add(b.Bytes)
			totals[key] = v
		} else {
			v := accounts[key]
			v.Add(b.Bytes)
			accounts[key] = v
		}
	}
	require.Equal(t, host, routes)
	for key, bytes := range totals {
		if key.Type != "unidentified" {
			require.Equal(t, bytes, accounts[key])
		}
	}
	assertSubjectConservation(t, m)
}
func TestTrafficEgressConservationPeaksAndRetry(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	m := NewMeter("egress", now)
	user := m.Scope()
	user.Bind(1, 11, true)
	defer user.CloseScope()
	a := user.EgressScope(EgressIdentity{"proxy", 7, 10})
	defer a.CloseEgress()
	b := user.EgressScope(EgressIdentity{"proxy", 7, 20})
	defer b.CloseEgress()
	c := user.EgressScope(EgressIdentity{"wireguard", 8, 30})
	defer c.CloseEgress()
	a.Add(UpstreamOut, 100)
	b.Add(UpstreamIn, 200)
	c.Add(UpstreamOut, 50)
	a.Add(ClientIn, 900)
	m.Add(UpstreamIn, 17)
	m.Sample(now.Add(time.Second))
	assertEgressConservation(t, m)
	route := EgressIdentity{"proxy", 7, 0}
	require.Equal(t, 300.0, egressBucket(t, m, route).Peaks["total"].BytesPerSecond)
	a.Add(UpstreamOut, 400)
	m.Sample(now.Add(2 * time.Second))
	b.Add(UpstreamIn, 500)
	m.Sample(now.Add(3 * time.Second))
	require.Equal(t, 500.0, egressBucket(t, m, route).Peaks["total"].BytesPerSecond)
	require.Equal(t, uint64(2), egressBucket(t, m, route).Attempts)
	require.Equal(t, uint64(17), egressBucket(t, m, EgressIdentity{Type: "unidentified"}).Bytes.In)
	assertEgressConservation(t, m)
}
func TestTrafficEgressOverflowFragmentsReentryAndAck(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	m := NewMeter("overflow", now)
	m.egressLimit = 1
	user := m.Scope()
	user.Bind(1, 11, true)
	a := user.EgressScope(EgressIdentity{"direct", 0, 10})
	b := user.EgressScope(EgressIdentity{"direct", 0, 20})
	a.Add(UpstreamOut, 1000)
	b.Add(UpstreamIn, 25)
	user.CloseScope() // leaves pin the user until their last bytes have been sampled
	m.Sample(now.Add(999 * time.Millisecond))
	route := EgressIdentity{"direct", 0, 0}
	require.Empty(t, egressBucket(t, m, route).Peaks)
	require.True(t, egressBucket(t, m, route).Overflow)
	require.Equal(t, uint64(25), egressBucket(t, m, EgressIdentity{"direct", 0, -1}).Bytes.In)
	a.Add(UpstreamOut, 999)
	m.Sample(now.Add(1998 * time.Millisecond))
	a.Add(UpstreamOut, 5000)
	m.SampleFinal(now.Add(2997 * time.Millisecond))
	require.InDelta(t, 1000, egressBucket(t, m, route).Peaks["total"].BytesPerSecond, 1e-9)
	a.CloseEgress()
	a.CloseEgress()
	b.CloseEgress()
	m.Sample(now.Add(64 * time.Second))
	require.Empty(t, m.egressCounters)
	require.Empty(t, m.subjects)
	assertEgressConservation(t, m)
	old := m.Snapshot(false)
	m.Ack(old)
	require.Empty(t, m.Snapshot(false).Egress)
	c := m.EgressScope(EgressIdentity{"direct", 0, 10})
	c.Add(UpstreamIn, 1)
	c.CloseEgress()
	m.Sample(now.Add(65 * time.Second))
	m.Ack(old)
	require.Equal(t, uint64(7025), egressBucket(t, m, route).Bytes.Total())
}
func TestTrafficEgressUnknownHistorySurvivesFirstRouteAndLimits(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	m := NewMeter("limits", now)
	m.Add(UpstreamIn, 13)
	m.Sample(now.Add(time.Second))
	// Simulate the route bound without allocating thousands of live request scopes.
	m.egressCounters = map[egressLeafKey]*egressCounter{}
	m.egressCombinations = map[EgressIdentity]int{}
	m.egressRoutes = map[EgressIdentity]int{}
	for i := 1; i <= maxEgressRoutes; i++ {
		m.egressRoutes[EgressIdentity{"proxy", int64(i), 0}] = 1
	}
	scope := m.EgressScope(EgressIdentity{"proxy", maxEgressRoutes + 1, 1})
	scope.Add(UpstreamOut, 19)
	scope.CloseEgress()
	m.Sample(now.Add(2 * time.Second))
	unknown := egressBucket(t, m, EgressIdentity{Type: "unidentified"})
	require.Equal(t, EgressBytes{19, 13}, unknown.Bytes)
	require.True(t, unknown.Overflow)
	require.Equal(t, uint64(1), unknown.Attempts)
	assertEgressConservation(t, m)
	// Starting routing normally also preserves the already sampled unknown bucket.
	n := NewMeter("init", now)
	n.Add(UpstreamIn, 7)
	n.Sample(now.Add(time.Second))
	r := n.EgressScope(EgressIdentity{"direct", 0, 1})
	r.Add(UpstreamOut, 3)
	r.CloseEgress()
	n.Sample(now.Add(2 * time.Second))
	require.Equal(t, uint64(7), egressBucket(t, n, EgressIdentity{Type: "unidentified"}).Bytes.In)
	assertEgressConservation(t, n)
}
func TestTrafficEgressConcurrentSamplingAndLeaseSwitch(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	m := NewMeter("concurrent", now)
	a, b := m.Scope(), m.Scope()
	a.Bind(1, 11, true)
	b.Bind(2, 22, true)
	id := EgressIdentity{"wireguard", 7, 9}
	ra, rb, idle := a.EgressScope(id), b.EgressScope(id), m.EgressIdleScope(id)
	sw := ra.Switch()
	sw.Add(UpstreamOut, 3)
	sw.SetSource(rb)
	sw.Add(UpstreamOut, 5)
	sw.SetSource(idle)
	sw.Add(UpstreamOut, 7)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10000; j++ {
				ra.Add(UpstreamIn, 1)
				rb.Add(UpstreamOut, 2)
			}
		}()
	}
	for i := 1; i <= 20; i++ {
		m.Sample(now.Add(time.Duration(i) * time.Second))
		assertEgressConservation(t, m)
	}
	wg.Wait()
	m.Sample(now.Add(21 * time.Second))
	assertEgressConservation(t, m)
	require.Equal(t, uint64(80003), subjectBucket(t, m, "api_key", 11).Bytes.Vector()[6])
	require.Equal(t, uint64(160005), subjectBucket(t, m, "api_key", 22).Bytes.Vector()[6])
	require.Equal(t, uint64(2), egressBucket(t, m, EgressIdentity{"wireguard", 7, 0}).Attempts)
	ra.CloseEgress()
	rb.CloseEgress()
	idle.CloseEgress()
	a.CloseScope()
	b.CloseScope()
}
func TestTrafficEgressHTTPNestedRetriesFailureAndBackground(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	m := NewMeter("http", now)
	scope := m.Scope()
	scope.Bind(1, 11, true)
	defer scope.CloseScope()
	for i, id := range []EgressIdentity{{"proxy", 7, 10}, {"wireguard", 8, 20}} {
		inner := EgressTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
			_, err := io.Copy(io.Discard, r.Body)
			require.NoError(t, err)
			require.NoError(t, r.Body.Close())
			// An underlying transport may replay the body without another RoundTrip.
			replay, err := r.GetBody()
			require.NoError(t, err)
			_, err = io.Copy(io.Discard, replay)
			require.NoError(t, err)
			_ = replay.Close()
			if i == 0 {
				return nil, errors.New("synthetic failure")
			}
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("response"))}, nil
		}), id)
		outer := Transport(roundTripFunc(func(r *http.Request) (*http.Response, error) { return inner.RoundTrip(r) }))
		req, err := http.NewRequestWithContext(WithMeter(context.Background(), scope), "POST", "http://example.test", strings.NewReader("upload"))
		require.NoError(t, err)
		resp, err := outer.RoundTrip(req)
		if i == 0 {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			_, err = io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
		}
	}
	m.Sample(now.Add(time.Second))
	assertEgressConservation(t, m)
	require.Equal(t, EgressBytes{12, 0}, egressBucket(t, m, EgressIdentity{"proxy", 7, 10}).Bytes)
	require.Equal(t, EgressBytes{12, 8}, egressBucket(t, m, EgressIdentity{"wireguard", 8, 20}).Bytes)
	for _, leaf := range m.egressCounters {
		require.Zero(t, leaf.refs)
	}
	bg := EgressTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		_, _ = io.Copy(io.Discard, r.Body)
		return nil, errors.New("background")
	}), EgressIdentity{"direct", 0, 3})
	req, _ := http.NewRequest("POST", "http://example.test", strings.NewReader("ignored"))
	_, _ = bg.RoundTrip(req)
	m.Sample(now.Add(2 * time.Second))
	assertEgressConservation(t, m)
	require.Len(t, m.egressCounters, 2)
}
func BenchmarkTrafficEgressCounter(b *testing.B) {
	m := NewMeter("bench", time.Now())
	subject := m.Scope()
	subject.Bind(1, 11, true)
	defer subject.CloseScope()
	route := subject.EgressScope(EgressIdentity{"wireguard", 1, 1})
	defer route.CloseEgress()
	for _, tc := range []struct {
		name  string
		meter *Meter
	}{{"subject", subject}, {"egress", route}, {"ws_switch", route.Switch()}} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				tc.meter.Add(UpstreamOut, 1024)
			}
		})
	}
}

func TestTrafficEgressHTTPEarlyResponseKeepsUploadAttribution(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	m := NewMeter("early-response", now)
	var upload io.ReadCloser
	transport := EgressTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		upload = r.Body
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	}), EgressIdentity{"direct", 0, 1})
	req, err := http.NewRequestWithContext(WithMeter(context.Background(), m), "POST", "http://example.test", strings.NewReader("late upload"))
	require.NoError(t, err)
	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	// net/http permits request-body consumption and Close after RoundTrip returns.
	_, err = io.Copy(io.Discard, upload)
	require.NoError(t, err)
	require.NoError(t, upload.Close())
	m.Sample(now.Add(time.Second))
	require.Equal(t, uint64(11), egressBucket(t, m, EgressIdentity{"direct", 0, 1}).Bytes.Out)
	require.Zero(t, egressBucket(t, m, EgressIdentity{Type: "unidentified"}).Bytes.Total())
	for _, leaf := range m.egressCounters {
		require.Zero(t, leaf.refs)
	}
	assertEgressConservation(t, m)
}
