package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
)

func saveTrafficEgress(ctx context.Context, tx *sql.Tx, s traffic.Snapshot) error {
	if len(s.Egress) == 0 {
		return nil
	}
	started := s.At
	for _, b := range s.Egress {
		if b.Quality.FirstSampleAt != nil && b.Quality.FirstSampleAt.Before(started) {
			started = *b.Quality.FirstSampleAt
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO service_traffic_egress_metadata(id,started_at) VALUES(TRUE,$1) ON CONFLICT(id) DO UPDATE SET started_at=LEAST(service_traffic_egress_metadata.started_at,EXCLUDED.started_at)`, started); err != nil {
		return err
	}
	const columns = "runtime_id,bucket_start,route_type,route_id,account_id,sequence,upstream_out,upstream_in,attempts,peak_out,peak_out_at,peak_out_window,peak_in,peak_in_at,peak_in_window,peak_total,peak_total_at,peak_total_window,first_sample_at,last_sample_at,valid_samples,invalid_samples,overflow"
	updates := []string{}
	for _, col := range strings.Split(columns, ",")[5:] {
		updates = append(updates, col+"=EXCLUDED."+col)
	}
	for offset := 0; offset < len(s.Egress); offset += 500 {
		end := offset + 500
		if end > len(s.Egress) {
			end = len(s.Egress)
		}
		args := []any{}
		values := []string{}
		for _, b := range s.Egress[offset:end] {
			row := []any{s.RuntimeID, b.Start, b.Type, b.ID, b.AccountID, b.Sequence, strconv.FormatUint(b.Bytes.Out, 10), strconv.FormatUint(b.Bytes.In, 10), b.Attempts}
			for _, key := range []string{"out", "in", "total"} {
				if p, ok := b.Peaks[key]; ok {
					row = append(row, p.BytesPerSecond, p.At, p.WindowSeconds)
				} else {
					row = append(row, nil, nil, nil)
				}
			}
			row = append(row, b.Quality.FirstSampleAt, b.Quality.LastSampleAt, b.Quality.ValidSamples, b.Quality.InvalidSamples, b.Overflow)
			slots := []string{}
			for _, v := range row {
				args = append(args, v)
				slots = append(slots, fmt.Sprintf("$%d", len(args)))
			}
			values = append(values, "("+strings.Join(slots, ",")+")")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO service_traffic_egress_buckets (`+columns+`) VALUES `+strings.Join(values, ",")+` ON CONFLICT(runtime_id,bucket_start,route_type,route_id,account_id) DO UPDATE SET `+strings.Join(updates, ",")+` WHERE service_traffic_egress_buckets.sequence<EXCLUDED.sequence`, args...); err != nil {
			return err
		}
	}
	return nil
}
