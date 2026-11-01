package traffic

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Store interface {
	Save(context.Context, Snapshot) error
	Prune(context.Context, time.Time) error
}

// Collector keeps sampling independent of database latency. Only counters and
// aggregate buckets are retained, not request bodies, identities or credentials.
type Collector struct {
	Meter        *Meter
	Store        Store
	stop         chan struct{}
	sampled      chan struct{}
	done         chan struct{}
	once         sync.Once
	mu           sync.Mutex
	savedAt      *time.Time
	persistError bool
}

func NewCollector(store Store) *Collector {
	c := &Collector{Meter: NewMeter(uuid.NewString(), time.Now()), Store: store, stop: make(chan struct{}), sampled: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(c.sampled)
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				c.Meter.Sample(time.Now())
			case <-c.stop:
				c.Meter.SampleFinal(time.Now())
				return
			}
		}
	}()
	go func() {
		defer close(c.done)
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		prune := time.NewTicker(time.Hour)
		defer prune.Stop()
		for {
			select {
			case <-t.C:
				c.flush(false)
			case <-prune.C:
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				_ = store.Prune(ctx, time.Now())
				cancel()
			case <-c.stop:
				<-c.sampled
				c.flush(true)
				return
			}
		}
	}()
	return c
}
func (c *Collector) flush(closed bool) {
	s := c.Meter.Snapshot(closed)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	err := c.Store.Save(ctx, s)
	cancel()
	c.mu.Lock()
	c.persistError = err != nil
	if err == nil {
		t := s.At
		c.savedAt = &t
	}
	c.mu.Unlock()
	if err == nil {
		c.Meter.Ack(s)
	}
}
func (c *Collector) Status() (*time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var at *time.Time
	if c.savedAt != nil {
		v := *c.savedAt
		at = &v
	}
	return at, c.persistError
}
func (c *Collector) Close() {
	if c != nil {
		c.once.Do(func() { close(c.stop) })
		<-c.done
	}
}

type contextKey struct{}

func WithMeter(ctx context.Context, m *Meter) context.Context {
	return context.WithValue(ctx, contextKey{}, m)
}
func FromContext(ctx context.Context) *Meter {
	if ctx == nil {
		return nil
	}
	m, _ := ctx.Value(contextKey{}).(*Meter)
	return m
}
