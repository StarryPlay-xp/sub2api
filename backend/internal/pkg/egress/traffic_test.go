package egress

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/stretchr/testify/require"
)

func TestTrafficEgressActualRouteAndBlockedAttempt(t *testing.T) {
	router, store := testRouter(t, Record{Kind: Direct, AccountID: 7, Revision: 1})
	route, err := router.Resolve(context.Background(), Reference{AccountID: 7})
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte("reply"))
	}))
	defer server.Close()
	transport := &http.Transport{}
	require.NoError(t, route.ConfigureHTTP(transport))
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: route.Wrap(transport)}
	now := time.Now().UTC().Truncate(time.Hour)
	root := traffic.NewMeter("actual-route", now)
	req, err := http.NewRequestWithContext(traffic.WithMeter(context.Background(), root), "POST", server.URL, strings.NewReader("upload"))
	require.NoError(t, err)
	resp, err := traffic.Do(client, req)
	require.NoError(t, err)
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	store.mu.Lock()
	store.failed = true
	store.mu.Unlock()
	req.Body, err = req.GetBody()
	require.NoError(t, err)
	_, err = traffic.Do(client, req)
	require.ErrorIs(t, err, ErrBlocked)
	root.Sample(now.Add(time.Second))
	var found bool
	for _, b := range root.Snapshot(false).Egress {
		if b.Type == "direct" && b.AccountID == 7 {
			found = true
			require.Equal(t, traffic.EgressBytes{Out: 6, In: 5}, b.Bytes)
			require.Equal(t, uint64(1), b.Attempts)
		}
	}
	require.True(t, found)
}
