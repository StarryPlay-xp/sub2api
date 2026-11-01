package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/stretchr/testify/require"
)

type trafficSubjectsFake struct {
	data          TrafficSubjectData
	identityReads int
}

func (f *trafficSubjectsFake) SubjectData(context.Context, TrafficSubjectQuery) (TrafficSubjectData, error) {
	return f.data, nil
}
func (f *trafficSubjectsFake) SubjectIdentities(_ context.Context, subject string, ids []int64) (map[int64]TrafficIdentity, error) {
	f.identityReads++
	result := map[int64]TrafficIdentity{}
	for _, id := range ids {
		result[id] = TrafficIdentity{SubjectType: subject, UserID: id, Username: "synthetic"}
	}
	return result, nil
}
func TestTrafficRankingNullsExactSortAndPaging(t *testing.T) {
	one, two, three := int64(1), int64(2), int64(3)
	store := &trafficSubjectsFake{data: TrafficSubjectData{Current: []TrafficSubjectStats{
		{TrafficIdentity: TrafficIdentity{SubjectType: "api_key", UserID: 1, APIKeyID: &one}, Bytes: traffic.Bytes{ClientIn: 9007199254740993}, Requests: 1},
		{TrafficIdentity: TrafficIdentity{SubjectType: "api_key", UserID: 1, APIKeyID: &two}, Bytes: traffic.Bytes{ClientIn: 9007199254740992}, Requests: 1},
		{TrafficIdentity: TrafficIdentity{SubjectType: "api_key", UserID: 1, APIKeyID: &three}, Bytes: traffic.Bytes{ClientIn: 1}},
	}}}
	svc := &ServiceTrafficService{Subjects: store}
	q := TrafficSubjectQuery{Subject: "api_key", Sort: "bytes_per_request", Order: "asc", Page: 1, PageSize: 20}
	r, err := svc.Ranking(context.Background(), q)
	require.NoError(t, err)
	require.Equal(t, two, r.Items[0].ID())
	require.Equal(t, three, r.Items[2].ID())
	require.Nil(t, r.Items[2].BytesPerRequest)
	q.Order = "desc"
	r, err = svc.Ranking(context.Background(), q)
	require.NoError(t, err)
	require.Equal(t, one, r.Items[0].ID())
	require.Equal(t, three, r.Items[2].ID())
	q.Page = int(^uint(0) >> 1)
	r, err = svc.Ranking(context.Background(), q)
	require.NoError(t, err)
	require.Empty(t, r.Items)
}
func TestTrafficLiveIdentityCache(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	m := traffic.NewMeter("live", now)
	a := m.Scope()
	a.Bind(1, 11, true)
	a.Add(traffic.ClientIn, 123)
	m.Sample(now.Add(time.Second))
	defer a.CloseScope()
	store := &trafficSubjectsFake{}
	svc := &ServiceTrafficService{Collector: &traffic.Collector{Meter: m}, Subjects: store}
	for i := 0; i < 3; i++ {
		r, err := svc.LiveSubjects(context.Background(), "user", "avg_60s", 10)
		require.NoError(t, err)
		require.Len(t, r.Items, 1)
		require.Equal(t, 123.0, r.Items[0].Average.Total)
		require.Equal(t, int64(1), r.Items[0].InFlight)
		require.Equal(t, "synthetic", r.Items[0].Username)
	}
	require.Equal(t, 1, store.identityReads)
	svc.identityCache.entries[trafficIdentityCacheKey{"user", 1}] = trafficIdentityCacheEntry{expires: time.Now().Add(-time.Second)}
	_, err := svc.LiveSubjects(context.Background(), "user", "rate", 1)
	require.NoError(t, err)
	require.Equal(t, 2, store.identityReads)
}

func TestTrafficRankingSearchTotalPreservesConcentrationAndTotals(t *testing.T) {
	store := &trafficSubjectsFake{data: TrafficSubjectData{Unattributed: traffic.Bytes{ClientIn: 100}}}
	for id := int64(1); id <= 45; id++ {
		name := "ordinary key"
		if id >= 44 {
			name = "Needle key"
		}
		store.data.Current = append(store.data.Current, TrafficSubjectStats{
			TrafficIdentity: TrafficIdentity{SubjectType: "api_key", UserID: 1, APIKeyID: &id, APIKeyName: &name},
			Bytes:           traffic.Bytes{ClientIn: uint64(id) * 10}, Requests: 1,
		})
	}
	svc := &ServiceTrafficService{Subjects: store}
	q := TrafficSubjectQuery{Subject: "api_key", Sort: "total_bytes", Order: "desc", Page: 1, PageSize: 200}
	baseline, err := svc.Ranking(context.Background(), q)
	require.NoError(t, err)
	require.Equal(t, 45, baseline.Total)
	q.Q = "nEeDlE"
	q.PageSize = 1
	for _, tc := range []struct {
		page int
		id   int64
	}{{1, 45}, {2, 44}, {3, 0}} {
		q.Page = tc.page
		result, err := svc.Ranking(context.Background(), q)
		require.NoError(t, err)
		require.Equal(t, 2, result.Total)
		require.Equal(t, baseline.Concentration, result.Concentration)
		require.Equal(t, 45, result.Concentration.Subjects)
		require.Equal(t, baseline.Totals, result.Totals)
		if tc.id == 0 {
			require.Empty(t, result.Items)
			continue
		}
		require.Len(t, result.Items, 1)
		require.Equal(t, tc.id, result.Items[0].ID())
		for _, item := range baseline.Items {
			if item.ID() == tc.id {
				require.Equal(t, item.Share, result.Items[0].Share)
			}
		}
	}
	q.Q = "no matching key"
	q.Page = 1
	empty, err := svc.Ranking(context.Background(), q)
	require.NoError(t, err)
	require.Zero(t, empty.Total)
	require.Empty(t, empty.Items)
	require.Equal(t, baseline.Concentration, empty.Concentration)
	require.Equal(t, baseline.Totals, empty.Totals)
}
