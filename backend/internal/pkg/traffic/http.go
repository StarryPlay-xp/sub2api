package traffic

import (
	"context"
	"io"
	"net/http"
	"sync"
)

type countingBody struct {
	io.ReadCloser
	meter     *Meter
	direction int
}

func (b *countingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.meter.Add(b.direction, n)
	return n, err
}
func Body(body io.ReadCloser, m *Meter, d int) io.ReadCloser {
	if body == nil || body == http.NoBody || m == nil {
		return body
	}
	if b, ok := body.(*countingBody); ok && b.meter == m && b.direction == d {
		return body
	}
	return &countingBody{body, m, d}
}

type transportKey struct{}
type transportObservation struct {
	base, handle  *Meter
	scope         *Meter
	routed        bool
	once          sync.Once
	mu            sync.Mutex
	requestBodies int
	responseDone  bool
}

func (o *transportObservation) route(identity *EgressIdentity) {
	if identity == nil || o.routed {
		return
	}
	o.routed = true
	// WS owners already count the lease/dial attempt and own its lifetime.
	if o.base.HasEgress(*identity) {
		return
	}
	o.scope = o.base.EgressScope(*identity)
	o.handle.SetSource(o.scope)
}
func (o *transportObservation) finishRequest() {
	o.mu.Lock()
	o.requestBodies--
	done := o.responseDone && o.requestBodies == 0
	o.mu.Unlock()
	if done {
		o.once.Do(func() { o.scope.CloseEgress() })
	}
}
func (o *transportObservation) close() {
	o.mu.Lock()
	o.responseDone = true
	done := o.requestBodies == 0
	o.mu.Unlock()
	if done {
		o.once.Do(func() { o.scope.CloseEgress() })
	}
}
func (o *transportObservation) requestBody(body io.ReadCloser) io.ReadCloser {
	if body == nil || body == http.NoBody {
		return body
	}
	o.mu.Lock()
	o.requestBodies++
	o.mu.Unlock()
	return &observedRequestBody{ReadCloser: Body(body, o.handle, UpstreamOut), observation: o}
}

type observedRequestBody struct {
	io.ReadCloser
	observation *transportObservation
	once        sync.Once
}

func (b *observedRequestBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.observation.finishRequest)
	return err
}

type transport struct {
	next     http.RoundTripper
	identity *EgressIdentity
}

func Transport(next http.RoundTripper) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}
	if _, ok := next.(*transport); ok {
		return next
	}
	return &transport{next: next}
}

// EgressTransport upgrades a nested generic observation rather than wrapping
// bodies twice. Both the request and response follow this attempt's actual route.
func EgressTransport(next http.RoundTripper, identity EgressIdentity) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}
	return &transport{next: next, identity: &identity}
}
func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	m := FromContext(req.Context())
	if m == nil {
		return t.next.RoundTrip(req)
	}
	if observation, ok := req.Context().Value(transportKey{}).(*transportObservation); ok {
		observation.route(t.identity)
		return t.next.RoundTrip(req)
	}
	observation := &transportObservation{base: m, handle: m.Switch()}
	observation.route(t.identity)
	copy := req.Clone(context.WithValue(req.Context(), transportKey{}, observation))
	copy.Body = observation.requestBody(req.Body)
	if req.GetBody != nil {
		original := req.GetBody
		copy.GetBody = func() (io.ReadCloser, error) {
			body, err := original()
			return observation.requestBody(body), err
		}
	}
	resp, err := t.next.RoundTrip(copy)
	if resp != nil && resp.Body != nil && resp.StatusCode != http.StatusSwitchingProtocols {
		resp.Body = &observedResponseBody{ReadCloser: Body(resp.Body, observation.handle, UpstreamIn), observation: observation}
	} else {
		observation.close()
	}
	if err != nil {
		observation.close()
	}
	return resp, err
}

type observedResponseBody struct {
	io.ReadCloser
	observation *transportObservation
}

func (b *observedResponseBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		b.observation.close()
	}
	return n, err
}
func (b *observedResponseBody) Close() error {
	err := b.ReadCloser.Close()
	b.observation.close()
	return err
}

// Do copies the client so shared pools and unrelated background calls are unchanged.
func Do(client *http.Client, req *http.Request) (*http.Response, error) {
	if client == nil {
		client = http.DefaultClient
	}
	copy := *client
	copy.Transport = Transport(client.Transport)
	return copy.Do(req) //nolint:gosec // G704: a transport decorator; callers retain their existing target validation, dial and redirect policies.
}
