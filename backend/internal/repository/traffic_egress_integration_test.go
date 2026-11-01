//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestTrafficEgressPersistenceComparisonIdentityAndWGWindow(t *testing.T) {
	db := wireguardTestDB(t)
	ctx := context.Background()
	store := &ServiceTrafficStore{DB: db}
	svc := &service.ServiceTrafficService{Egress: store}
	_, err := db.Exec(`ALTER TABLE accounts ADD COLUMN name TEXT NOT NULL DEFAULT '';INSERT INTO accounts(id,name) VALUES(10,'synthetic account'),(20,'previous account');UPDATE accounts SET deleted_at=NOW() WHERE id=10;UPDATE proxies SET deleted_at=NOW() WHERE id=1;`)
	require.NoError(t, err)
	now := time.Now().UTC().Truncate(time.Hour)
	base := now.Add(-time.Hour)
	old := traffic.NewMeter("previous-egress", base.Add(-time.Hour))
	p := old.EgressScope(traffic.EgressIdentity{Type: "wireguard", ID: 1, AccountID: 20})
	p.Add(traffic.UpstreamIn, 80)
	p.CloseEgress()
	old.Sample(base.Add(-time.Hour + time.Second))
	require.NoError(t, store.Save(ctx, old.Snapshot(true)))
	m := traffic.NewMeter("egress-current", base)
	user := m.Scope()
	user.Bind(1, 11, true)
	a := user.EgressScope(traffic.EgressIdentity{Type: "wireguard", ID: 1, AccountID: 10})
	a.Add(traffic.UpstreamOut, 100)
	a.Add(traffic.UpstreamIn, 200)
	m.Add(traffic.UpstreamIn, 5)
	m.Sample(base.Add(time.Second))
	stale := m.Snapshot(false)
	require.NoError(t, store.Save(ctx, stale))
	require.NoError(t, store.Save(ctx, stale))
	a.Add(traffic.UpstreamIn, 100)
	m.Sample(base.Add(2 * time.Second))
	a.CloseEgress()
	user.CloseScope()
	require.NoError(t, store.Save(ctx, m.Snapshot(true)))
	require.NoError(t, store.Save(ctx, stale))
	restart := traffic.NewMeter("egress-restart", base.Add(3*time.Second))
	b := restart.EgressScope(traffic.EgressIdentity{Type: "wireguard", ID: 1, AccountID: 10})
	b.Add(traffic.UpstreamOut, 20)
	b.CloseEgress()
	restart.Sample(base.Add(4 * time.Second))
	require.NoError(t, store.Save(ctx, restart.Snapshot(true)))
	// Both boundaries have distractors, and minute-only data must not leak in.
	_, err = db.Exec(`INSERT INTO wireguard_traffic_hourly(proxy_id,bucket_start,tx_bytes,rx_bytes) VALUES(1,$1,9000,9000),(1,$2,600,700),(1,$3,8000,8000)`, base.Add(-time.Hour), base, now)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO wireguard_traffic_runtimes(runtime_id,proxy_id,config_version) VALUES('minute-only',1,1)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO wireguard_traffic_buckets(runtime_id,bucket_start,tx_bytes,rx_bytes) VALUES('minute-only',$1,12345,12345)`, base)
	require.NoError(t, err)
	w := wgtunnel.TrafficWindow{Range: "custom", Interval: "hour", Start: base, End: now, AsOf: now, RetainedFrom: now.Add(-90 * 24 * time.Hour)}
	q := service.TrafficEgressQuery{Window: w, Compare: true, Detail: true, RouteType: "wireguard", RouteID: 1}
	detail, err := svc.EgressDetail(ctx, q)
	require.NoError(t, err)
	require.Equal(t, "420", detail.Summary.TotalBytes)
	require.Equal(t, uint64(2), detail.Summary.Attempts)
	require.Equal(t, "80", detail.Summary.Previous.TotalBytes)
	require.Equal(t, "340", *detail.Summary.GrowthBytes)
	require.True(t, detail.Summary.RouteDeleted)
	require.Equal(t, "test", *detail.Summary.RouteName)
	require.Len(t, detail.Accounts, 2)
	require.True(t, detail.Accounts[0].AccountDeleted)
	require.Equal(t, "synthetic account", detail.Accounts[0].AccountName)
	require.Equal(t, "-80", *detail.Accounts[1].GrowthBytes)
	require.Equal(t, 300.0, detail.Summary.Peaks["total"].BytesPerSecond)
	require.Len(t, detail.History, 1)
	require.Equal(t, detail.Summary.Bytes, detail.History[0].Bytes)
	require.Equal(t, 1, detail.Summary.AccountCount)
	require.False(t, detail.Window.ConcurrentRuntimes)
	require.Equal(t, base.Add(-time.Hour+time.Second), *detail.Window.EgressStartedAt)
	wg, err := (&WireGuardStore{DB: db}).TrafficReport(ctx, 1, w)
	require.NoError(t, err)
	require.Equal(t, wg.TX, detail.Summary.WireGuard.TX)
	require.Equal(t, wg.RX, detail.Summary.WireGuard.RX)
	require.Equal(t, wg.Total, detail.Summary.WireGuard.Total)
	require.True(t, detail.Summary.WireGuard.HasRecords)
	report, err := svc.EgressReport(ctx, q)
	require.NoError(t, err)
	require.Equal(t, "425", report.Totals.Upstream.TotalBytes)
	require.Equal(t, "5", report.Totals.Unidentified.TotalBytes)
	host, err := store.Report(ctx, w)
	require.NoError(t, err)
	require.Equal(t, host.Bytes.UpstreamOut, report.Totals.Upstream.Bytes.Out)
	require.Equal(t, host.Bytes.UpstreamIn, report.Totals.Upstream.Bytes.In)
	raw, err := json.Marshal(detail)
	require.NoError(t, err)
	for _, field := range []string{`"host":`, `"port":`, `"username":`, `"password":`, `"credentials":`, `"config_ciphertext":`} {
		require.NotContains(t, string(raw), field)
	}
	// Hard deletion of an unbound account preserves the recorded ID.
	_, err = db.Exec(`DELETE FROM accounts WHERE id=10`)
	require.NoError(t, err)
	detail, err = svc.EgressDetail(ctx, q)
	require.NoError(t, err)
	require.True(t, detail.Accounts[0].AccountDeleted)
	require.Equal(t, int64(10), detail.Accounts[0].AccountID)
	// A second live process adds bytes but cannot contribute a fabricated peak.
	overlapping := traffic.NewMeter("egress-overlap", base)
	c := overlapping.EgressScope(traffic.EgressIdentity{Type: "wireguard", ID: 1, AccountID: 20})
	c.Add(traffic.UpstreamIn, 9)
	c.CloseEgress()
	overlapping.Sample(base.Add(time.Second))
	require.NoError(t, store.Save(ctx, overlapping.Snapshot(true)))
	detail, err = svc.EgressDetail(ctx, q)
	require.NoError(t, err)
	require.True(t, detail.Window.ConcurrentRuntimes)
	require.True(t, detail.Window.Incomplete)
	require.Nil(t, detail.Summary.Peaks)
	require.Nil(t, detail.History[0].Peaks)
	require.Nil(t, detail.Accounts[0].Peaks)
	require.Equal(t, "429", detail.Summary.TotalBytes)
}
func TestTrafficEgressEmptyHistoricalRoutesRetentionAndAtomicSave(t *testing.T) {
	db := wireguardTestDB(t)
	ctx := context.Background()
	store := &ServiceTrafficStore{DB: db}
	svc := &service.ServiceTrafficService{Egress: store}
	_, err := db.Exec(`ALTER TABLE accounts ADD COLUMN name TEXT NOT NULL DEFAULT ''`)
	require.NoError(t, err)
	now := time.Now().UTC().Truncate(time.Hour)
	q := service.TrafficEgressQuery{Window: wgtunnel.TrafficWindow{Interval: "hour", Start: now.Add(-time.Hour), End: now, AsOf: now, RetainedFrom: now.Add(-90 * 24 * time.Hour)}, Detail: true, RouteType: "direct"}
	direct, err := svc.EgressDetail(ctx, q)
	require.NoError(t, err)
	require.Equal(t, "0", direct.Summary.TotalBytes)
	require.Nil(t, direct.Summary.Peaks)
	require.Nil(t, direct.Summary.Share)
	require.False(t, direct.Window.HasRecords)
	require.Nil(t, direct.Window.EgressStartedAt)
	q.RouteType = "proxy"
	q.RouteID = 999
	_, err = svc.EgressDetail(ctx, q)
	require.ErrorIs(t, err, service.ErrTrafficRouteNotFound)
	q.RouteID = 1
	_, err = svc.EgressDetail(ctx, q)
	require.ErrorIs(t, err, service.ErrTrafficRouteNotFound) // a WG node is not a proxy route
	q.RouteType = "wireguard"
	empty, err := svc.EgressDetail(ctx, q)
	require.NoError(t, err)
	require.Equal(t, "0", empty.Summary.WireGuard.Total)
	require.False(t, empty.Summary.WireGuard.HasRecords)
	for _, tc := range []struct {
		id    string
		age   time.Duration
		route int64
	}{{"old-egress", 91 * 24 * time.Hour, 998}, {"retained-egress", 2 * time.Hour, 999}} {
		at := now.Add(-tc.age)
		m := traffic.NewMeter(tc.id, at)
		a := m.EgressScope(traffic.EgressIdentity{Type: "proxy", ID: tc.route, AccountID: 10})
		a.Add(traffic.UpstreamOut, 7)
		a.CloseEgress()
		m.Sample(at.Add(time.Second))
		require.NoError(t, store.Save(ctx, m.Snapshot(true)))
	}
	require.NoError(t, store.Prune(ctx, now))
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM service_traffic_egress_buckets WHERE route_id=998`).Scan(&count))
	require.Zero(t, count)
	q.RouteType = "proxy"
	q.RouteID = 999
	historical, err := svc.EgressDetail(ctx, q)
	require.NoError(t, err)
	require.True(t, historical.Summary.RouteDeleted)
	require.Equal(t, "0", historical.Summary.TotalBytes)
	q.RouteID = 998
	_, err = svc.EgressDetail(ctx, q)
	require.ErrorIs(t, err, service.ErrTrafficRouteNotFound)
	// A failure writing the new table must roll back host/user data and sequence.
	m := traffic.NewMeter("atomic-egress", now)
	m.Add(traffic.UpstreamOut, 1)
	m.Sample(now.Add(time.Second))
	snapshot := m.Snapshot(false)
	snapshot.Egress[0].Type = "invalid"
	require.Error(t, store.Save(ctx, snapshot))
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM service_traffic_runtimes WHERE runtime_id='atomic-egress'`).Scan(&count))
	require.Zero(t, count)
	require.NoError(t, store.Save(ctx, m.Snapshot(true)))
}
