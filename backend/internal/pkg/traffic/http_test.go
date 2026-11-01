package traffic

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestTrafficHTTPNestedTransportCountsRetriesOnce(t *testing.T) {
	now := time.Now()
	m := NewMeter("http", now)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = io.WriteString(w, "response")
	}))
	defer server.Close()
	inner := Transport(http.DefaultTransport)
	outer := Transport(roundTripFunc(func(r *http.Request) (*http.Response, error) { return inner.RoundTrip(r) }))
	client := &http.Client{Transport: outer}
	req, err := http.NewRequestWithContext(WithMeter(context.Background(), m), "POST", server.URL, strings.NewReader("upload"))
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		if i > 0 {
			req.Body, err = req.GetBody()
			require.NoError(t, err)
		}
		resp, e := client.Do(req)
		require.NoError(t, e)
		_, e = io.Copy(io.Discard, resp.Body)
		require.NoError(t, e)
		require.NoError(t, resp.Body.Close())
	}
	m.Sample(now.Add(time.Second))
	v := m.Snapshot(false).Buckets[0].Bytes
	require.Equal(t, uint64(12), v.UpstreamOut)
	require.Equal(t, uint64(16), v.UpstreamIn)
}
func TestTrafficUnmarkedBackgroundRequestExcluded(t *testing.T) {
	now := time.Now()
	m := NewMeter("http", now)
	client := &http.Client{Transport: Transport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		_, _ = io.Copy(io.Discard, r.Body)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("reply")), Request: r}, nil
	}))}
	req, _ := http.NewRequest("POST", "http://example.test", strings.NewReader("background"))
	resp, err := client.Do(req)
	require.NoError(t, err)
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	m.Sample(now.Add(time.Second))
	require.Zero(t, m.Current().Rates["total"])
}
func TestTrafficBodyCountsPartialReadsNotContentLength(t *testing.T) {
	now := time.Now()
	m := NewMeter("partial", now)
	r := Body(io.NopCloser(strings.NewReader("123456789")), m, ClientIn)
	buf := make([]byte, 3)
	n, err := r.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 3, n)
	require.NoError(t, r.Close())
	m.Sample(now.Add(time.Second))
	require.Equal(t, 3.0, m.Current().Rates["in"])
}

func TestTrafficStreamingCountsBeforeResponseCompletes(t *testing.T) {
	now := time.Now()
	m := NewMeter("stream", now)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "first")
		_ = http.NewResponseController(w).Flush()
		<-release
		_, _ = io.WriteString(w, "last")
	}))
	defer server.Close()
	defer close(release)
	req, err := http.NewRequestWithContext(WithMeter(context.Background(), m), "GET", server.URL, nil)
	require.NoError(t, err)
	resp, err := Do(server.Client(), req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	buf := make([]byte, 5)
	_, err = io.ReadFull(resp.Body, buf)
	require.NoError(t, err)
	m.Sample(now.Add(time.Second))
	require.Equal(t, 5.0, m.Current().Rates["upstream_in"])
}

func TestTrafficHTTPCompressionUsesExposedBodyBytes(t *testing.T) {
	payload := strings.Repeat("response", 1024)
	var encoded bytes.Buffer
	zip := gzip.NewWriter(&encoded)
	_, err := io.WriteString(zip, payload)
	require.NoError(t, err)
	require.NoError(t, zip.Close())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(encoded.Bytes())
	}))
	defer server.Close()
	for _, explicit := range []bool{false, true} {
		now := time.Now()
		m := NewMeter("compression", now)
		req, err := http.NewRequestWithContext(WithMeter(context.Background(), m), "GET", server.URL, nil)
		require.NoError(t, err)
		expected := len(payload)
		if explicit {
			req.Header.Set("Accept-Encoding", "gzip")
			expected = encoded.Len()
		}
		resp, err := Do(server.Client(), req)
		require.NoError(t, err)
		n, err := io.Copy(io.Discard, resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, int64(expected), n)
		m.Sample(now.Add(time.Second))
		require.Equal(t, float64(expected), m.Current().Rates["upstream_in"])
	}
}
