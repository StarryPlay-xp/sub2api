// Package traffic measures application payload bytes, never billing tokens or
// packet sizes. Samples are independent of requests completing and UI polling.
package traffic

import (
	"sync"
	"sync/atomic"
	"time"
)

const MetricBasis = "service_payload_v1"
const (
	ClientIn = iota
	ClientOut
	UpstreamOut
	UpstreamIn
)

var Keys = [...]string{"client_in", "client_out", "upstream_out", "upstream_in", "in", "out", "total"}

type Bytes struct {
	ClientIn    uint64 `json:"client_in,string"`
	ClientOut   uint64 `json:"client_out,string"`
	UpstreamOut uint64 `json:"upstream_out,string"`
	UpstreamIn  uint64 `json:"upstream_in,string"`
}

func (b Bytes) Vector() [7]uint64 {
	return [7]uint64{b.ClientIn, b.ClientOut, b.UpstreamOut, b.UpstreamIn, b.ClientIn + b.UpstreamIn, b.ClientOut + b.UpstreamOut, b.ClientIn + b.ClientOut + b.UpstreamOut + b.UpstreamIn}
}
func (b *Bytes) Add(v Bytes) {
	b.ClientIn += v.ClientIn
	b.ClientOut += v.ClientOut
	b.UpstreamOut += v.UpstreamOut
	b.UpstreamIn += v.UpstreamIn
}
func (b Bytes) Sub(v Bytes) Bytes {
	return Bytes{b.ClientIn - v.ClientIn, b.ClientOut - v.ClientOut, b.UpstreamOut - v.UpstreamOut, b.UpstreamIn - v.UpstreamIn}
}

type Peak struct {
	BytesPerSecond float64   `json:"bytes_per_second"`
	At             time.Time `json:"at"`
	WindowSeconds  float64   `json:"window_seconds"`
}
type Peaks map[string]Peak

func (p Peaks) Merge(other Peaks) {
	for k, v := range other {
		old, ok := p[k]
		if !ok || v.BytesPerSecond > old.BytesPerSecond || (v.BytesPerSecond == old.BytesPerSecond && v.At.Before(old.At)) {
			p[k] = v
		}
	}
}
func (p Peaks) Clone() Peaks { v := Peaks{}; v.Merge(p); return v }

// PeaksFromRates allows small scheduler jitter around a one-second tick.
// Larger deviations are not comparable one-second peaks. Lifecycle fragments
// must additionally be excluded by the caller, even within this tolerance.
func PeaksFromRates(values map[string]float64, at time.Time, seconds float64) Peaks {
	if seconds < 0.9 || seconds > 1.1 {
		return nil
	}
	p := Peaks{}
	for k, v := range values {
		p[k] = Peak{v, at, seconds}
	}
	return p
}

type Quality struct {
	FirstSampleAt  *time.Time `json:"first_sample_at"`
	LastSampleAt   *time.Time `json:"last_sample_at"`
	ValidSamples   uint64     `json:"valid_samples"`
	InvalidSamples uint64     `json:"invalid_samples"`
}

func (q *Quality) Observe(at time.Time, valid bool) {
	if q.FirstSampleAt == nil {
		t := at
		q.FirstSampleAt = &t
	}
	t := at
	q.LastSampleAt = &t
	if valid {
		q.ValidSamples++
	} else {
		q.InvalidSamples++
	}
}
func (q *Quality) Merge(v Quality) {
	if v.FirstSampleAt != nil && (q.FirstSampleAt == nil || v.FirstSampleAt.Before(*q.FirstSampleAt)) {
		t := *v.FirstSampleAt
		q.FirstSampleAt = &t
	}
	if v.LastSampleAt != nil && (q.LastSampleAt == nil || v.LastSampleAt.After(*q.LastSampleAt)) {
		t := *v.LastSampleAt
		q.LastSampleAt = &t
	}
	q.ValidSamples += v.ValidSamples
	q.InvalidSamples += v.InvalidSamples
}

type Bucket struct {
	Start    time.Time `json:"start"`
	Interval string    `json:"interval"`
	Bytes    Bytes     `json:"bytes"`
	Peaks    Peaks     `json:"peaks"`
	Quality  Quality   `json:"quality"`
	Sequence uint64    `json:"-"`
}
type Current struct {
	Rates         map[string]float64 `json:"bytes_per_second"`
	At            time.Time          `json:"sampled_at"`
	WindowSeconds float64            `json:"window_seconds"`
	State         string             `json:"state"`
}
type Snapshot struct {
	RuntimeID  string
	StartedAt  time.Time
	At         time.Time
	Sequence   uint64
	Closed     bool
	Incomplete bool
	Buckets    []Bucket
}

type Meter struct {
	counters      [4]atomic.Uint64
	mu            sync.Mutex
	runtimeID     string
	started, last time.Time
	previous      Bytes
	seq           uint64
	incomplete    bool
	buckets       map[string]Bucket
	current       Current
}

func NewMeter(id string, now time.Time) *Meter {
	return &Meter{runtimeID: id, started: now, last: now, buckets: map[string]Bucket{}, current: Current{State: "warming_up"}}
}
func (m *Meter) Add(direction, n int) {
	if m != nil && direction >= 0 && direction < 4 && n > 0 {
		m.counters[direction].Add(uint64(n))
	}
}
func (m *Meter) Sample(now time.Time) {
	m.sample(now, false)
}

// SampleFinal preserves all remaining bytes without promoting an unscheduled
// shutdown fragment to a one-second peak.
func (m *Meter) SampleFinal(now time.Time) {
	m.sample(now, true)
}

func (m *Meter) sample(now time.Time, final bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	seconds := now.Sub(m.last).Seconds()
	if seconds <= 0 {
		return
	}
	counts := Bytes{m.counters[0].Load(), m.counters[1].Load(), m.counters[2].Load(), m.counters[3].Load()}
	delta := counts.Sub(m.previous)
	m.previous = counts
	m.last = now
	startupFragment := m.seq == 0 && seconds < 1
	m.seq++
	rates := map[string]float64{}
	for i, n := range delta.Vector() {
		rates[Keys[i]] = float64(n) / seconds
	}
	peaks := PeaksFromRates(rates, now, seconds)
	if final || startupFragment {
		peaks = nil
	}
	state := "sampled"
	if peaks == nil {
		state = "invalid_interval"
	}
	m.current = Current{rates, now, seconds, state}
	for _, g := range []struct {
		name     string
		duration time.Duration
	}{{"minute", time.Minute}, {"hour", time.Hour}} {
		start := now.UTC().Truncate(g.duration)
		key := g.name + start.Format(time.RFC3339)
		b := m.buckets[key]
		b.Start = start
		b.Interval = g.name
		b.Bytes.Add(delta)
		if b.Peaks == nil {
			b.Peaks = Peaks{}
		}
		b.Peaks.Merge(peaks)
		b.Quality.Observe(now, peaks != nil)
		b.Sequence = m.seq
		m.buckets[key] = b
	}
	// Bound the unsaved backlog. Explicitly report loss instead of blocking I/O.
	for key, b := range m.buckets {
		if b.Start.Before(now.Add(-30 * time.Minute).Truncate(time.Hour)) {
			delete(m.buckets, key)
			m.incomplete = true
		}
	}
}
func (m *Meter) Snapshot(closed bool) Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := Snapshot{RuntimeID: m.runtimeID, StartedAt: m.started, At: m.last, Sequence: m.seq, Closed: closed, Incomplete: m.incomplete}
	for _, b := range m.buckets {
		b.Peaks = b.Peaks.Clone()
		s.Buckets = append(s.Buckets, b)
	}
	return s
}
func (m *Meter) Ack(s Snapshot) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, b := range s.Buckets {
		g := time.Minute
		if b.Interval == "hour" {
			g = time.Hour
		}
		key := b.Interval + b.Start.Format(time.RFC3339)
		if current, ok := m.buckets[key]; ok && current.Sequence == b.Sequence && b.Start.Before(m.last.UTC().Truncate(g)) {
			delete(m.buckets, key)
		}
	}
}
func (m *Meter) Current() Current {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.current
	c.Rates = map[string]float64{}
	for k, v := range m.current.Rates {
		c.Rates[k] = v
	}
	return c
}
