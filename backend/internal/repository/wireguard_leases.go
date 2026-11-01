package repository

import (
	"context"
	"database/sql"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
)

func (s *WireGuardStore) ClaimRuntime(ctx context.Context, id, version int64, runtimeID string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var paused bool
	var current int64
	if err = tx.QueryRowContext(ctx, `SELECT paused,config_version FROM wireguard_nodes WHERE proxy_id=$1 FOR UPDATE`, id).Scan(&paused, &current); err != nil {
		return err
	}
	if paused || current != version {
		return wgtunnel.ErrBlocked
	}
	var owner string
	var expired bool
	err = tx.QueryRowContext(ctx, `SELECT runtime_id,expires_at<=NOW() FROM wireguard_runtime_leases WHERE proxy_id=$1 FOR UPDATE`, id).Scan(&owner, &expired)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if owner != "" {
		if !expired {
			return wgtunnel.ErrBlocked
		}
		// A crash or lost lease leaves an unmeasured tail; never silently resume.
		if _, err = tx.ExecContext(ctx, `UPDATE wireguard_nodes SET paused=TRUE,pause_reason='interrupted_runtime',updated_at=NOW() WHERE proxy_id=$1`, id); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO wireguard_traffic_totals(proxy_id,incomplete) VALUES($1,TRUE) ON CONFLICT(proxy_id) DO UPDATE SET incomplete=TRUE`, id); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE wireguard_traffic_runtimes SET closed=TRUE WHERE runtime_id=$1`, owner); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM wireguard_runtime_leases WHERE proxy_id=$1`, id); err != nil {
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		return wgtunnel.ErrBlocked
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO wireguard_traffic_runtimes(runtime_id,proxy_id,config_version) VALUES($1,$2,$3)`, runtimeID, id, version); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO wireguard_runtime_leases(proxy_id,runtime_id,expires_at) VALUES($1,$2,NOW()+INTERVAL '30 seconds')`, id, runtimeID); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *WireGuardStore) RenewRuntime(ctx context.Context, id, version int64, runtimeID string) error {
	result, err := s.DB.ExecContext(ctx, `UPDATE wireguard_runtime_leases l SET expires_at=NOW()+INTERVAL '30 seconds'
 FROM wireguard_nodes n,proxies p WHERE l.proxy_id=$1 AND l.runtime_id=$2 AND l.expires_at>NOW()
 AND n.proxy_id=l.proxy_id AND n.config_version=$3 AND NOT n.paused AND p.id=n.proxy_id AND p.status='active'
 AND p.deleted_at IS NULL AND (p.expires_at IS NULL OR p.expires_at>NOW())`, id, runtimeID, version)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return wgtunnel.ErrBlocked
	}
	return nil
}
