//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestTrafficSubjectPersistenceQueriesAndRetention(t *testing.T) {
	db := wireguardTestDB(t)
	ctx := context.Background()
	store := &ServiceTrafficStore{DB: db}
	svc := &service.ServiceTrafficService{Subjects: store}
	_, err := db.Exec(`CREATE TABLE users(id BIGINT PRIMARY KEY,email TEXT,username TEXT,deleted_at TIMESTAMPTZ);CREATE TABLE groups(id BIGINT PRIMARY KEY,name TEXT);CREATE TABLE api_keys(id BIGINT PRIMARY KEY,user_id BIGINT,name TEXT,group_id BIGINT,deleted_at TIMESTAMPTZ);
 INSERT INTO users VALUES(1,'one@example.invalid','Alpha',NULL),(2,'two@example.invalid','Beta',NOW()),(3,'three@example.invalid','Gamma',NULL),(4,'four@example.invalid','NoTraffic',NULL);
 INSERT INTO groups VALUES(1,'synthetic group');INSERT INTO api_keys VALUES(11,1,'First',1,NULL),(12,1,'Second',1,NOW()),(21,2,'Third',1,NOW()),(31,3,'OldOnly',NULL,NULL);`)
	require.NoError(t, err)
	now := time.Now().UTC().Truncate(time.Hour)
	base := now.Add(-2 * time.Hour)
	previous := traffic.NewMeter("previous", base)
	p := previous.Scope()
	p.Bind(3, 31, true)
	p.Add(traffic.ClientIn, 500)
	previous.Sample(base.Add(time.Second))
	p.CloseScope()
	require.NoError(t, store.Save(ctx, previous.Snapshot(true)))
	current := traffic.NewMeter("current", base.Add(time.Hour))
	a, b, c := current.Scope(), current.Scope(), current.Scope()
	a.Bind(1, 11, true)
	b.Bind(1, 12, true)
	c.Bind(2, 21, true)
	a.Add(traffic.ClientIn, 100)
	b.Add(traffic.ClientOut, 200)
	c.Add(traffic.UpstreamIn, 300)
	current.Add(traffic.UpstreamOut, 100)
	current.Sample(base.Add(time.Hour + time.Second))
	snapshot := current.Snapshot(false)
	require.NoError(t, store.Save(ctx, snapshot))
	require.NoError(t, store.Save(ctx, snapshot))
	a.Add(traffic.ClientIn, 100)
	current.Sample(base.Add(time.Hour + 2*time.Second))
	a.CloseScope()
	b.CloseScope()
	c.CloseScope()
	require.NoError(t, store.Save(ctx, current.Snapshot(true)))
	require.NoError(t, store.Save(ctx, snapshot))
	q := service.TrafficSubjectQuery{Subject: "user", Window: wgtunnel.TrafficWindow{Range: "custom", Interval: "hour", Start: base.Add(time.Hour), End: now, AsOf: now, RetainedFrom: now.Add(-90 * 24 * time.Hour)}, Sort: "total_bytes", Order: "desc", Page: 1, PageSize: 20, Compare: true}
	rank, err := svc.Ranking(ctx, q)
	require.NoError(t, err)
	require.Equal(t, "800", rank.Totals.AllTotalBytes)
	require.Equal(t, "700", rank.Totals.Attributed.TotalBytes)
	require.Equal(t, "100", rank.Totals.Unattributed.TotalBytes)
	require.Len(t, rank.Items, 3)
	require.Equal(t, 3, rank.Total)
	require.Equal(t, int64(1), rank.Items[0].UserID)
	require.Equal(t, "400", rank.Items[0].TotalBytes)
	require.Equal(t, uint64(2), *rank.Items[0].ActiveAPIKeys)
	require.Equal(t, 0.5, *rank.Items[0].Share)
	require.Equal(t, 300.0, rank.Items[0].Peaks["total"].BytesPerSecond)
	require.Equal(t, "-500", *rank.Items[2].GrowthBytes)
	require.Equal(t, "0", rank.Items[2].TotalBytes)
	require.False(t, rank.PreviousWindow.Complete)
	require.False(t, rank.Window.ConcurrentRuntimes)
	q.Sort = "growth"
	q.Order = "asc"
	q.PageSize = 1
	rank, err = svc.Ranking(ctx, q)
	require.NoError(t, err)
	require.Equal(t, int64(3), rank.Items[0].UserID)
	q.Subject = "api_key"
	q.Sort = "total_bytes"
	q.Order = "desc"
	q.UserID = 1
	q.Q = "sEcOnD"
	q.PageSize = 20
	rank, err = svc.Ranking(ctx, q)
	require.NoError(t, err)
	require.Equal(t, 2, rank.Total)
	require.Len(t, rank.Items, 1)
	require.Equal(t, int64(12), *rank.Items[0].APIKeyID)
	require.True(t, *rank.Items[0].APIKeyDeleted)
	require.Equal(t, "800", rank.Totals.AllTotalBytes)
	require.Equal(t, 0.25, *rank.Concentration.Top1)
	require.Equal(t, 0.5, *rank.Concentration.Top5)
	q.Q = ""
	q.PageSize = 1
	rank, err = svc.Ranking(ctx, q)
	require.NoError(t, err)
	require.Equal(t, int64(11), *rank.Items[0].APIKeyID)
	q.Page = 2
	rank, err = svc.Ranking(ctx, q)
	require.NoError(t, err)
	require.Equal(t, int64(12), *rank.Items[0].APIKeyID)
	q.ID = 12
	detail, err := svc.SubjectDetail(ctx, q)
	require.NoError(t, err)
	require.Len(t, detail.History, 1)
	require.Equal(t, "200", detail.Summary.TotalBytes)
	require.True(t, *detail.Summary.APIKeyDeleted)
	q.Subject = "user"
	q.UserID = 0
	q.ID = 4
	detail, err = svc.SubjectDetail(ctx, q)
	require.NoError(t, err)
	require.Equal(t, "0", detail.Summary.TotalBytes)
	require.Nil(t, detail.Summary.Peaks)
	require.Empty(t, detail.History)
	q.ID = 999
	_, err = svc.SubjectDetail(ctx, q)
	require.ErrorIs(t, err, service.ErrTrafficSubjectNotFound)
	// A restart is additive without invalidating peaks when runtimes do not overlap.
	restart := traffic.NewMeter("restart", base.Add(time.Hour+3*time.Second))
	r := restart.Scope()
	r.Bind(1, 11, false)
	r.Add(traffic.ClientIn, 50)
	restart.Sample(base.Add(time.Hour + 4*time.Second))
	r.CloseScope()
	require.NoError(t, store.Save(ctx, restart.Snapshot(true)))
	q.ID = 1
	detail, err = svc.SubjectDetail(ctx, q)
	require.NoError(t, err)
	require.Equal(t, "450", detail.Summary.TotalBytes)
	require.False(t, detail.Window.ConcurrentRuntimes)
	overlap := traffic.NewMeter("overlap", base.Add(time.Hour))
	o := overlap.Scope()
	o.Bind(1, 11, false)
	o.Add(traffic.ClientIn, 10)
	overlap.Sample(base.Add(time.Hour + time.Second))
	o.CloseScope()
	require.NoError(t, store.Save(ctx, overlap.Snapshot(true)))
	detail, err = svc.SubjectDetail(ctx, q)
	require.NoError(t, err)
	require.True(t, detail.Window.ConcurrentRuntimes)
	require.Nil(t, detail.Summary.Peaks)
	require.Nil(t, detail.History[0].Peaks)
	require.Equal(t, "460", detail.Summary.TotalBytes)
	// Retained data cannot disappear when identities are soft deleted.
	q.ID = 2
	detail, err = svc.SubjectDetail(ctx, q)
	require.NoError(t, err)
	require.True(t, detail.Summary.UserDeleted)
	require.Equal(t, "Beta", detail.Summary.Username)
	q.ID = 0
	q.Page = 1
	q.PageSize = 20
	q.Window.RetainedFrom = base.Add(time.Hour)
	rank, err = svc.Ranking(ctx, q)
	require.NoError(t, err)
	require.False(t, rank.PreviousWindow.Complete)
	for _, item := range rank.Items {
		require.NotEqual(t, int64(3), item.UserID)
	}
	require.NoError(t, store.Prune(ctx, now.Add(91*24*time.Hour)))
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM service_traffic_subject_buckets`).Scan(&count))
	require.Zero(t, count)
	var started time.Time
	require.NoError(t, db.QueryRow(`SELECT started_at FROM service_traffic_attribution_metadata`).Scan(&started))
	require.True(t, started.Equal(base.Add(time.Second)))
}

func TestTrafficSubjectSaveAtomicAndPreviousRetention(t *testing.T) {
	db := wireguardTestDB(t)
	s := &ServiceTrafficStore{DB: db}
	now := time.Now().UTC().Truncate(time.Hour)
	m := traffic.NewMeter("atomic", now)
	a := m.Scope()
	a.Bind(1, 11, true)
	a.Add(traffic.ClientIn, 12)
	m.Sample(now.Add(time.Second))
	snapshot := m.Snapshot(false)
	for i := range snapshot.Subjects {
		snapshot.Subjects[i].Type = "invalid"
	}
	require.Error(t, s.Save(context.Background(), snapshot))
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM service_traffic_runtimes`).Scan(&n))
	require.Zero(t, n)
	require.NoError(t, s.Save(context.Background(), m.Snapshot(true)))
	a.CloseScope()
	// Failed batches roll back the runtime sequence too, so retries remain possible.
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM service_traffic_subject_buckets`).Scan(&n))
	require.Equal(t, 3, n)
}
