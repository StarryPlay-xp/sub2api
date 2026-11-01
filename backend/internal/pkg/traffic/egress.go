package traffic

import (
	"sync/atomic"
	"time"
)

const MaxEgressCombinations = 5000
const maxEgressLeaves = 40000
const maxEgressRoutes = 1024

// EgressIdentity contains only database IDs. No connection material is retained.
type EgressIdentity struct {
	Type      string
	ID        int64
	AccountID int64
}
type EgressBytes struct {
	Out uint64 `json:"upstream_out,string"`
	In  uint64 `json:"upstream_in,string"`
}

func (b EgressBytes) Total() uint64      { return b.Out + b.In }
func (b *EgressBytes) Add(v EgressBytes) { b.Out += v.Out; b.In += v.In }

type EgressBucket struct {
	EgressIdentity
	Start         time.Time
	Bytes         EgressBytes
	Attempts      uint64
	Peaks         Peaks
	Quality       Quality
	Sequence      uint64
	Overflow      bool
	savedSequence uint64
}
type egressLeafKey struct {
	subject  *subjectCounter
	identity EgressIdentity
}
type egressBucketKey struct {
	identity EgressIdentity
	start    time.Time
}
type egressCounter struct {
	key              egressLeafKey
	bytes            [2]atomic.Uint64
	attempts         atomic.Uint64
	previous         EgressBytes
	previousAttempts uint64
	refs             int
	lastTouched      time.Time
	overflow         bool
}
type egressDelta struct {
	bytes    EgressBytes
	attempts uint64
	overflow bool
}

func (m *Meter) effective() *Meter {
	for m != nil {
		source := m.source.Load()
		if source == nil {
			return m
		}
		m = source
	}
	return nil
}
func (m *Meter) HasEgress(identity EgressIdentity) bool {
	m = m.effective()
	return m != nil && m.egressIdentity != nil && *m.egressIdentity == identity
}

// EgressScope pins the user counter until all leaf bytes are sampled. Upstream
// I/O updates this leaf ONLY; sampling derives the user, route and host totals.
// account -1 is the residual bucket, while account 0 is reserved for route totals.
func (m *Meter) EgressScope(identity EgressIdentity) *Meter { return m.egressScope(identity, true) }
func (m *Meter) EgressIdleScope(identity EgressIdentity) *Meter {
	return m.egressScope(identity, false)
}
func (m *Meter) egressScope(identity EgressIdentity, attempt bool) *Meter {
	m = m.effective()
	if m == nil {
		return nil
	}
	r := m.Root()
	r.mu.Lock()
	defer r.mu.Unlock()
	c := m.target.Load()
	if c == nil {
		c = r.unattributed
	}
	scoped := &Meter{root: r, egressIdentity: &identity}
	scoped.target.Store(c)
	if r.egressCounters == nil {
		r.egressCounters = map[egressLeafKey]*egressCounter{}
		r.egressCombinations = map[EgressIdentity]int{}
		r.egressRoutes = map[EgressIdentity]int{}
		if r.egressBuckets == nil {
			r.egressBuckets = map[egressBucketKey]EgressBucket{}
		}
	}
	key := identity
	if key.AccountID <= 0 {
		key.AccountID = -1
	}
	route := EgressIdentity{Type: key.Type, ID: key.ID}
	valid := (key.Type == "direct" && key.ID == 0) || ((key.Type == "proxy" || key.Type == "wireguard") && key.ID > 0)
	overflow := false
	limit := r.egressLimit
	if limit == 0 {
		limit = MaxEgressCombinations
	}
	_, known := r.egressBuckets[egressBucketKey{key, r.last.UTC().Truncate(time.Hour)}]
	if key.AccountID > 0 && r.egressCombinations[key] == 0 && (len(r.egressCombinations) >= limit || (!known && len(r.egressBuckets) >= limit*2)) {
		key.AccountID = -1
		overflow = true
	}
	leafKey := egressLeafKey{c, key}
	leaf := r.egressCounters[leafKey]
	if !valid || (leaf == nil && (len(r.egressCounters) >= maxEgressLeaves || (r.egressRoutes[route] == 0 && len(r.egressRoutes) >= maxEgressRoutes))) {
		// A second bound limits user×route fan-out. Preserve user/host bytes as
		// unidentified when even a residual leaf cannot be allocated.
		c.refs++
		scoped.egressFallback = c
		if attempt {
			r.egressUnidentifiedAttempts++
		}
		if valid {
			scoped.egressOverflowed = true
			r.egressOverflowScopes++
			r.egressOverflow = true
		}
		return scoped
	}
	if leaf == nil {
		leaf = &egressCounter{key: leafKey, lastTouched: r.last, overflow: overflow}
		r.egressCounters[leafKey] = leaf
		c.refs++
		if key.AccountID > 0 {
			r.egressCombinations[key]++
		}
		r.egressRoutes[route]++
	}
	leaf.overflow = leaf.overflow || overflow
	leaf.refs++
	if attempt {
		leaf.attempts.Add(1)
	}
	scoped.egress = leaf
	return scoped
}
func (m *Meter) CloseEgress() {
	if m == nil || m.egressIdentity == nil {
		return
	}
	r := m.Root()
	r.mu.Lock()
	defer r.mu.Unlock()
	if m.egressClosed {
		return
	}
	m.egressClosed = true
	// Late I/O remains visible in the host's unidentified/unattributed counters.
	m.source.Store(r)
	if m.egress != nil {
		m.egress.refs--
	}
	if m.egressFallback != nil {
		m.egressFallback.refs--
	}
	if m.egressOverflowed {
		r.egressOverflowScopes--
	}
}
func (m *Meter) sampleEgressCounters(now time.Time) (map[*subjectCounter]Bytes, map[EgressIdentity]egressDelta) {
	subjects := map[*subjectCounter]Bytes{}
	deltas := map[EgressIdentity]egressDelta{}
	for key, c := range m.egressCounters {
		counts := EgressBytes{c.bytes[0].Load(), c.bytes[1].Load()}
		b := EgressBytes{counts.Out - c.previous.Out, counts.In - c.previous.In}
		c.previous = counts
		attempts := c.attempts.Load()
		n := attempts - c.previousAttempts
		c.previousAttempts = attempts
		if b.Total() > 0 || n > 0 || c.refs > 0 {
			c.lastTouched = now
		}
		extra := subjects[key.subject]
		extra.UpstreamOut += b.Out
		extra.UpstreamIn += b.In
		subjects[key.subject] = extra
		if b.Total() > 0 || n > 0 || c.refs > 0 {
			d := deltas[key.identity]
			d.bytes.Add(b)
			d.attempts += n
			d.overflow = d.overflow || c.overflow
			deltas[key.identity] = d
		}
		if c.refs == 0 && now.Sub(c.lastTouched) > 60*time.Second {
			delete(m.egressCounters, key)
			key.subject.refs--
			route := EgressIdentity{Type: key.identity.Type, ID: key.identity.ID}
			m.egressRoutes[route]--
			if m.egressRoutes[route] == 0 {
				delete(m.egressRoutes, route)
			}
			if key.identity.AccountID > 0 {
				m.egressCombinations[key.identity]--
				if m.egressCombinations[key.identity] == 0 {
					delete(m.egressCombinations, key.identity)
				}
			}
		}
	}
	return subjects, deltas
}
func (m *Meter) observeEgress(now time.Time, seconds float64, valid bool, deltas map[EgressIdentity]egressDelta, unknown EgressBytes) {
	if m.egressBuckets == nil {
		m.egressBuckets = map[egressBucketKey]EgressBucket{}
	}
	totals := map[EgressIdentity]egressDelta{}
	for key, d := range deltas {
		route := EgressIdentity{Type: key.Type, ID: key.ID}
		v := totals[route]
		v.bytes.Add(d.bytes)
		v.attempts += d.attempts
		v.overflow = v.overflow || d.overflow
		totals[route] = v
	}
	for key, d := range totals {
		deltas[key] = d
	}
	deltas[EgressIdentity{Type: "unidentified"}] = egressDelta{bytes: unknown, attempts: m.egressUnidentifiedAttempts, overflow: m.egressOverflow || m.egressOverflowScopes > 0}
	m.egressUnidentifiedAttempts = 0
	for identity, d := range deltas {
		key := egressBucketKey{identity, now.UTC().Truncate(time.Hour)}
		b := m.egressBuckets[key]
		b.EgressIdentity = identity
		b.Start = key.start
		b.Bytes.Add(d.bytes)
		b.Attempts += d.attempts
		b.Sequence = m.seq
		b.Overflow = b.Overflow || d.overflow
		if valid {
			if b.Peaks == nil {
				b.Peaks = Peaks{}
			}
			b.Peaks.Merge(PeaksFromRates(map[string]float64{"out": float64(d.bytes.Out) / seconds, "in": float64(d.bytes.In) / seconds, "total": float64(d.bytes.Total()) / seconds}, now, seconds))
		}
		b.Quality.Observe(now, valid)
		m.egressBuckets[key] = b
	}
	for key, b := range m.egressBuckets {
		if b.Start.Before(now.Add(-30 * time.Minute).Truncate(time.Hour)) {
			delete(m.egressBuckets, key)
			m.incomplete = true
		}
	}
	m.egressOverflow = false
}
