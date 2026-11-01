//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWSConnPoolTrafficLeaseAttribution(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	pool := newOpenAIWSConnPool(cfg)
	defer pool.Close()
	pool.setClientDialerForTest(&openAIWSCountingDialer{})
	now := time.Now().UTC().Truncate(time.Hour)
	root := traffic.NewMeter("pool", now)
	a, b := root.Scope(), root.Scope()
	a.Bind(1, 11, true)
	b.Bind(2, 22, true)
	defer a.CloseScope()
	defer b.CloseScope()
	request := openAIWSAcquireRequest{Account: &Account{ID: 123, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, WSURL: "wss://example.invalid/v1/responses"}
	first, err := pool.Acquire(traffic.WithMeter(context.Background(), a), request)
	require.NoError(t, err)
	transport := first.conn.trafficMeter
	require.NotNil(t, transport)
	transport.Add(traffic.UpstreamOut, 100)
	id := first.ConnID()
	first.Release()
	transport.Add(traffic.UpstreamIn, 7)
	second, err := pool.Acquire(traffic.WithMeter(context.Background(), b), request)
	require.NoError(t, err)
	require.Equal(t, id, second.ConnID())
	transport.Add(traffic.UpstreamIn, 200)
	second.Release()
	transport.Add(traffic.UpstreamIn, 9)
	root.Sample(now.Add(time.Second))
	counts := map[int64]uint64{}
	var un uint64
	for _, bucket := range root.Snapshot(false).Subjects {
		if bucket.Type == "api_key" {
			counts[bucket.ID] = bucket.Bytes.Vector()[6]
		}
		if bucket.Type == "unattributed" {
			un += bucket.Bytes.Vector()[6]
		}
	}
	require.Equal(t, map[int64]uint64{11: 100, 22: 200}, counts)
	require.Equal(t, uint64(16), un)
}

type trafficBatchProvider struct{ fakeProcessorProvider }

func (p *trafficBatchProvider) Get(ctx context.Context, _ *BatchImageJob, _ *Account) (*BatchProviderStatus, error) {
	traffic.FromContext(ctx).Add(traffic.UpstreamOut, 123)
	return &BatchProviderStatus{InternalState: BatchProviderStateQueued}, nil
}
func TestBatchImageProviderTrafficAttribution(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	m := traffic.NewMeter("batch", now)
	repo := newFakeBatchImageRepository()
	key, account := int64(11), int64(10)
	name := "synthetic/job"
	repo.jobs["job"] = &BatchImageJob{BatchID: "job", UserID: 1, APIKeyID: &key, Status: BatchImageJobStatusSubmitted, Provider: "fake", AccountID: &account, ProviderJobName: &name}
	processor := &BatchImageProviderProcessor{Repo: repo, ProviderRegistry: NewBatchImageProviderRegistry(&trafficBatchProvider{}), AccountResolver: &fakeBatchImageAccountResolver{account: &Account{}}}
	_, err := processor.Process(traffic.WithMeter(context.Background(), m), "job")
	require.NoError(t, err)
	m.Sample(now.Add(time.Second))
	var keyBytes uint64
	for _, b := range m.Snapshot(false).Subjects {
		require.Zero(t, b.Requests)
		if b.Type == "api_key" {
			require.Equal(t, int64(11), b.ID)
			require.Equal(t, int64(1), b.UserID)
			keyBytes += b.Bytes.UpstreamOut
		}
	}
	require.Equal(t, uint64(123), keyBytes)
}
