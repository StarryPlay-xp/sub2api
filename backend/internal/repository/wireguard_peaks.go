package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
)

// Merge only the newly observed sample counts. Peaks are monotone maxima;
// checkpoint retries are gated by the runtime sequence in SaveCheckpoint.
func saveWireGuardPeaks(ctx context.Context, tx *sql.Tx, c wgtunnel.Checkpoint, b wgtunnel.Bucket, oldRaw []byte) error {
	if b.PeakHistory == nil {
		return nil
	}
	var old traffic.PeakHistory
	if len(oldRaw) > 0 {
		if err := json.Unmarshal(oldRaw, &old); err != nil {
			return err
		}
	}
	delta := *b.PeakHistory.Clone()
	if delta.Quality.ValidSamples < old.Quality.ValidSamples || delta.Quality.InvalidSamples < old.Quality.InvalidSamples {
		return errors.New("wireguard peak sample counters regressed")
	}
	delta.Quality.ValidSamples -= old.Quality.ValidSamples
	delta.Quality.InvalidSamples -= old.Quality.InvalidSamples
	raw, err := json.Marshal(b.PeakHistory)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE wireguard_traffic_buckets SET peak_data=$3 WHERE runtime_id=$1 AND bucket_start=$2`, c.RuntimeID, b.Start, string(raw)); err != nil {
		return err
	}
	var hourlyRaw []byte
	if err = tx.QueryRowContext(ctx, `SELECT peak_data FROM wireguard_traffic_hourly WHERE proxy_id=$1 AND bucket_start=date_trunc('hour',$2::timestamptz,'UTC') FOR UPDATE`, c.NodeID, b.Start).Scan(&hourlyRaw); err != nil {
		return err
	}
	var hourly traffic.PeakHistory
	if len(hourlyRaw) > 0 {
		if err = json.Unmarshal(hourlyRaw, &hourly); err != nil {
			return err
		}
	}
	hourly.Merge(&delta)
	raw, err = json.Marshal(hourly)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE wireguard_traffic_hourly SET peak_data=$3 WHERE proxy_id=$1 AND bucket_start=date_trunc('hour',$2::timestamptz,'UTC')`, c.NodeID, b.Start, string(raw))
	return err
}
