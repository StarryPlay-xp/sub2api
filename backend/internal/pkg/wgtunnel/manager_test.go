package wgtunnel

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type memoryNodeStore struct {
	mu          sync.Mutex
	node        Node
	failed      bool
	checkpoints map[string]Checkpoint
	gaps        map[string]bool
}

func (s *memoryNodeStore) LoadNode(context.Context, int64) (Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failed {
		return Node{}, errors.New("database unavailable")
	}
	return s.node, nil
}
func (s *memoryNodeStore) SaveCheckpoint(_ context.Context, c Checkpoint, gap bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failed {
		return errors.New("database unavailable")
	}
	s.checkpoints[c.RuntimeID] = c
	s.gaps[c.RuntimeID] = gap
	return nil
}
func managedTestNode(t *testing.T) (*Manager, *memoryNodeStore) {
	t.Helper()
	box, _ := NewSecretBox([]byte(strings.Repeat("k", 32)))
	raw := strings.Replace(exampleConfig(), "vpn.example:51820", "127.0.0.1:51820", 1)
	cipher, err := box.Seal(1, raw)
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryNodeStore{node: Node{ID: 1, Version: 1, Active: true, Ciphertext: cipher}, checkpoints: map[string]Checkpoint{}, gaps: map[string]bool{}}
	m := NewManager(store, box)
	t.Cleanup(m.Close)
	return m, store
}
func TestManagerSharesRuntimeAndRevokesOnStateChange(t *testing.T) {
	m, store := managedTestNode(t)
	r, v, id, err := m.Acquire(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	second, _, secondID, err := m.Acquire(context.Background(), 1)
	if err != nil || second != r || id != secondID || v != 1 {
		t.Fatal("node was not shared")
	}
	store.mu.Lock()
	store.node.Paused = true
	store.mu.Unlock()
	m.validateRuntimes()
	if r.ctx.Err() == nil {
		t.Fatal("paused node left active connections alive")
	}
	if _, _, _, err = m.Acquire(context.Background(), 1); !errors.Is(err, ErrBlocked) {
		t.Fatal("paused node was usable")
	}
	store.mu.Lock()
	store.node.Paused = false
	store.node.Version++
	store.mu.Unlock()
	updated, version, newID, err := m.Acquire(context.Background(), 1)
	if err != nil || updated == r || newID == id || version != 2 {
		t.Fatal("new configuration reused old runtime")
	}
	store.mu.Lock()
	store.failed = true
	store.mu.Unlock()
	m.validateRuntimes()
	if updated.ctx.Err() == nil {
		t.Fatal("unverifiable runtime remained usable")
	}
	store.mu.Lock()
	store.failed = false
	store.mu.Unlock()
}
func TestRetirementPreservesIncompleteStatistics(t *testing.T) {
	m, store := managedTestNode(t)
	_, _, id, err := m.Acquire(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	n := m.nodes[1]
	n.meter.mu.Lock()
	n.meter.gap = true
	n.meter.mu.Unlock()
	m.mu.Unlock()
	m.Revoke(1)
	m.flush()
	store.mu.Lock()
	defer store.mu.Unlock()
	if !store.gaps[id] || !store.checkpoints[id].Closed {
		t.Fatal("retirement lost missing-history indication")
	}
}
func TestManagerRejectsExpiredNode(t *testing.T) {
	m, store := managedTestNode(t)
	past := time.Now().Add(-time.Second)
	store.mu.Lock()
	store.node.ExpiresAt = &past
	store.mu.Unlock()
	if _, _, _, err := m.Acquire(context.Background(), 1); !errors.Is(err, ErrBlocked) {
		t.Fatal("expired node accepted")
	}
}

func TestFailureLatchRequiresNewGeneration(t *testing.T) {
	m, store := managedTestNode(t)
	r, version, _, err := m.Acquire(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	m.Fail(1, version)
	if r.ctx.Err() == nil {
		t.Fatal("failed runtime stayed open")
	}
	if _, _, _, err = m.Acquire(context.Background(), 1); !errors.Is(err, ErrBlocked) {
		t.Fatal("failed node resumed automatically")
	}
	store.mu.Lock()
	store.node.Version++
	store.mu.Unlock()
	if _, _, _, err = m.Acquire(context.Background(), 1); err != nil {
		t.Fatal("new manual generation blocked", err)
	}
}
func TestCanceledRequestDoesNotRevokeSharedRuntime(t *testing.T) {
	m, _ := managedTestNode(t)
	r, _, _, err := m.Acquire(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, _, _ = m.Acquire(ctx, 1)
	if r.ctx.Err() != nil {
		t.Fatal("one canceled caller revoked other accounts' runtime")
	}
}

func TestClosedRuntimeCannotRestartBeforeValidation(t *testing.T) {
	m, store := managedTestNode(t)
	r, _, _, err := m.Acquire(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	r.Close() // Models the independent lease watchdog beating the validation loop.
	if _, _, _, err = m.Acquire(context.Background(), 1); !errors.Is(err, ErrBlocked) {
		t.Fatal("closed runtime resumed automatically")
	}
	if _, _, _, err = m.Acquire(context.Background(), 1); !errors.Is(err, ErrBlocked) {
		t.Fatal("failure was not latched")
	}
	store.mu.Lock()
	store.node.Version++
	store.mu.Unlock()
	if _, _, _, err = m.Acquire(context.Background(), 1); err != nil {
		t.Fatal("manual resume failed", err)
	}
}

type slowCheckpointStore struct {
	*memoryNodeStore
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *slowCheckpointStore) SaveCheckpoint(ctx context.Context, c Checkpoint, gap bool) error {
	s.once.Do(func() {
		close(s.entered)
		select {
		case <-s.release:
		case <-ctx.Done():
		}
	})
	return s.memoryNodeStore.SaveCheckpoint(ctx, c, gap)
}
func TestManagerSamplingContinuesDuringCheckpointWrite(t *testing.T) {
	box, _ := NewSecretBox([]byte(strings.Repeat("k", 32)))
	raw := strings.Replace(exampleConfig(), "vpn.example:51820", "127.0.0.1:51820", 1)
	cipher, err := box.Seal(1, raw)
	if err != nil {
		t.Fatal(err)
	}
	store := &slowCheckpointStore{memoryNodeStore: &memoryNodeStore{node: Node{ID: 1, Version: 1, Active: true, Ciphertext: cipher}, checkpoints: map[string]Checkpoint{}, gaps: map[string]bool{}}, entered: make(chan struct{}), release: make(chan struct{})}
	m := NewManager(store, box)
	defer m.Close()
	defer close(store.release)
	if _, _, _, err = m.Acquire(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	select {
	case <-store.entered:
	case <-time.After(8 * time.Second):
		t.Fatal("checkpoint not reached")
	}
	before := m.Rate(1).SampledAt
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if m.Rate(1).SampledAt.After(before) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("database checkpoint blocked bandwidth sampling")
}
