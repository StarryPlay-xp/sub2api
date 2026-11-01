package traffic

import (
	"context"
	"io"
	"net/http"
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

type transport struct{ next http.RoundTripper }

func Transport(next http.RoundTripper) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}
	if _, ok := next.(*transport); ok {
		return next
	}
	return &transport{next}
}
func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	m := FromContext(req.Context())
	if m == nil || req.Context().Value(transportKey{}) != nil {
		return t.next.RoundTrip(req)
	}
	copy := req.Clone(context.WithValue(req.Context(), transportKey{}, true))
	copy.Body = Body(req.Body, m, UpstreamOut)
	if req.GetBody != nil {
		original := req.GetBody
		copy.GetBody = func() (io.ReadCloser, error) { b, e := original(); return Body(b, m, UpstreamOut), e }
	}
	resp, err := t.next.RoundTrip(copy)
	// A 101 body is a bidirectional socket; WS messages are counted separately.
	if resp != nil && resp.StatusCode != http.StatusSwitchingProtocols {
		resp.Body = Body(resp.Body, m, UpstreamIn)
	}
	return resp, err
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
