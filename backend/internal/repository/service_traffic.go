package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type ServiceTrafficStore struct{ DB *sql.DB }

func ProvideServiceTraffic(db *sql.DB, gateway *service.OpenAIGatewayService) *service.ServiceTrafficService {
	store := &ServiceTrafficStore{DB: db}
	collector := traffic.NewCollector(store)
	gateway.SetTrafficMeter(collector.Meter)
	return &service.ServiceTrafficService{Collector: collector, Store: store, Limits: &BandwidthLimitStore{DB: db}}
}
func (s *ServiceTrafficStore) Save(ctx context.Context, c traffic.Snapshot) error {
	if c.RuntimeID == "" || c.StartedAt.IsZero() || c.At.Before(c.StartedAt) {
		return errors.New("invalid traffic snapshot")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `INSERT INTO service_traffic_runtimes(runtime_id,started_at,saved_at,sequence,closed,incomplete) VALUES($1,$2,$3,$4,$5,$6)
 ON CONFLICT(runtime_id) DO UPDATE SET saved_at=EXCLUDED.saved_at,sequence=EXCLUDED.sequence,closed=EXCLUDED.closed,incomplete=service_traffic_runtimes.incomplete OR EXCLUDED.incomplete
 WHERE NOT service_traffic_runtimes.closed AND (service_traffic_runtimes.sequence<EXCLUDED.sequence OR (service_traffic_runtimes.sequence=EXCLUDED.sequence AND EXCLUDED.closed))`, c.RuntimeID, c.StartedAt, c.At, c.Sequence, c.Closed, c.Incomplete)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return tx.Commit()
	}
	for _, b := range c.Buckets {
		raw, e := json.Marshal(b)
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO service_traffic_buckets(runtime_id,interval,bucket_start,sequence,data) VALUES($1,$2,$3,$4,$5)
 ON CONFLICT(runtime_id,interval,bucket_start) DO UPDATE SET sequence=EXCLUDED.sequence,data=EXCLUDED.data WHERE service_traffic_buckets.sequence<EXCLUDED.sequence`, c.RuntimeID, b.Interval, b.Start, b.Sequence, string(raw)); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func (s *ServiceTrafficStore) Prune(ctx context.Context, now time.Time) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM service_traffic_buckets WHERE (interval='minute' AND bucket_start<$1) OR (interval='hour' AND bucket_start<$2)`, now.Add(-7*24*time.Hour).UTC().Truncate(time.Minute), now.Add(-90*24*time.Hour).UTC().Truncate(time.Hour))
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `DELETE FROM service_traffic_runtimes WHERE saved_at<$1 AND NOT EXISTS(SELECT 1 FROM service_traffic_buckets b WHERE b.runtime_id=service_traffic_runtimes.runtime_id)`, now.Add(-90*24*time.Hour))
	return err
}
func (s *ServiceTrafficStore) Report(ctx context.Context, w wgtunnel.TrafficWindow) (service.ServiceTrafficReport, error) {
	result := service.ServiceTrafficReport{TrafficWindow: w, MetricBasis: traffic.MetricBasis, Scope: "single_instance_history", PeakWindowSeconds: 1, History: []traffic.Bucket{}, InBytes: "0", OutBytes: "0", TotalBytes: "0"}
	rows, err := s.DB.QueryContext(ctx, `SELECT b.data,r.incomplete,r.closed,r.started_at,r.saved_at,r.runtime_id FROM service_traffic_buckets b JOIN service_traffic_runtimes r USING(runtime_id) WHERE b.interval=$1 AND b.bucket_start>=$2 AND b.bucket_start<$3 ORDER BY b.bucket_start,r.started_at`, w.Interval, w.Start, w.End)
	if err != nil {
		return result, err
	}
	defer func() { _ = rows.Close() }()
	type runtimeSpan struct{ start, end time.Time }
	spans := map[string]runtimeSpan{}
	buckets := map[time.Time]*traffic.Bucket{}
	for rows.Next() {
		var raw []byte
		var incomplete, closed bool
		var start, saved time.Time
		var id string
		if err = rows.Scan(&raw, &incomplete, &closed, &start, &saved, &id); err != nil {
			return result, err
		}
		var b traffic.Bucket
		if err = json.Unmarshal(raw, &b); err != nil {
			return result, err
		}
		spans[id] = runtimeSpan{start, saved}
		if incomplete || (!closed && saved.Before(time.Now().Add(-15*time.Second))) {
			result.Incomplete = true
		}
		if result.SavedAt == nil || saved.After(*result.SavedAt) {
			t := saved
			result.SavedAt = &t
		}
		result.Bytes.Add(b.Bytes)
		result.Quality.Merge(b.Quality)
		if result.Peaks == nil {
			result.Peaks = traffic.Peaks{}
		}
		result.Peaks.Merge(b.Peaks)
		if old := buckets[b.Start]; old != nil {
			old.Bytes.Add(b.Bytes)
			old.Peaks.Merge(b.Peaks)
			old.Quality.Merge(b.Quality)
		} else {
			copy := b
			buckets[b.Start] = &copy
		}
	}
	if err = rows.Err(); err != nil {
		return result, err
	}
	// Per-runtime peaks cannot be summed to reconstruct a simultaneous cluster peak.
	ordered := make([]runtimeSpan, 0, len(spans))
	for _, v := range spans {
		ordered = append(ordered, v)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].start.Before(ordered[j].start) })
	for i := 1; i < len(ordered); i++ {
		if ordered[i].start.Before(ordered[i-1].end) {
			result.ConcurrentRuntimes = true
		}
	}
	for _, b := range buckets {
		if result.ConcurrentRuntimes {
			b.Peaks = nil
		}
		result.History = append(result.History, *b)
	}
	sort.Slice(result.History, func(i, j int) bool { return result.History[i].Start.Before(result.History[j].Start) })
	if result.ConcurrentRuntimes {
		result.Peaks = nil
		result.Incomplete = true
	}
	result.HasRecords = len(result.History) > 0
	result.Incomplete = result.Incomplete || result.Quality.InvalidSamples > 0
	v := result.Bytes.Vector()
	result.InBytes = strconv.FormatUint(v[4], 10)
	result.OutBytes = strconv.FormatUint(v[5], 10)
	result.TotalBytes = strconv.FormatUint(v[6], 10)
	return result, nil
}
