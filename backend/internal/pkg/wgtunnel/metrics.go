package wgtunnel

import (
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"sync"
	"time"
)

const MetricBasis = "wg_udp_payload_v1"

// Checkpoint is an absolute snapshot. Persistence must compare Seq and subtract
// the previously committed counters in the same transaction as bucket updates.
type Checkpoint struct {
	NodeID    int64
	RuntimeID string
	Version   int64
	Seq       int64
	At        time.Time
	TX        uint64
	RX        uint64
	Closed    bool
	Buckets   []Bucket
}

type Bucket struct {
	Start       time.Time            `json:"start"`
	TX          uint64               `json:"tx_bytes,string"`
	RX          uint64               `json:"rx_bytes,string"`
	PeakHistory *traffic.PeakHistory `json:"peak_history"`
}

type Rate struct {
	TX            float64   `json:"tx_bytes_per_second"`
	RX            float64   `json:"rx_bytes_per_second"`
	WindowSeconds float64   `json:"window_seconds"`
	SampledAt     time.Time `json:"sampled_at"`
	State         string    `json:"state"`
}

type sample struct {
	at     time.Time
	counts UDPCounts
}

// Meter holds only counters, not packets or credentials. At most 30 minutes of
// unsaved minute buckets are retained; loss of detail is explicitly reported.
type Meter struct {
	mu      sync.Mutex
	counts  *Counters
	last    sample
	buckets map[int64]Bucket
	seq     int64
	gap     bool
}

func NewMeter(c *Counters, now time.Time) *Meter {
	initial := sample{at: now}
	return &Meter{counts: c, last: initial, buckets: make(map[int64]Bucket)}
}

func (m *Meter) Sample(now time.Time) (Rate, error) {
	return m.sample(now, false)
}

// SampleFinal records the tail counters when a runtime is retired. This
// unscheduled lifecycle sample must not compete with periodic one-second peaks.
func (m *Meter) SampleFinal(now time.Time) (Rate, error) {
	return m.sample(now, true)
}

func (m *Meter) sample(now time.Time, final bool) (Rate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.counts.Snapshot()
	if now.Sub(m.last.at) <= 0 {
		return Rate{State: "no_sample"}, errors.New("non-positive sample interval")
	}
	if c.TX < m.last.counts.TX || c.RX < m.last.counts.RX {
		m.gap = true
		return Rate{State: "counter_reset"}, errors.New("counter reset requires a new runtime")
	}
	// Assign deltas to the interval end. Window boundaries have one-sample
	// uncertainty; lifetime counters are not estimated from these buckets.
	minute := now.UTC().Truncate(time.Minute)
	b := m.buckets[minute.Unix()]
	b.Start = minute
	b.TX += c.TX - m.last.counts.TX
	b.RX += c.RX - m.last.counts.RX
	seconds := now.Sub(m.last.at).Seconds()
	txRate, rxRate := float64(c.TX-m.last.counts.TX)/seconds, float64(c.RX-m.last.counts.RX)/seconds
	peaks := traffic.PeaksFromRates(map[string]float64{"tx": txRate, "rx": rxRate, "total": txRate + rxRate}, now, seconds)
	// Nodes start between ticks of the manager's shared sampling clock. Their
	// first tick may therefore cover only a fraction of a second.
	if final || (m.seq == 0 && seconds < 1) {
		peaks = nil
	}
	if b.PeakHistory == nil {
		b.PeakHistory = &traffic.PeakHistory{Peaks: traffic.Peaks{}}
	}
	b.PeakHistory.Peaks.Merge(peaks)
	b.PeakHistory.Quality.Observe(now, peaks != nil)
	m.buckets[minute.Unix()] = b
	for k := range m.buckets {
		if k < minute.Add(-30*time.Minute).Unix() {
			delete(m.buckets, k)
			m.gap = true
		}
	}
	m.last = sample{now, c}
	m.seq++
	state := "sampled"
	if peaks == nil {
		state = "invalid_interval"
	}
	return Rate{TX: txRate, RX: rxRate, WindowSeconds: seconds, SampledAt: now, State: state}, nil
}

func (m *Meter) Checkpoint(node int64, runtime string, version int64, closed bool) (Checkpoint, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := Checkpoint{NodeID: node, RuntimeID: runtime, Version: version, Seq: m.seq, At: m.last.at, TX: m.last.counts.TX, RX: m.last.counts.RX, Closed: closed}
	for _, b := range m.buckets {
		b.PeakHistory = b.PeakHistory.Clone()
		c.Buckets = append(c.Buckets, b)
	}
	return c, m.gap
}

// Ack only retires completed minute buckets represented by the acknowledged
// snapshot. The open bucket remains cumulative for idempotent upserts.
func (m *Meter) Ack(c Checkpoint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current := m.last.at.UTC().Truncate(time.Minute)
	for _, b := range c.Buckets {
		if b.Start.Before(current) {
			if live, ok := m.buckets[b.Start.Unix()]; ok && live.TX == b.TX && live.RX == b.RX && samePeakSamples(live.PeakHistory, b.PeakHistory) {
				delete(m.buckets, b.Start.Unix())
			}
		}
	}
}

func samePeakSamples(a, b *traffic.PeakHistory) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Quality.ValidSamples == b.Quality.ValidSamples && a.Quality.InvalidSamples == b.Quality.InvalidSamples
}
