package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type trafficSQLReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// The database reduces hourly data to one row per subject. Only these compact
// aggregates cross into Go for shared, exact-integer ranking/filter semantics.
func trafficAggregateColumns() string {
	columns := []string{"SUM(client_in)::text", "SUM(client_out)::text", "SUM(upstream_out)::text", "SUM(upstream_in)::text", "SUM(requests)", "SUM(active_seconds)"}
	for _, key := range []string{"in", "out", "total"} {
		column := "peak_" + key
		columns = append(columns, "MAX("+column+")", "(array_agg("+column+"_at ORDER BY "+column+" DESC NULLS LAST,"+column+"_at))[1]", "(array_agg("+column+"_window ORDER BY "+column+" DESC NULLS LAST,"+column+"_at))[1]")
	}
	return strings.Join(columns, ",")
}
func scanTrafficAggregate(rows *sql.Rows, prefix []any) (traffic.Bytes, uint64, uint64, traffic.Peaks, error) {
	var b traffic.Bytes
	var requests, active uint64
	var rates, seconds [3]sql.NullFloat64
	var at [3]sql.NullTime
	args := append(prefix, &b.ClientIn, &b.ClientOut, &b.UpstreamOut, &b.UpstreamIn, &requests, &active)
	for i := 0; i < 3; i++ {
		args = append(args, &rates[i], &at[i], &seconds[i])
	}
	if err := rows.Scan(args...); err != nil {
		return b, 0, 0, nil, err
	}
	var peaks traffic.Peaks
	for i, key := range []string{"in", "out", "total"} {
		if rates[i].Valid && at[i].Valid && seconds[i].Valid {
			if peaks == nil {
				peaks = traffic.Peaks{}
			}
			peaks[key] = traffic.Peak{BytesPerSecond: rates[i].Float64, At: at[i].Time, WindowSeconds: seconds[i].Float64}
		}
	}
	return b, requests, active, peaks, nil
}
func readTrafficAggregates(ctx context.Context, db trafficSQLReader, subject string, w wgtunnel.TrafficWindow) ([]service.TrafficSubjectStats, traffic.Bytes, error) {
	rows, err := db.QueryContext(ctx, `SELECT subject_type,subject_id,user_id,`+trafficAggregateColumns()+` FROM service_traffic_subject_buckets WHERE bucket_start >= $1 AND bucket_start < $2 AND subject_type IN ($3,'unattributed') GROUP BY subject_type,subject_id,user_id HAVING SUM(client_in+client_out+upstream_out+upstream_in)>0 OR SUM(requests)>0`, w.Start, w.End, subject)
	if err != nil {
		return nil, traffic.Bytes{}, err
	}
	defer func() { _ = rows.Close() }()
	items := []service.TrafficSubjectStats{}
	var unattributed traffic.Bytes
	for rows.Next() {
		var typ string
		var id, userID int64
		b, requests, active, peaks, e := scanTrafficAggregate(rows, []any{&typ, &id, &userID})
		if e != nil {
			return nil, unattributed, e
		}
		if typ == "unattributed" {
			unattributed.Add(b)
			continue
		}
		identity := service.TrafficIdentity{SubjectType: typ, UserID: userID, UserDeleted: true}
		if typ == "api_key" {
			identity.APIKeyID = &id
			deleted := true
			identity.APIKeyDeleted = &deleted
		}
		items = append(items, service.TrafficSubjectStats{TrafficIdentity: identity, Bytes: b, Requests: requests, ActiveSeconds: active, Peaks: peaks})
	}
	if err = rows.Err(); err != nil {
		return nil, unattributed, err
	}
	_ = rows.Close()
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID())
	}
	identities, err := readTrafficIdentities(ctx, db, subject, ids)
	if err != nil {
		return nil, unattributed, err
	}
	for i := range items {
		if identity, ok := identities[items[i].ID()]; ok {
			items[i].TrafficIdentity = identity
		}
	}
	if subject == "user" {
		rows, err = db.QueryContext(ctx, `SELECT user_id,COUNT(DISTINCT subject_id) FROM service_traffic_subject_buckets WHERE subject_type='api_key' AND bucket_start >= $1 AND bucket_start < $2 AND client_in+client_out+upstream_out+upstream_in>0 GROUP BY user_id`, w.Start, w.End)
		if err != nil {
			return nil, unattributed, err
		}
		defer func() { _ = rows.Close() }()
		counts := map[int64]uint64{}
		for rows.Next() {
			var id int64
			var count uint64
			if err = rows.Scan(&id, &count); err != nil {
				return nil, unattributed, err
			}
			counts[id] = count
		}
		if err = rows.Err(); err != nil {
			return nil, unattributed, err
		}
		for i := range items {
			n := counts[items[i].UserID]
			items[i].ActiveAPIKeys = &n
		}
	}
	return items, unattributed, nil
}
func readTrafficIdentities(ctx context.Context, db trafficSQLReader, subject string, ids []int64) (map[int64]service.TrafficIdentity, error) {
	identities := map[int64]service.TrafficIdentity{}
	if len(ids) == 0 {
		return identities, nil
	}
	query := `SELECT u.id,u.email,u.username,u.deleted_at IS NOT NULL,NULL::bigint,NULL::text,NULL::boolean,NULL::bigint,NULL::text FROM users u WHERE u.id=ANY($1)`
	if subject == "api_key" {
		query = `SELECT k.user_id,COALESCE(u.email,''),COALESCE(u.username,''),u.id IS NULL OR u.deleted_at IS NOT NULL,k.id,k.name,k.deleted_at IS NOT NULL,k.group_id,g.name FROM api_keys k LEFT JOIN users u ON u.id=k.user_id LEFT JOIN groups g ON g.id=k.group_id WHERE k.id=ANY($1)`
	}
	rows, err := db.QueryContext(ctx, query, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		i := service.TrafficIdentity{SubjectType: subject}
		if err = rows.Scan(&i.UserID, &i.UserEmail, &i.Username, &i.UserDeleted, &i.APIKeyID, &i.APIKeyName, &i.APIKeyDeleted, &i.GroupID, &i.GroupName); err != nil {
			return nil, err
		}
		identities[i.ID()] = i
	}
	return identities, rows.Err()
}
func (s *ServiceTrafficStore) SubjectIdentities(ctx context.Context, subject string, ids []int64) (map[int64]service.TrafficIdentity, error) {
	return readTrafficIdentities(ctx, s.DB, subject, ids)
}

func trafficWindowMetadata(ctx context.Context, db trafficSQLReader, w wgtunnel.TrafficWindow) (service.TrafficRankingWindow, error) {
	result := service.TrafficRankingWindow{TrafficWindow: w, MetricBasis: traffic.MetricBasis}
	if err := db.QueryRowContext(ctx, `SELECT (SELECT started_at FROM service_traffic_attribution_metadata WHERE id=TRUE),COUNT(*)>0,COALESCE(BOOL_OR(subject_overflow),FALSE),COALESCE(BOOL_OR(invalid_samples>0),FALSE) FROM service_traffic_subject_buckets WHERE bucket_start >= $1 AND bucket_start < $2`, w.Start, w.End).Scan(&result.AttributionStartedAt, &result.HasRecords, &result.SubjectOverflow, &result.Incomplete); err != nil {
		return result, err
	}
	rows, err := db.QueryContext(ctx, `SELECT started_at,saved_at,closed,incomplete FROM service_traffic_runtimes WHERE started_at < $2 AND saved_at > $1 ORDER BY started_at`, w.Start, w.End)
	if err != nil {
		return result, err
	}
	defer func() { _ = rows.Close() }()
	var latestEnd time.Time
	for rows.Next() {
		var start, end time.Time
		var closed, incomplete bool
		if err = rows.Scan(&start, &end, &closed, &incomplete); err != nil {
			return result, err
		}
		if start.Before(latestEnd) {
			result.ConcurrentRuntimes = true
		}
		if end.After(latestEnd) {
			latestEnd = end
		}
		if incomplete || (!closed && end.Before(w.AsOf.Add(-15*time.Second))) {
			result.Incomplete = true
		}
	}
	if result.ConcurrentRuntimes || result.SubjectOverflow || result.AttributionStartedAt == nil || w.Start.Before(*result.AttributionStartedAt) {
		result.Incomplete = true
	}
	return result, rows.Err()
}
func (s *ServiceTrafficStore) SubjectData(ctx context.Context, q service.TrafficSubjectQuery) (service.TrafficSubjectData, error) {
	result := service.TrafficSubjectData{History: []service.TrafficSubjectHistory{}}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	result.Window, err = trafficWindowMetadata(ctx, tx, q.Window)
	if err != nil {
		return result, err
	}
	result.Current, result.Unattributed, err = readTrafficAggregates(ctx, tx, q.Subject, q.Window)
	if err != nil {
		return result, err
	}
	if result.Window.ConcurrentRuntimes {
		for i := range result.Current {
			result.Current[i].Peaks = nil
		}
	}
	if q.Compare {
		previous := q.Window
		previous.End = q.Window.Start
		endHour := q.Window.End.UTC().Truncate(time.Hour)
		if endHour.Before(q.Window.End) {
			endHour = endHour.Add(time.Hour)
		}
		previous.Start = previous.End.Add(-endHour.Sub(q.Window.Start))
		result.PreviousWindow = &service.TrafficPreviousWindow{Start: previous.Start, End: previous.End}
		meta, e := trafficWindowMetadata(ctx, tx, previous)
		if e != nil {
			return result, e
		}
		result.PreviousWindow.Complete = !meta.Incomplete && !previous.Start.Before(q.Window.RetainedFrom) && meta.AttributionStartedAt != nil && !previous.Start.Before(*meta.AttributionStartedAt)
		// Expired buckets are excluded even if the hourly cleanup has not run yet.
		if previous.Start.Before(q.Window.RetainedFrom) {
			previous.Start = q.Window.RetainedFrom
		}
		result.Previous, _, err = readTrafficAggregates(ctx, tx, q.Subject, previous)
		if err != nil {
			return result, err
		}
		if meta.ConcurrentRuntimes {
			for i := range result.Previous {
				result.Previous[i].Peaks = nil
			}
		}
	}
	if q.ID > 0 {
		identities, e := readTrafficIdentities(ctx, tx, q.Subject, []int64{q.ID})
		if e != nil {
			return result, e
		}
		if identity, ok := identities[q.ID]; ok {
			result.Identity = &identity
		} else {
			// A hard-deleted identity may still be represented in retained traffic.
			var userID int64
			err = tx.QueryRowContext(ctx, `SELECT user_id FROM service_traffic_subject_buckets WHERE subject_type=$1 AND subject_id=$2 LIMIT 1`, q.Subject, q.ID).Scan(&userID)
			if err != nil && err != sql.ErrNoRows {
				return result, err
			}
			if err == nil {
				identity := service.TrafficIdentity{SubjectType: q.Subject, UserID: userID, UserDeleted: true}
				if q.Subject == "api_key" {
					id := q.ID
					deleted := true
					identity.APIKeyID = &id
					identity.APIKeyDeleted = &deleted
				}
				result.Identity = &identity
			}
		}
		result.History, err = readTrafficSubjectHistory(ctx, tx, q, result.Window.ConcurrentRuntimes)
		if err != nil {
			return result, err
		}
	}
	return result, tx.Commit()
}
func readTrafficSubjectHistory(ctx context.Context, db trafficSQLReader, q service.TrafficSubjectQuery, concurrent bool) ([]service.TrafficSubjectHistory, error) {
	query := fmt.Sprintf(`SELECT bucket_start,%s FROM service_traffic_subject_buckets WHERE bucket_start >= $1 AND bucket_start < $2 AND subject_type=$3 AND subject_id=$4 GROUP BY bucket_start ORDER BY bucket_start`, trafficAggregateColumns())
	rows, err := db.QueryContext(ctx, query, q.Window.Start, q.Window.End, q.Subject, q.ID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	history := []service.TrafficSubjectHistory{}
	for rows.Next() {
		var start time.Time
		b, requests, active, peaks, e := scanTrafficAggregate(rows, []any{&start})
		if e != nil {
			return nil, e
		}
		if concurrent {
			peaks = nil
		}
		history = append(history, service.TrafficSubjectHistory{Start: start, Interval: "hour", Bytes: b, Requests: requests, ActiveSeconds: active, Peaks: peaks})
	}
	return history, rows.Err()
}
