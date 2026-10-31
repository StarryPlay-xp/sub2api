package egress

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

type Attempt struct {
	CorrelationIDs []string  `json:"-"`
	ID             string    `json:"attempt_id"`
	RequestID      string    `json:"request_id,omitempty"`
	AccountID      int64     `json:"account_id,omitempty"`
	RouteKey       string    `json:"route_key"`
	StartedAt      time.Time `json:"started_at"`
	FinishedAt     time.Time `json:"finished_at"`
	Snapshot       Snapshot  `json:"snapshot"`
	Outcome        string    `json:"outcome"`
}
type AttemptStore interface {
	SaveAttempt(context.Context, Attempt) error
}
type Recorder struct {
	store   AttemptStore
	ctx     context.Context
	cancel  context.CancelFunc
	queue   chan Attempt
	done    chan struct{}
	mu      sync.Mutex
	closed  bool
	dropped atomic.Uint64
}

func NewRecorder(store AttemptStore) *Recorder {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Recorder{ctx: ctx, cancel: cancel, store: store, queue: make(chan Attempt, 1024), done: make(chan struct{})}
	go func() {
		defer close(r.done)
		for a := range r.queue {
			ctx, cancel := context.WithTimeout(r.ctx, time.Second)
			err := store.SaveAttempt(ctx, a)
			cancel()
			if err != nil {
				r.dropped.Add(1)
			}
		}
	}()
	return r
}
func (r *Recorder) Record(a Attempt) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	select {
	case r.queue <- a:
	default:
		r.dropped.Add(1)
	}
}
func (r *Recorder) Dropped() uint64 { return r.dropped.Load() }
func (r *Recorder) Close() {
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		close(r.queue)
	}
	r.mu.Unlock()
	select {
	case <-r.done:
	case <-time.After(3 * time.Second):
		r.cancel()
		<-r.done
	}
	r.cancel()
}
func (r *Route) observeRoundTrip(request *http.Request, next http.RoundTripper) (*http.Response, error) {
	observer, recorder := r.router.observer, r.router.recorder
	if observer == nil || recorder == nil {
		return next.RoundTrip(request)
	}
	observer.Watch(r)
	id := [16]byte{}
	if _, err := rand.Read(id[:]); err != nil {
		return nil, err
	}
	requestID, _ := request.Context().Value(ctxkey.ClientRequestID).(string)
	a := Attempt{ID: hex.EncodeToString(id[:]), RequestID: requestID, AccountID: r.AccountID(), RouteKey: r.EvidenceKey(), StartedAt: time.Now(), Snapshot: observer.Snapshot(r, nil, false)}
	a.CorrelationIDs = []string{requestID, "client:" + requestID}
	if localID, ok := request.Context().Value(ctxkey.RequestID).(string); ok && localID != "" {
		a.CorrelationIDs = append(a.CorrelationIDs, localID, "local:"+localID)
	}
	var mu sync.Mutex
	connected := false
	attemptNumber := 0
	trace := &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) {
		mu.Lock()
		if connected {
			a.FinishedAt = time.Now()
			a.Outcome = "connection_retry"
			recorder.Record(a)
			attemptNumber++
			a.ID = hex.EncodeToString(id[:]) + ":" + strconv.Itoa(attemptNumber)
			a.StartedAt = time.Now()
			a.FinishedAt = time.Time{}
			a.Outcome = ""
		}
		connected = true
		a.Snapshot = observer.Snapshot(r, info.Conn.RemoteAddr(), info.Reused)
		mu.Unlock()
	}}
	request = request.WithContext(httptrace.WithClientTrace(request.Context(), trace))
	var once sync.Once
	finish := func(outcome string) {
		once.Do(func() {
			mu.Lock()
			a.FinishedAt = time.Now()
			a.Outcome = outcome
			copy := a
			mu.Unlock()
			recorder.Record(copy)
		})
	}
	response, err := next.RoundTrip(request)
	if err != nil {
		finish("transport_error")
		return response, err
	}
	if response.Body == nil || response.StatusCode == http.StatusSwitchingProtocols {
		finish("response")
		return response, nil
	}
	response.Body = &attemptBody{ReadCloser: response.Body, finish: finish}
	return response, nil
}

type attemptBody struct {
	io.ReadCloser
	finish func(string)
}

func (b *attemptBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err == io.EOF {
		b.finish("complete")
	} else if err != nil {
		b.finish("stream_error")
	}
	return n, err
}
func (b *attemptBody) Close() error { err := b.ReadCloser.Close(); b.finish("closed"); return err }

// SetTelemetry is called during application construction, before the router is shared.
func (r *Router) SetTelemetry(observer *Observer, recorder *Recorder) {
	r.observer = observer
	r.recorder = recorder
}

// RecordConnectionAttempt captures the family of an actually acquired socket,
// including WS reuse where no new HTTP handshake occurs for this request.
func (r *Route) RecordConnectionAttempt(ctx context.Context, remote net.Addr, reused bool, outcome string) {
	if r.router.observer == nil || r.router.recorder == nil {
		return
	}
	id := [16]byte{}
	if _, err := rand.Read(id[:]); err != nil {
		return
	}
	requestID, _ := ctx.Value(ctxkey.ClientRequestID).(string)
	aliases := []string{requestID, "client:" + requestID}
	if localID, ok := ctx.Value(ctxkey.RequestID).(string); ok && localID != "" {
		aliases = append(aliases, localID, "local:"+localID)
	}
	r.router.observer.Watch(r)
	r.router.recorder.Record(Attempt{ID: hex.EncodeToString(id[:]), RequestID: requestID, CorrelationIDs: aliases, AccountID: r.AccountID(), RouteKey: r.EvidenceKey(), StartedAt: time.Now(), FinishedAt: time.Now(), Snapshot: r.router.observer.Snapshot(r, remote, reused), Outcome: outcome})
}
