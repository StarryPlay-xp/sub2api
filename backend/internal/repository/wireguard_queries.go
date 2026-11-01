package repository

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
	"math/big"
	"strconv"
)

func (s *WireGuardStore) TrafficReport(ctx context.Context, id int64, window wgtunnel.TrafficWindow) (wgtunnel.TrafficReport, error) {
	result := wgtunnel.TrafficReport{TrafficWindow: window, TX: "0", RX: "0", Total: "0", MetricBasis: wgtunnel.MetricBasis, PeakWindowSeconds: 1, History: []wgtunnel.Bucket{}}
	query := `SELECT bucket_start,tx_bytes::text,rx_bytes::text,peak_data FROM wireguard_traffic_hourly
		WHERE proxy_id=$1 AND bucket_start >= $2 AND bucket_start < $3 ORDER BY bucket_start`
	if window.Interval == "minute" {
		query = `SELECT b.bucket_start,b.tx_bytes::text,b.rx_bytes::text,b.peak_data
			FROM wireguard_traffic_buckets b JOIN wireguard_traffic_runtimes r ON r.runtime_id=b.runtime_id
			WHERE r.proxy_id=$1 AND b.bucket_start >= $2 AND b.bucket_start < $3
			ORDER BY b.bucket_start`
	} else if window.Interval != "hour" {
		return result, errors.New("invalid traffic interval")
	}
	rows, err := s.DB.QueryContext(ctx, query, id, window.Start, window.End)
	if err != nil {
		return result, err
	}
	defer func() { _ = rows.Close() }()
	tx, rx := new(big.Int), new(big.Int)
	for rows.Next() {
		var bucket wgtunnel.Bucket
		var raw []byte
		if err = rows.Scan(&bucket.Start, &bucket.TX, &bucket.RX, &raw); err != nil {
			return result, err
		}
		if len(raw) > 0 {
			bucket.PeakHistory = &traffic.PeakHistory{}
			if err = json.Unmarshal(raw, bucket.PeakHistory); err != nil {
				return result, err
			}
			if result.PeakHistory == nil {
				result.PeakHistory = &traffic.PeakHistory{}
			}
			result.PeakHistory.Merge(bucket.PeakHistory)
		}
		if n := len(result.History); n > 0 && result.History[n-1].Start.Equal(bucket.Start) {
			prior := &result.History[n-1]
			prior.TX += bucket.TX
			prior.RX += bucket.RX
			if bucket.PeakHistory != nil {
				if prior.PeakHistory == nil {
					prior.PeakHistory = &traffic.PeakHistory{}
				}
				prior.PeakHistory.Merge(bucket.PeakHistory)
			}
		} else {
			result.History = append(result.History, bucket)
		}
		tx.Add(tx, new(big.Int).SetUint64(bucket.TX))
		rx.Add(rx, new(big.Int).SetUint64(bucket.RX))
	}
	if err = rows.Err(); err != nil {
		return result, err
	}
	result.TX, result.RX, result.Total = tx.String(), rx.String(), new(big.Int).Add(tx, rx).String()
	result.BucketCount = len(result.History)
	result.HasRecords = result.BucketCount > 0
	if result.HasRecords {
		result.FirstRecordAt = &result.History[0].Start
		result.LastRecordAt = &result.History[result.BucketCount-1].Start
	}
	return result, nil
}

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
	defer func() { _ = rows.Close() }()
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
	defer func() { _ = rows.Close() }()
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
