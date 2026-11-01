package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/egress"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
)

func (s *WireGuardStore) SaveObservation(ctx context.Context, key string, e wgtunnel.Evidence) error {
	if key == "" || e.State != "observed" || e.Source != "route_probe" || !e.ExpiresAt.After(e.ObservedAt) {
		return errors.New("invalid egress observation")
	}
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO egress_observations(route_key,family,snapshot,observed_at,expires_at) VALUES($1,$2,$3,$4,$5)`, key, e.Family, string(data), e.ObservedAt, e.ExpiresAt)
	return err
}
func (s *WireGuardStore) LatestObservation(ctx context.Context, key, family string) (wgtunnel.Evidence, error) {
	var data []byte
	err := s.DB.QueryRowContext(ctx, `SELECT snapshot FROM egress_observations WHERE route_key=$1 AND family=$2 ORDER BY observed_at DESC LIMIT 1`, key, family).Scan(&data)
	if err != nil {
		return wgtunnel.Evidence{}, err
	}
	var e wgtunnel.Evidence
	if err = json.Unmarshal(data, &e); err != nil {
		return e, err
	}
	if !e.ExpiresAt.After(time.Now()) {
		e.State = "expired"
	}
	return e, nil
}

// Runtime high-watermarks are intentionally retained: deleting them would let
// an old checkpoint be counted again. Only detailed charts/observations expire.
func (s *WireGuardStore) PruneTrafficHistory(ctx context.Context, now time.Time) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, q := range []struct {
		sql    string
		before time.Time
	}{
		{`DELETE FROM wireguard_traffic_buckets WHERE bucket_start<$1`, now.Add(-7 * 24 * time.Hour)},
		{`DELETE FROM wireguard_traffic_hourly WHERE bucket_start<$1`, now.Add(-90 * 24 * time.Hour)},
		{`DELETE FROM egress_observations WHERE expires_at<$1`, now.Add(-7 * 24 * time.Hour)},
		{`DELETE FROM upstream_egress_attempts WHERE started_at<$1`, now.Add(-90 * 24 * time.Hour)},
	} {
		if _, err = tx.ExecContext(ctx, q.sql, q.before); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *WireGuardStore) SaveAttempt(ctx context.Context, a egress.Attempt) error {
	data, err := json.Marshal(a.Snapshot)
	if err != nil {
		return err
	}
	aliases, _ := json.Marshal(a.CorrelationIDs)
	_, err = s.DB.ExecContext(ctx, `INSERT INTO upstream_egress_attempts(attempt_id,request_id,account_id,route_key,started_at,finished_at,snapshot,outcome,correlation_ids) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(attempt_id) DO NOTHING`, a.ID, a.RequestID, a.AccountID, a.RouteKey, a.StartedAt, a.FinishedAt, string(data), a.Outcome, string(aliases))
	return err
}
func (s *WireGuardStore) ReadAttempts(ctx context.Context, requestID string) ([]egress.Attempt, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT attempt_id,request_id,account_id,route_key,started_at,finished_at,snapshot,outcome FROM upstream_egress_attempts WHERE request_id=$1 OR correlation_ids ? $1 ORDER BY started_at LIMIT 100`, requestID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := []egress.Attempt{}
	for rows.Next() {
		var a egress.Attempt
		var data []byte
		if err = rows.Scan(&a.ID, &a.RequestID, &a.AccountID, &a.RouteKey, &a.StartedAt, &a.FinishedAt, &data, &a.Outcome); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(data, &a.Snapshot); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// ReadExitSummaries loads one page in a single query and keeps accounts separate
// when retries or routing changes share the same request correlation ID.
func (s *WireGuardStore) ReadExitSummaries(ctx context.Context, queries []egress.AttemptQuery) ([]egress.ExitSummary, error) {
	result := make([]egress.ExitSummary, len(queries))
	for i, q := range queries {
		result[i] = egress.ExitSummary{AttemptQuery: q, IPs: []string{}}
	}
	if len(queries) == 0 {
		return result, nil
	}
	data, err := json.Marshal(queries)
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `
		WITH requested AS (
			SELECT ordinality - 1 AS idx, value->>'request_id' AS request_id,
				(value->>'account_id')::bigint AS account_id
			FROM jsonb_array_elements($1::jsonb) WITH ORDINALITY
		)
		SELECT r.idx, COALESCE(a.snapshot->'evidence'->>'state',''),
			COALESCE(a.snapshot->'evidence'->>'ip','')
		FROM requested r JOIN upstream_egress_attempts a
			ON (a.request_id=r.request_id OR a.correlation_ids ? r.request_id)
			AND a.account_id=r.account_id
		GROUP BY 1,2,3`, string(data))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var i int
		var state, ip string
		if err = rows.Scan(&i, &state, &ip); err != nil {
			return nil, err
		}
		result[i].Add(state, ip)
	}
	return result, rows.Err()
}
