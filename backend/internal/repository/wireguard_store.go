package repository

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
)

type WireGuardStore struct{ DB *sql.DB }

func (s *WireGuardStore) LoadNode(ctx context.Context, id int64) (wgtunnel.Node, error) {
	var n wgtunnel.Node
	err := s.DB.QueryRowContext(ctx, `SELECT p.id,w.config_version,w.config_ciphertext,
        p.status='active' AND p.deleted_at IS NULL,w.paused,p.expires_at
        FROM proxies p JOIN wireguard_nodes w ON w.proxy_id=p.id
        WHERE p.id=$1 AND p.protocol='wireguard'`, id).Scan(&n.ID, &n.Version, &n.Ciphertext, &n.Active, &n.Paused, &n.ExpiresAt)
	return n, err
}

// SaveCheckpoint atomically advances a runtime watermark, lifetime totals and
// minute/hour buckets. A commit whose acknowledgement was lost can be retried.
func (s *WireGuardStore) SaveCheckpoint(ctx context.Context, c wgtunnel.Checkpoint, gap bool) error {
	if c.RuntimeID == "" || c.NodeID <= 0 || c.Seq < 0 {
		return errors.New("invalid WireGuard checkpoint")
	}
	cutoff := time.Now().UTC().Add(-7 * 24 * time.Hour).Truncate(time.Minute)
	for _, b := range c.Buckets {
		if b.Start.Before(cutoff) {
			gap = true
		}
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback() }()
	_, e = tx.ExecContext(ctx, `INSERT INTO wireguard_traffic_runtimes(runtime_id,proxy_id,config_version)
        VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, c.RuntimeID, c.NodeID, c.Version)
	if e != nil {
		return e
	}
	var oldSeq, node, version int64
	var oldTX, oldRX string
	var closed bool
	e = tx.QueryRowContext(ctx, `SELECT sequence,proxy_id,config_version,tx_bytes::text,rx_bytes::text,closed
        FROM wireguard_traffic_runtimes WHERE runtime_id=$1 FOR UPDATE`, c.RuntimeID).Scan(&oldSeq, &node, &version, &oldTX, &oldRX, &closed)
	if e != nil {
		return e
	}
	if node != c.NodeID || version != c.Version {
		return errors.New("WireGuard runtime identity mismatch")
	}
	if closed || oldSeq >= c.Seq {
		return tx.Commit()
	}
	txOld, e := strconv.ParseUint(oldTX, 10, 64)
	if e != nil {
		return e
	}
	rxOld, e := strconv.ParseUint(oldRX, 10, 64)
	if e != nil {
		return e
	}
	if c.TX < txOld || c.RX < rxOld {
		return errors.New("WireGuard counter reset within runtime")
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO wireguard_traffic_totals(proxy_id,tx_bytes,rx_bytes,saved_at,incomplete)
        VALUES($1,$2,$3,$4,$5) ON CONFLICT(proxy_id) DO UPDATE SET
        tx_bytes=wireguard_traffic_totals.tx_bytes+EXCLUDED.tx_bytes,
        rx_bytes=wireguard_traffic_totals.rx_bytes+EXCLUDED.rx_bytes,
        saved_at=GREATEST(wireguard_traffic_totals.saved_at,EXCLUDED.saved_at),
        incomplete=wireguard_traffic_totals.incomplete OR EXCLUDED.incomplete`, c.NodeID, strconv.FormatUint(c.TX-txOld, 10), strconv.FormatUint(c.RX-rxOld, 10), c.At, gap)
	if e != nil {
		return e
	}
	for _, b := range c.Buckets {
		if b.Start.Before(cutoff) {
			continue
		}
		var bt, br string
		var oldPeak []byte
		e = tx.QueryRowContext(ctx, `SELECT tx_bytes::text,rx_bytes::text,peak_data FROM wireguard_traffic_buckets
            WHERE runtime_id=$1 AND bucket_start=$2`, c.RuntimeID, b.Start).Scan(&bt, &br, &oldPeak)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		var ot, or uint64
		if e == nil {
			ot, e = strconv.ParseUint(bt, 10, 64)
			if e != nil {
				return e
			}
			or, e = strconv.ParseUint(br, 10, 64)
			if e != nil {
				return e
			}
		}
		if b.TX < ot || b.RX < or {
			return errors.New("WireGuard bucket counter decreased")
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO wireguard_traffic_buckets(runtime_id,bucket_start,tx_bytes,rx_bytes)
            VALUES($1,$2,$3,$4) ON CONFLICT(runtime_id,bucket_start) DO UPDATE SET tx_bytes=EXCLUDED.tx_bytes,rx_bytes=EXCLUDED.rx_bytes`, c.RuntimeID, b.Start, strconv.FormatUint(b.TX, 10), strconv.FormatUint(b.RX, 10))
		if e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO wireguard_traffic_hourly(proxy_id,bucket_start,tx_bytes,rx_bytes)
            VALUES($1,date_trunc('hour',$2::timestamptz,'UTC'),$3,$4) ON CONFLICT(proxy_id,bucket_start) DO UPDATE SET
            tx_bytes=wireguard_traffic_hourly.tx_bytes+EXCLUDED.tx_bytes,rx_bytes=wireguard_traffic_hourly.rx_bytes+EXCLUDED.rx_bytes`, c.NodeID, b.Start, strconv.FormatUint(b.TX-ot, 10), strconv.FormatUint(b.RX-or, 10))
		if e != nil {
			return e
		}
		if e = saveWireGuardPeaks(ctx, tx, c, b, oldPeak); e != nil {
			return e
		}
	}
	_, e = tx.ExecContext(ctx, `UPDATE wireguard_traffic_runtimes SET sequence=$2,tx_bytes=$3,rx_bytes=$4,saved_at=$5,closed=$6 WHERE runtime_id=$1`, c.RuntimeID, c.Seq, strconv.FormatUint(c.TX, 10), strconv.FormatUint(c.RX, 10), c.At, c.Closed)
	if e != nil {
		return e
	}
	if c.Closed {
		if _, e = tx.ExecContext(ctx, `DELETE FROM wireguard_runtime_leases WHERE proxy_id=$1 AND runtime_id=$2`, c.NodeID, c.RuntimeID); e != nil {
			return e
		}
	}
	return tx.Commit()
}
