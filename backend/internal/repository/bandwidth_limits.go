package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
)

const bandwidthHostKey = "ops_bandwidth_limit:host"
const bandwidthNodePrefix = "ops_bandwidth_limit:node:"

type BandwidthLimitStore struct{ DB *sql.DB }

func (s *BandwidthLimitStore) ReadBandwidthLimits(ctx context.Context) (service.BandwidthLimits, error) {
	result := service.BandwidthLimits{Nodes: map[string]service.BandwidthLimit{}}
	rows, err := s.DB.QueryContext(ctx, `SELECT s.key,s.value FROM settings s
 WHERE s.key=$1 OR EXISTS (SELECT 1 FROM proxies p WHERE s.key=$2 || p.id::text
 AND p.protocol='wireguard' AND p.deleted_at IS NULL)`, bandwidthHostKey, bandwidthNodePrefix)
	if err != nil {
		return result, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var key, raw string
		var value service.BandwidthLimit
		if err = rows.Scan(&key, &raw); err != nil {
			return result, err
		}
		if err = json.Unmarshal([]byte(raw), &value); err != nil {
			return result, err
		}
		if err = service.ValidateBandwidthLimit(0, value.Version, value.Mbps); err != nil {
			return result, err
		}
		if key == bandwidthHostKey {
			result.Host = value
		} else {
			result.Nodes[strings.TrimPrefix(key, bandwidthNodePrefix)] = value
		}
	}
	return result, rows.Err()
}

func (s *BandwidthLimitStore) UpdateBandwidthLimit(ctx context.Context, nodeID int64, expectedVersion string, mbps *float64) (service.BandwidthLimit, error) {
	var result service.BandwidthLimit
	if err := service.ValidateBandwidthLimit(nodeID, expectedVersion, mbps); err != nil {
		return result, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	key := bandwidthHostKey
	if nodeID > 0 {
		// Serialize against node deletion without touching its WG configuration.
		var id int64
		err = tx.QueryRowContext(ctx, `SELECT id FROM proxies WHERE id=$1 AND protocol='wireguard' AND deleted_at IS NULL FOR SHARE`, nodeID).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return result, service.ErrBandwidthNodeNotFound
		}
		if err != nil {
			return result, err
		}
		key = bandwidthNodePrefix + strconv.FormatInt(nodeID, 10)
	}
	result = service.BandwidthLimit{Mbps: mbps, Version: uuid.NewString()}
	raw, err := json.Marshal(result)
	if err != nil {
		return result, err
	}
	var saved string
	if expectedVersion == "" {
		err = tx.QueryRowContext(ctx, `INSERT INTO settings(key,value,updated_at) VALUES($1,$2,NOW())
 ON CONFLICT(key) DO NOTHING RETURNING value`, key, string(raw)).Scan(&saved)
	} else {
		err = tx.QueryRowContext(ctx, `UPDATE settings SET value=$2,updated_at=NOW()
 WHERE key=$1 AND value::jsonb->>'version'=$3 RETURNING value`, key, string(raw), expectedVersion).Scan(&saved)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return service.BandwidthLimit{}, service.ErrBandwidthLimitConflict
	}
	if err != nil {
		return service.BandwidthLimit{}, err
	}
	if err = tx.Commit(); err != nil {
		return service.BandwidthLimit{}, err
	}
	return result, nil
}
