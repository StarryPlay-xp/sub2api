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
	defer tx.Rollback()
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
	defer rows.Close()
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
