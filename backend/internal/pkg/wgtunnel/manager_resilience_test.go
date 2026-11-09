package wgtunnel

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type resilientTestStore struct {
	*memoryNodeStore
	controlMu sync.Mutex
	renewErr  error
	renewHook func(context.Context) error
	saveHook  func(context.Context, Checkpoint) error
	claimHook func(context.Context) error
	renewals  atomic.Int64
}

func (s *resilientTestStore) ClaimRuntime(ctx context.Context, _ int64, _ int64, _ string) error {
	if s.claimHook != nil {
		return s.claimHook(ctx)
	}
	return nil
}
func (s *resilientTestStore) RenewRuntime(ctx context.Context, _ int64, _ int64, _ string) error {
	s.renewals.Add(1)
	s.controlMu.Lock()
	err, hook := s.renewErr, s.renewHook
	s.controlMu.Unlock()
	if hook != nil {
		return hook(ctx)
	}
	return err
}
func (s *resilientTestStore) SaveCheckpoint(ctx context.Context, c Checkpoint, gap bool) error {
	s.controlMu.Lock()
	hook := s.saveHook
	s.controlMu.Unlock()
	if hook != nil {
		if err := hook(ctx, c); err != nil {
			return err
		}
	}
	return s.memoryNodeStore.SaveCheckpoint(ctx, c, gap)
}
func leasedTestManager(t *testing.T) (*Manager, *resilientTestStore) {
	m, base := managedTestNode(t)
	store := &resilientTestStore{memoryNodeStore: base}
	m.mu.Lock()
	m.store = store
	m.mu.Unlock()
	return m, store
}

func TestTransientControlFailureDoesNotLatchOrExtendLease(t *testing.T) {
	m, store := leasedTestManager(t)
	r, _, id, err := m.Acquire(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	current := m.nodes[1]
	m.mu.Unlock()
	current.leaseMu.Lock()
	deadline := current.leaseDeadline
	current.leaseMu.Unlock()
	store.controlMu.Lock()
	store.renewErr = context.DeadlineExceeded
	store.controlMu.Unlock()
	m.validateRuntimes()
	if r.ctx.Err() != nil {
		t.Fatal("transient renewal error closed a confirmed runtime")
	}
	current.leaseMu.Lock()
	unchanged := current.leaseDeadline.Equal(deadline)
	current.leaseMu.Unlock()
	if !unchanged {
		t.Fatal("failed renewal extended the lease")
	}
	// New requests still fail closed when their authoritative read is unavailable.
	store.mu.Lock()
	store.failed = true
	store.mu.Unlock()
	if _, _, _, err = m.Acquire(context.Background(), 1); !errors.Is(err, ErrBlocked) {
		t.Fatal("unverified request allowed")
	}
	if r.ctx.Err() != nil {
		t.Fatal("one failed read killed the shared leased runtime")
	}
	store.mu.Lock()
	store.failed = false
	store.mu.Unlock()
	store.controlMu.Lock()
	store.renewErr = nil
	store.controlMu.Unlock()
	m.validateRuntimes()
	reused, _, newID, err := m.Acquire(context.Background(), 1)
	if err != nil || reused != r || newID != id {
		t.Fatal("recovery required manual resume", err)
	}
}

func TestAuthoritativeLeaseRejectionStillClosesImmediately(t *testing.T) {
	m, store := leasedTestManager(t)
	r, _, _, err := m.Acquire(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	store.controlMu.Lock()
	store.renewErr = ErrBlocked
	store.controlMu.Unlock()
	m.validateRuntimes()
	if r.ctx.Err() == nil {
		t.Fatal("lost ownership retained a live tunnel")
	}
	store.controlMu.Lock()
	store.renewErr = nil
	store.controlMu.Unlock()
	if _, _, _, err = m.Acquire(context.Background(), 1); !errors.Is(err, ErrBlocked) {
		t.Fatal("rejected runtime silently resumed")
	}
}

func TestManualPauseStillClosesDuringDatabaseGrace(t *testing.T) {
	m, store := leasedTestManager(t)
	r, _, _, err := m.Acquire(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	store.controlMu.Lock()
	store.renewErr = context.DeadlineExceeded
	store.controlMu.Unlock()
	m.validateRuntimes()
	store.mu.Lock()
	store.node.Paused = true
	store.mu.Unlock()
	m.validateRuntimes()
	if r.ctx.Err() == nil {
		t.Fatal("manual pause was delayed by the grace period")
	}
}

func TestLeaseDeadlineUsesRequestStartAndCannotBeRevived(t *testing.T) {
	base := time.Now()
	n := &managedRuntime{leaseDeadline: base.Add(runtimeLeaseGrace), renew: make(chan struct{}, 1)}
	// A reply delayed by 14 seconds must not grant 20 fresh seconds from receipt.
	if !n.confirmLease(base.Add(time.Second), base.Add(15*time.Second)) {
		t.Fatal("valid delayed confirmation rejected")
	}
	if want := base.Add(21 * time.Second); !n.leaseDeadline.Equal(want) {
		t.Fatalf("deadline %v, want %v", n.leaseDeadline, want)
	}
	if n.confirmLease(base.Add(20*time.Second), base.Add(22*time.Second)) {
		t.Fatal("late reply revived an expired runtime")
	}
	if n.leaseValid(base.Add(22 * time.Second)) {
		t.Fatal("expired runtime stayed valid")
	}
}

func TestIndependentWatchdogClosesWhenControlLoopCannotRun(t *testing.T) {
	m, _ := managedTestNode(t)
	r, _, _, err := m.Acquire(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	n := &managedRuntime{runtime: r, version: 1, leaseDeadline: time.Now().Add(30 * time.Millisecond), renew: make(chan struct{}, 1)}
	done := make(chan struct{})
	go func() { n.leaseWatchdog(1); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watchdog waited for database or manager")
	}
	if r.ctx.Err() == nil {
		t.Fatal("expired lease retained runtime")
	}
	if n.confirmLease(time.Now(), time.Now()) {
		t.Fatal("watchdog-closed runtime was revived")
	}
}

func TestCheckpointStallDoesNotStarveLeaseRenewal(t *testing.T) {
	t.Parallel()
	m, store := leasedTestManager(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	store.saveHook = func(_ context.Context, c Checkpoint) error {
		if !c.Closed {
			once.Do(func() { close(entered); <-release })
		}
		return nil
	}
	defer close(release)
	r, _, _, err := m.Acquire(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(7 * time.Second):
		t.Fatal("checkpoint not reached")
	}
	before := store.renewals.Load()
	// Deliberately outlive both the old 10s watchdog and the new 20s window.
	select {
	case <-r.ctx.Done():
		t.Fatal("statistics write killed the tunnel")
	case <-time.After(22 * time.Second):
	}
	if store.renewals.Load()-before < 15 {
		t.Fatal("renewal loop stalled behind persistence")
	}
}

func TestFourteenSecondStorageStallRecoversWithoutManualResume(t *testing.T) {
	t.Parallel()
	m, store := leasedTestManager(t)
	entered := make(chan struct{})
	var once sync.Once
	store.renewHook = func(ctx context.Context) error {
		once.Do(func() { close(entered); time.Sleep(14 * time.Second) })
		return ctx.Err()
	}
	r, _, id, err := m.Acquire(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("renewal not reached")
	}
	select {
	case <-r.ctx.Done():
		t.Fatal("observed 14s incident still killed runtime")
	case <-time.After(16 * time.Second):
	}
	if store.renewals.Load() < 2 {
		t.Fatal("renewal did not retry after storage recovered")
	}
	reused, _, newID, err := m.Acquire(context.Background(), 1)
	if err != nil || reused != r || newID != id {
		t.Fatal("recovery required a new generation", err)
	}
}

func TestLateLeaseGrantDoesNotStartRuntime(t *testing.T) {
	t.Parallel()
	m, store := leasedTestManager(t)
	store.claimHook = func(context.Context) error {
		time.Sleep(runtimeLeaseGrace + time.Second)
		return nil
	}
	if _, _, _, err := m.Acquire(context.Background(), 1); !errors.Is(err, ErrBlocked) {
		t.Fatal("late initial grant started a runtime", err)
	}
	if id, _ := m.Identity(1); id != "" {
		t.Fatal("late grant left an active runtime")
	}
}

func TestExpiredGraceStillLatchesDatabaseFailure(t *testing.T) {
	m, store := leasedTestManager(t)
	r, _, _, err := m.Acquire(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	current := m.nodes[1]
	m.mu.Unlock()
	current.leaseMu.Lock()
	current.leaseDeadline = time.Now().Add(-time.Second)
	current.leaseMu.Unlock()
	store.controlMu.Lock()
	store.renewErr = context.DeadlineExceeded
	store.controlMu.Unlock()
	m.validateRuntimes()
	if r.ctx.Err() == nil {
		t.Fatal("expired grace allowed continued UDP")
	}
	store.controlMu.Lock()
	store.renewErr = nil
	store.controlMu.Unlock()
	if _, _, _, err = m.Acquire(context.Background(), 1); !errors.Is(err, ErrBlocked) {
		t.Fatal("expired runtime automatically restarted", err)
	}
}

func TestReadFailureAfterLeaseExpiryCannotRestartRuntime(t *testing.T) {
	m, store := leasedTestManager(t)
	r, _, _, err := m.Acquire(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	current := m.nodes[1]
	m.mu.Unlock()
	current.leaseMu.Lock()
	current.leaseDeadline = time.Now().Add(-time.Second)
	current.leaseMu.Unlock()
	store.mu.Lock()
	store.failed = true
	store.mu.Unlock()
	if _, _, _, err = m.Acquire(context.Background(), 1); !errors.Is(err, ErrBlocked) {
		t.Fatal("unverified request allowed", err)
	}
	if r.ctx.Err() == nil {
		t.Fatal("expired runtime remained open")
	}
	store.mu.Lock()
	store.failed = false
	store.mu.Unlock()
	if _, _, _, err = m.Acquire(context.Background(), 1); !errors.Is(err, ErrBlocked) {
		t.Fatal("read failure cleared the expired runtime latch", err)
	}
}
