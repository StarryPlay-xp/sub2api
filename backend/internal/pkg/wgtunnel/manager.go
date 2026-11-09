package wgtunnel

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

type Node struct {
	ID         int64
	Version    int64
	Ciphertext string
	Active     bool
	Paused     bool
	ExpiresAt  *time.Time
}

type Store interface {
	LoadNode(context.Context, int64) (Node, error)
	SaveCheckpoint(context.Context, Checkpoint, bool) error
}

type RuntimeStore interface {
	ClaimRuntime(context.Context, int64, int64, string) error
	RenewRuntime(context.Context, int64, int64, string) error
}

type retiredRuntime struct {
	checkpoint Checkpoint
	gap        bool
}

type managedRuntime struct {
	runtime         *Runtime
	meter           *Meter
	id              string
	version         int64
	rate            Rate
	renew           chan struct{}
	leaseMu         sync.Mutex
	leaseDeadline   time.Time
	leaseExpired    bool
	controlDegraded bool
}

// Manager is per-process and owns the only network runtime for each node.
// Persistent identity is rechecked before handing out a runtime; disabled,
// expired, missing or corrupt configurations never yield a direct dialer.
type Manager struct {
	mu      sync.Mutex
	store   Store
	box     *SecretBox
	nodes   map[int64]*managedRuntime
	failed  map[int64]int64
	retired map[string]retiredRuntime
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
}

func NewManager(store Store, box *SecretBox) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{store: store, box: box, nodes: make(map[int64]*managedRuntime), failed: make(map[int64]int64), retired: make(map[string]retiredRuntime), ctx: ctx, cancel: cancel, done: make(chan struct{})}
	go m.loop()
	return m
}

func (m *Manager) Acquire(ctx context.Context, id int64) (*Runtime, int64, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ctx.Err() != nil || m.store == nil {
		return nil, 0, "", ErrBlocked
	}
	n, err := m.store.LoadNode(ctx, id)
	if ctx.Err() != nil {
		return nil, 0, "", ctx.Err()
	}
	if err != nil {
		// A caller still fails closed, but a transient database read failure must
		// not retire another request's runtime while its confirmed lease is valid.
		if current := m.nodes[id]; current != nil && !controlPlaneRejected(err) && current.hasLease() {
			if current.leaseValid(time.Now()) {
				return nil, 0, "", ErrBlocked
			}
			// Keep expiry latched even when this read error wins the race with
			// validation; retiring alone would permit a fresh claim on recovery.
			m.failed[id] = current.version
			go m.persistFailure(id, current.version)
		}
		m.retireLocked(id)
		return nil, 0, "", ErrBlocked
	}
	if !n.Active || n.Paused || (n.ExpiresAt != nil && !n.ExpiresAt.After(time.Now())) {
		m.retireLocked(id)
		return nil, 0, "", ErrBlocked
	}
	if m.failed[id] == n.Version {
		return nil, 0, "", ErrBlocked
	}
	if current := m.nodes[id]; current != nil {
		if !current.leaseValid(time.Now()) {
			current.runtime.Close()
		}
		if current.version == n.Version && current.runtime.ctx.Err() == nil {
			return current.runtime, n.Version, current.id, nil
		}
		if current.version == n.Version && current.runtime.ctx.Err() != nil {
			m.failed[id] = n.Version
			m.retireLocked(id)
			go m.persistFailure(id, n.Version)
			return nil, 0, "", ErrBlocked
		}
		m.retireLocked(id)
	}
	for rid, old := range m.retired {
		if old.checkpoint.NodeID == id {
			saveCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			err := m.store.SaveCheckpoint(saveCtx, old.checkpoint, old.gap)
			cancel()
			if err != nil {
				return nil, 0, "", ErrBlocked
			}
			delete(m.retired, rid)
		}
	}
	// Bound unsaved retired runtimes instead of silently dropping final totals.
	if len(m.retired) >= 64 {
		return nil, 0, "", errors.New("WireGuard statistics backlog; node startup blocked")
	}
	raw, err := m.box.Open(id, n.Ciphertext)
	if err != nil {
		return nil, 0, "", ErrBlocked
	}
	cfg, err := Parse(raw)
	if err != nil {
		return nil, 0, "", ErrBlocked
	}
	b := make([]byte, 16)
	if _, err = rand.Read(b); err != nil {
		return nil, 0, "", ErrBlocked
	}
	rid := hex.EncodeToString(b)
	leaseStarted := time.Now()
	_, leased := m.store.(RuntimeStore)
	if lifecycle, ok := m.store.(RuntimeStore); ok {
		if err = lifecycle.ClaimRuntime(ctx, id, n.Version, rid); err != nil {
			return nil, 0, "", ErrBlocked
		}
	}
	if leased && !time.Now().Before(leaseStarted.Add(runtimeLeaseGrace)) {
		// Never start UDP on a grant whose acknowledgement arrived too late.
		m.failed[id] = n.Version
		go m.persistFailure(id, n.Version)
		return nil, 0, "", ErrBlocked
	}
	r, err := Start(ctx, cfg)
	if err != nil {
		if ctx.Err() == nil {
			m.failed[id] = n.Version
			go m.persistFailure(id, n.Version)
		}
		m.retired[rid] = retiredRuntime{checkpoint: Checkpoint{NodeID: id, Version: n.Version, RuntimeID: rid, Seq: 0, At: time.Now(), Closed: true}}
		return nil, 0, "", ErrBlocked
	}
	current := &managedRuntime{runtime: r, meter: NewMeter(&r.Counts, time.Now()), id: rid, version: n.Version, rate: Rate{State: "warming_up"}, renew: make(chan struct{}, 1)}
	if leased {
		// Anchor to the start of the database operation, never a delayed reply.
		current.leaseDeadline = leaseStarted.Add(runtimeLeaseGrace)
	}
	m.nodes[id] = current
	if leased {
		if !current.leaseValid(time.Now()) {
			m.failed[id] = n.Version
			m.retireLocked(id)
			go m.persistFailure(id, n.Version)
			return nil, 0, "", ErrBlocked
		}
		go current.leaseWatchdog(id)
	}
	return r, n.Version, rid, nil
}

func (m *Manager) Revoke(id int64) { m.mu.Lock(); defer m.mu.Unlock(); m.retireLocked(id) }
func (m *Manager) retireLocked(id int64) {
	n := m.nodes[id]
	if n == nil {
		return
	}
	n.runtime.Close()
	_, _ = n.meter.SampleFinal(time.Now())
	c, gap := n.meter.Checkpoint(id, n.id, n.version, true)
	m.retired[n.id] = retiredRuntime{c, gap}
	delete(m.nodes, id)
}
func (m *Manager) Rate(id int64) Rate {
	m.mu.Lock()
	defer m.mu.Unlock()
	if n := m.nodes[id]; n != nil {
		return n.rate
	}
	return Rate{State: "not_running"}
}
func (m *Manager) Close() { m.cancel(); <-m.done }
func (m *Manager) loop() {
	defer close(m.done)
	// Store latency must not stretch a normal one-second sample into a multi-
	// second average. Persistence must also never delay lease renewal.
	maintained := make(chan struct{})
	go func() {
		defer close(maintained)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-m.ctx.Done():
				return
			case <-ticker.C:
				m.validateRuntimes()
			}
		}
	}()
	persisted := make(chan struct{})
	go func() {
		defer close(persisted)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-m.ctx.Done():
				return
			case <-ticker.C:
				m.flush()
			}
		}
	}()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.mu.Lock()
			now := time.Now()
			for _, n := range m.nodes {
				n.rate, _ = n.meter.Sample(now)
			}
			m.mu.Unlock()
		case <-m.ctx.Done():
			<-maintained
			<-persisted
			m.mu.Lock()
			for id := range m.nodes {
				m.retireLocked(id)
			}
			m.mu.Unlock()
			m.flush()
			return
		}
	}
}
func (m *Manager) flush() {
	type pending struct {
		c     Checkpoint
		gap   bool
		meter *Meter
	}
	m.mu.Lock()
	items := make([]pending, 0, len(m.nodes)+len(m.retired))
	for id, n := range m.nodes {
		c, g := n.meter.Checkpoint(id, n.id, n.version, false)
		items = append(items, pending{c, g, n.meter})
	}
	for _, r := range m.retired {
		items = append(items, pending{r.checkpoint, r.gap, nil})
	}
	m.mu.Unlock()
	for _, p := range items {
		started := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := m.store.SaveCheckpoint(ctx, p.c, p.gap)
		cancel()
		if err != nil {
			logControlEvent("wireguard.checkpoint_failed", p.c.NodeID, p.c.Version, "checkpoint", err, time.Since(started))
			continue
		}
		if p.meter != nil {
			p.meter.Ack(p.c)
		}
		if p.c.Closed {
			m.mu.Lock()
			delete(m.retired, p.c.RuntimeID)
			m.mu.Unlock()
		}
	}
}

// Persistent edits also invalidate already-open streams, not just new callers.
// A snapshot/version check prevents an old validation result revoking a newer
// configuration installed concurrently by Acquire.
func (m *Manager) validateRuntimes() {
	m.mu.Lock()
	snapshot := make(map[int64]*managedRuntime, len(m.nodes))
	for id, n := range m.nodes {
		snapshot[id] = n
	}
	m.mu.Unlock()
	for id, current := range snapshot {
		started := time.Now()
		stage := "load_node"
		ctx, cancel := context.WithTimeout(m.ctx, time.Second)
		n, err := m.store.LoadNode(ctx, id)
		valid := err == nil && n.Active && !n.Paused && n.Version == current.version && (n.ExpiresAt == nil || n.ExpiresAt.After(time.Now()))
		if valid {
			if lifecycle, ok := m.store.(RuntimeStore); ok {
				stage = "renew_runtime"
				err = lifecycle.RenewRuntime(ctx, id, current.version, current.id)
				if err == nil && ctx.Err() != nil {
					err = ctx.Err()
				}
				if err == nil && !current.confirmLease(started, time.Now()) {
					err = ErrBlocked
				}
			}
		}
		cancel()
		if m.ctx.Err() != nil {
			return
		}
		if err != nil && !controlPlaneRejected(err) && current.hasLease() && current.leaseValid(time.Now()) && current.runtime.ctx.Err() == nil {
			if !current.controlDegraded {
				logControlEvent("wireguard.control_plane_degraded", id, current.version, stage, err, time.Since(started))
			}
			current.controlDegraded = true
			continue
		}
		if err != nil || current.runtime.ctx.Err() != nil {
			logControlEvent("wireguard.runtime_failed", id, current.version, stage, err, time.Since(started))
			m.Fail(id, current.version)
		}
		if err == nil && valid && current.runtime.ctx.Err() == nil {
			if current.controlDegraded {
				logControlEvent("wireguard.control_plane_recovered", id, current.version, stage, nil, time.Since(started))
				current.controlDegraded = false
			}
			continue
		}
		m.mu.Lock()
		if m.nodes[id] == current {
			m.retireLocked(id)
		}
		m.mu.Unlock()
	}
}

// Fail latches a confirmed tunnel/control-plane failure locally even if its
// database write fails. Only an explicit resume with a new generation clears it.
func (m *Manager) Fail(id, version int64) {
	m.mu.Lock()
	if current := m.nodes[id]; current != nil && current.version != version {
		m.mu.Unlock()
		return
	}
	m.failed[id] = version
	m.retireLocked(id)
	m.mu.Unlock()
	m.persistFailure(id, version)
}

func (m *Manager) persistFailure(id, version int64) {
	if store, ok := m.store.(interface {
		SetNodePaused(context.Context, int64, int64, bool) error
	}); ok {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := store.SetNodePaused(ctx, id, version, true); err != nil {
			logControlEvent("wireguard.pause_persist_failed", id, version, "persist_pause", err, 0)
		}
	}
}

func (m *Manager) Identity(id int64) (string, int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if n := m.nodes[id]; n != nil {
		return n.id, n.version
	}
	return "", 0
}
