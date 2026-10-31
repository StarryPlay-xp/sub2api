package repository

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
	"strconv"
)

func (s *WireGuardStore) TrafficHistory(ctx context.Context, id int64, interval string) ([]wgtunnel.Bucket, error) {
	query := `SELECT b.bucket_start,SUM(b.tx_bytes)::text,SUM(b.rx_bytes)::text FROM wireguard_traffic_buckets b JOIN wireguard_traffic_runtimes r ON r.runtime_id=b.runtime_id WHERE r.proxy_id=$1 AND b.bucket_start>=NOW()-INTERVAL '24 hours' GROUP BY b.bucket_start ORDER BY b.bucket_start`
	if interval == "hour" {
		query = `SELECT bucket_start,tx_bytes::text,rx_bytes::text FROM wireguard_traffic_hourly WHERE proxy_id=$1 AND bucket_start>=NOW()-INTERVAL '90 days' ORDER BY bucket_start`
	} else if interval != "" && interval != "minute" {
		return nil, errors.New("invalid traffic interval")
	}
	rows, err := s.DB.QueryContext(ctx, query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []wgtunnel.Bucket{}
	for rows.Next() {
		var b wgtunnel.Bucket
		if err = rows.Scan(&b.Start, &b.TX, &b.RX); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
func (s *WireGuardStore) NodeObservations(ctx context.Context, id int64) ([]wgtunnel.Evidence, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT ON(family) snapshot FROM egress_observations WHERE snapshot->>'node_id'=$1 AND snapshot->>'route'='wireguard' ORDER BY family,observed_at DESC`, strconv.FormatInt(id, 10))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []wgtunnel.Evidence{}
	for rows.Next() {
		var data []byte
		var e wgtunnel.Evidence
		if err = rows.Scan(&data); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
