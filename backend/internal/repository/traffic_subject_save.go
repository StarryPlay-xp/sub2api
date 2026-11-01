package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
)

func saveTrafficSubjects(ctx context.Context, tx *sql.Tx, s traffic.Snapshot) error {
	if len(s.Subjects) == 0 {
		return nil
	}
	started := s.At
	for _, b := range s.Subjects {
		if b.Quality.FirstSampleAt != nil && b.Quality.FirstSampleAt.Before(started) {
			started = *b.Quality.FirstSampleAt
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO service_traffic_attribution_metadata(id,started_at) VALUES(TRUE,$1) ON CONFLICT(id) DO UPDATE SET started_at=LEAST(service_traffic_attribution_metadata.started_at,EXCLUDED.started_at)`, started); err != nil {
		return err
	}
	const columns = "runtime_id,bucket_start,subject_type,subject_id,user_id,sequence,client_in,client_out,upstream_out,upstream_in,requests,active_seconds,peak_in,peak_in_at,peak_in_window,peak_out,peak_out_at,peak_out_window,peak_total,peak_total_at,peak_total_window,first_sample_at,last_sample_at,valid_samples,invalid_samples,subject_overflow"
	updates := []string{}
	for _, col := range strings.Split(columns, ",")[4:] {
		updates = append(updates, col+"=EXCLUDED."+col)
	}
	// Bounded batches keep well below PostgreSQL's parameter limit.
	for offset := 0; offset < len(s.Subjects); offset += 500 {
		end := offset + 500
		if end > len(s.Subjects) {
			end = len(s.Subjects)
		}
		args := []any{}
		values := []string{}
		for _, b := range s.Subjects[offset:end] {
			row := []any{s.RuntimeID, b.Start, b.Type, b.ID, b.UserID, b.Sequence, fmt.Sprint(b.Bytes.ClientIn), fmt.Sprint(b.Bytes.ClientOut), fmt.Sprint(b.Bytes.UpstreamOut), fmt.Sprint(b.Bytes.UpstreamIn), b.Requests, b.ActiveSeconds}
			for _, key := range []string{"in", "out", "total"} {
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
		query := `INSERT INTO service_traffic_subject_buckets (` + columns + `) VALUES ` + strings.Join(values, ",") + ` ON CONFLICT(runtime_id,bucket_start,subject_type,subject_id) DO UPDATE SET ` + strings.Join(updates, ",") + ` WHERE service_traffic_subject_buckets.sequence<EXCLUDED.sequence`
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return err
		}
	}
	return nil
}
