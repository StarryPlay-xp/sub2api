package traffic

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeStore struct {
	mu        sync.Mutex
	fail      bool
	snapshots []Snapshot
}

func (s *fakeStore) Save(_ context.Context, v Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return errors.New("test storage unavailable")
	}
	s.snapshots = append(s.snapshots, v)
	return nil
}
func (s *fakeStore) Prune(context.Context, time.Time) error { return nil }
func TestTrafficCollectorFailureRetryAndShutdown(t *testing.T) {
	store := &fakeStore{fail: true}
	c := NewCollector(store)
	c.Meter.Add(ClientIn, 123)
	c.Meter.Sample(time.Now().Add(time.Second))
	c.flush(false)
	saved, failed := c.Status()
	require.Nil(t, saved)
	require.True(t, failed)
	store.mu.Lock()
	store.fail = false
	store.mu.Unlock()
	c.flush(false)
	saved, failed = c.Status()
	require.NotNil(t, saved)
	require.False(t, failed)
	c.Close()
	c.Close()
	store.mu.Lock()
	defer store.mu.Unlock()
	require.True(t, store.snapshots[len(store.snapshots)-1].Closed)
	require.Equal(t, uint64(123), store.snapshots[len(store.snapshots)-1].Buckets[0].Bytes.ClientIn)
}
