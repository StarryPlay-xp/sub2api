package traffic

import (
	"context"
	"sync/atomic"
	"time"
)

const MaxSubjects = 20000

type Subject struct {
	Type   string `json:"subject_type"`
	ID     int64  `json:"subject_id"`
	UserID int64  `json:"user_id"`
}

type subjectCounter struct {
	subject          Subject
	bytes            [4]atomic.Uint64
	requests         atomic.Uint64
	inFlight         atomic.Int64
	refs             int // protected by root.mu
	previous         Bytes
	previousRequests uint64
	lastTouched      time.Time
}

type SubjectBucket struct {
	Subject
	Bucket
	Requests      uint64 `json:"requests"`
	ActiveSeconds uint64 `json:"active_seconds"`
	Overflow      bool   `json:"subject_overflow"`
	savedSequence uint64
}

type subjectBucketKey struct {
	Subject
	Start time.Time
}
type rateSample struct {
	At      time.Time
	Seconds float64
	Bytes   Bytes
}
type SubjectCurrent struct {
	Subject
	Rate           RateTriple `json:"rate"`
	Average        RateTriple `json:"avg_60s"`
	AverageSeconds float64    `json:"avg_60s_seconds"`
	InFlight       int64      `json:"in_flight"`
	LastActiveAt   *time.Time `json:"last_active_at"`
	samples        []rateSample
}
type RateTriple struct {
	In    float64 `json:"in"`
	Out   float64 `json:"out"`
	Total float64 `json:"total"`
}

func triple(b Bytes, seconds float64) RateTriple {
	if seconds <= 0 {
		return RateTriple{}
	}
	v := b.Vector()
	return RateTriple{float64(v[4]) / seconds, float64(v[5]) / seconds, float64(v[6]) / seconds}
}

type SubjectLive struct {
	Current      Current
	Host         SubjectCurrent
	Unattributed SubjectCurrent
	Items        []SubjectCurrent
}

// Scope creates a request-local handle. Before authentication and after CloseScope
// writes go to unattributed. The root's counters are derived from the SAME sampled
// leaf deltas, so concurrent writes cannot break conservation at sample boundaries.
func (m *Meter) Scope() *Meter {
	if m == nil {
		return nil
	}
	r := m.Root()
	s := &Meter{root: r}
	s.target.Store(r.unattributed)
	return s
}
func (m *Meter) Root() *Meter {
	if m == nil {
		return nil
	}
	if m.root != nil {
		return m.root
	}
	return m
}
func (m *Meter) Bind(userID, keyID int64, request bool) {
	if m == nil || m.root == nil || userID <= 0 || keyID <= 0 {
		return
	}
	r := m.root
	r.mu.Lock()
	defer r.mu.Unlock()
	if m.bound || m.ended {
		return
	}
	m.bound = true
	subject := Subject{Type: "api_key", ID: keyID, UserID: userID}
	c := r.subjects[subject]
	if c == nil {
		// Cap both live counters and distinct subjects in an unsaved/current hour.
		_, known := r.subjectBuckets[subjectBucketKey{subject, r.last.UTC().Truncate(time.Hour)}]
		if len(r.subjects) >= r.subjectLimit || (!known && len(r.subjectBuckets) >= r.subjectLimit*2) {
			r.overflow = true
			c = r.unattributed
			r.overflowScopes++
			m.overflowed = true
		} else {
			c = &subjectCounter{subject: subject, lastTouched: r.last}
			r.subjects[subject] = c
		}
	}
	c.refs++
	m.target.Store(c)
	m.request = request
	if request {
		c.requests.Add(1)
		c.inFlight.Add(1)
	}
}
func BindContext(ctx context.Context, userID, keyID int64) {
	FromContext(ctx).Bind(userID, keyID, true)
}
func BackgroundScope(ctx context.Context, userID, keyID int64) (context.Context, func()) {
	m := FromContext(ctx).Scope()
	m.Bind(userID, keyID, false)
	return WithMeter(ctx, m), m.CloseScope
}
func (m *Meter) CloseScope() {
	if m == nil || m.root == nil {
		return
	}
	r := m.root
	r.mu.Lock()
	defer r.mu.Unlock()
	if m.ended {
		return
	}
	m.ended = true
	c := m.target.Swap(r.unattributed)
	if m.bound {
		if m.overflowed {
			r.overflowScopes--
		}
		c.refs--
		if m.request {
			c.inFlight.Add(-1)
		}
	}
}

// Switch creates a stable transport handle whose source can change per WS lease.
// It never owns the source scope; the request/background owner closes that scope.
func (m *Meter) Switch() *Meter {
	if m == nil {
		return nil
	}
	s := &Meter{root: m.Root()}
	s.source.Store(m)
	return s
}
func (m *Meter) SetSource(source *Meter) {
	if m == nil || m.root == nil || source == m {
		return
	}
	if source == nil {
		source = m.Root()
	}
	m.source.Store(source)
}

func (m *Meter) sampleSubjects(now time.Time, seconds float64, valid bool) Bytes {
	type delta struct {
		bytes    Bytes
		requests uint64
		flight   int64
	}
	deltas := map[Subject]delta{}
	var host Bytes
	sample := func(c *subjectCounter) {
		counts := Bytes{c.bytes[0].Load(), c.bytes[1].Load(), c.bytes[2].Load(), c.bytes[3].Load()}
		b := counts.Sub(c.previous)
		c.previous = counts
		requests := c.requests.Load()
		n := requests - c.previousRequests
		c.previousRequests = requests
		flight := c.inFlight.Load()
		if b.Vector()[6] > 0 || n > 0 || c.refs > 0 {
			c.lastTouched = now
		}
		d := delta{b, n, flight}
		deltas[c.subject] = d
		host.Add(b)
		if c.subject.Type == "api_key" {
			u := Subject{Type: "user", ID: c.subject.UserID, UserID: c.subject.UserID}
			v := deltas[u]
			v.bytes.Add(b)
			v.requests += n
			v.flight += flight
			deltas[u] = v
		}
	}
	sample(m.unattributed)
	for s, c := range m.subjects {
		sample(c)
		if c.refs == 0 && now.Sub(c.lastTouched) > 60*time.Second {
			delete(m.subjects, s)
		}
	}
	for s, d := range deltas {
		// Only store observed subjects (and a zero unattributed bucket as coverage).
		if d.bytes.Vector()[6] > 0 || d.requests > 0 || d.flight > 0 || s.Type == "unattributed" {
			key := subjectBucketKey{s, now.UTC().Truncate(time.Hour)}
			b := m.subjectBuckets[key]
			b.Subject = s
			b.Start = key.Start
			b.Interval = "hour"
			b.Sequence = m.seq
			b.Bytes.Add(d.bytes)
			b.Requests += d.requests
			b.Overflow = b.Overflow || m.overflow || m.overflowScopes > 0
			if valid {
				if d.bytes.Vector()[6] > 0 {
					b.ActiveSeconds++
				}
				rates := map[string]float64{}
				v := d.bytes.Vector()
				for i := 4; i < 7; i++ {
					rates[Keys[i]] = float64(v[i]) / seconds
				}
				if b.Peaks == nil {
					b.Peaks = Peaks{}
				}
				b.Peaks.Merge(PeaksFromRates(rates, now, seconds))
			}
			b.Quality.Observe(now, valid)
			m.subjectBuckets[key] = b
		}
		v := m.subjectLive[s]
		v.Subject = s
		v.InFlight = d.flight
		m.updateLive(&v, now, seconds, d.bytes)
		m.subjectLive[s] = v
	}
	for s, v := range m.subjectLive {
		if _, ok := deltas[s]; !ok {
			v.InFlight = 0
			m.updateLive(&v, now, seconds, Bytes{})
			m.subjectLive[s] = v
		}
		if v.InFlight == 0 && (v.LastActiveAt == nil || now.Sub(*v.LastActiveAt) >= 60*time.Second) {
			delete(m.subjectLive, s)
		}
	}
	m.updateLive(&m.hostLive, now, seconds, host)
	for key, b := range m.subjectBuckets {
		if b.Start.Before(now.Add(-30 * time.Minute).Truncate(time.Hour)) {
			delete(m.subjectBuckets, key)
			m.incomplete = true
		}
	}
	m.overflow = false
	return host
}
func (m *Meter) updateLive(v *SubjectCurrent, now time.Time, seconds float64, b Bytes) {
	v.Rate = triple(b, seconds)
	if b.Vector()[6] > 0 {
		t := now
		v.LastActiveAt = &t
	}
	v.samples = append(v.samples, rateSample{now, seconds, b})
	cutoff := now.Add(-60 * time.Second)
	first := 0
	for first < len(v.samples) && !v.samples[first].At.After(cutoff) {
		first++
	}
	v.samples = v.samples[first:]
	if len(v.samples) > 68 {
		v.samples = v.samples[len(v.samples)-68:]
	}
	elapsed := now.Sub(m.started).Seconds()
	if elapsed > 60 {
		elapsed = 60
	}
	v.AverageSeconds = elapsed
	// Fractionally weight the oldest delayed sample crossing the 60-second boundary.
	var in, out float64
	for _, p := range v.samples {
		fraction := 1.0
		if p.Seconds > 0 && p.At.Add(-time.Duration(p.Seconds*float64(time.Second))).Before(cutoff) {
			fraction = p.At.Sub(cutoff).Seconds() / p.Seconds
		}
		x := p.Bytes.Vector()
		in += float64(x[4]) * fraction
		out += float64(x[5]) * fraction
	}
	if elapsed > 0 {
		v.Average = RateTriple{in / elapsed, out / elapsed, (in + out) / elapsed}
	}
}
func (m *Meter) Live(subject string) SubjectLive {
	m = m.Root()
	m.mu.Lock()
	defer m.mu.Unlock()
	result := SubjectLive{Current: m.current, Host: m.hostLive, Items: []SubjectCurrent{}}
	result.Unattributed = m.subjectLive[Subject{Type: "unattributed"}]
	for s, v := range m.subjectLive {
		if s.Type == subject && (v.InFlight > 0 || v.LastActiveAt != nil && m.last.Sub(*v.LastActiveAt) < 60*time.Second) {
			v.samples = nil
			result.Items = append(result.Items, v)
		}
	}
	result.Host.samples = nil
	result.Unattributed.samples = nil
	return result
}
