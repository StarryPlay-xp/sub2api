package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/stretchr/testify/require"
)

type egressMemoryStore struct{ data TrafficEgressData }

func (s egressMemoryStore) EgressData(context.Context, TrafficEgressQuery) (TrafficEgressData, error) {
	return s.data, nil
}
func TestTrafficEgressReportDetailsComparisonAndPrecision(t *testing.T) {
	id := traffic.EgressIdentity{Type: "direct"}
	row := func(kind string, route, account int64, out, in uint64) TrafficEgressRow {
		return TrafficEgressRow{EgressIdentity: traffic.EgressIdentity{Type: kind, ID: route, AccountID: account}, Bytes: traffic.EgressBytes{Out: out, In: in}, Attempts: 1}
	}
	d := TrafficEgressData{
		Current:  []TrafficEgressRow{row("direct", 0, 0, 10, 90), row("direct", 0, 1, 10, 50), row("direct", 0, -1, 0, 40), row("unidentified", 0, 0, 0, 100), row("proxy", 8, 0, 0, 0)},
		Previous: []TrafficEgressRow{row("direct", 0, 0, 0, 200), row("direct", 0, 1, 0, 100), row("direct", 0, 2, 0, 100), row("wireguard", 7, 0, 0, 20)},
		Routes:   map[traffic.EgressIdentity]TrafficRouteIdentity{id: {RouteType: "direct"}},
		Accounts: map[int64]TrafficEgressAccount{1: {AccountID: 1, AccountName: "deleted-account", AccountDeleted: true}}, History: []TrafficEgressHistory{},
	}
	svc := ServiceTrafficService{Egress: egressMemoryStore{d}}
	report, err := svc.EgressReport(context.Background(), TrafficEgressQuery{Compare: true})
	require.NoError(t, err)
	require.Len(t, report.Routes, 3)
	require.Equal(t, "200", report.Totals.Upstream.TotalBytes)
	require.Equal(t, "100", report.Totals.Unidentified.TotalBytes)
	require.Equal(t, "direct", report.Routes[0].RouteType)
	require.Equal(t, 0.5, *report.Routes[0].Share)
	require.Equal(t, "-100", *report.Routes[0].GrowthBytes)
	detail, err := svc.EgressDetail(context.Background(), TrafficEgressQuery{RouteType: "direct", Compare: true})
	require.NoError(t, err)
	require.Len(t, detail.Accounts, 2)
	require.Equal(t, 1, detail.Summary.AccountCount)
	require.Equal(t, 0.6, *detail.Accounts[0].Share)
	require.True(t, detail.Accounts[0].AccountDeleted)
	require.Equal(t, "40", detail.UnattributedAccount.TotalBytes)
	require.Equal(t, "-100", *detail.Accounts[1].GrowthBytes)
	// Sorting and signed growth must not round through float64.
	large := uint64(1 << 53)
	d.Current = []TrafficEgressRow{row("proxy", 1, 0, large, 0), row("proxy", 2, 0, large+1, 0)}
	d.Previous = nil
	svc.Egress = egressMemoryStore{d}
	report, err = svc.EgressReport(context.Background(), TrafficEgressQuery{})
	require.NoError(t, err)
	require.Equal(t, int64(2), *report.Routes[0].RouteID)
	require.Nil(t, report.Routes[0].Previous)
	_, diff := egressComparison(0, ^uint64(0), true)
	require.Equal(t, "-18446744073709551615", *diff)
}
