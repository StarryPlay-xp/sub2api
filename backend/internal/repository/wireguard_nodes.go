package repository

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
)

func (s *WireGuardStore) CreateNode(ctx context.Context, input wgtunnel.NodeInput, box *wgtunnel.SecretBox) (int64, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || len([]rune(name)) > 100 {
		return 0, errors.New("WireGuard node name must contain 1..100 characters")
	}
	cfg, err := wgtunnel.Parse(input.Config)
	if err != nil {
		return 0, err
	}
	summary, err := json.Marshal(cfg.Summary())
	if err != nil {
		return 0, err
	}
	host, port, _ := net.SplitHostPort(cfg.Endpoint)
	n, _ := strconv.Atoi(port)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var id int64
	err = tx.QueryRowContext(ctx, `INSERT INTO proxies(name,protocol,host,port,status,fallback_mode,expiry_warn_days,expires_at)
 VALUES($1,'wireguard',$2,$3,'active','none',7,$4) RETURNING id`, name, host, n, input.ExpiresAt).Scan(&id)
	if err != nil {
		return 0, err
	}
	encrypted, err := box.Seal(id, input.Config)
	if err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO wireguard_nodes(proxy_id,config_ciphertext,public_summary) VALUES($1,$2,$3)`, id, encrypted, string(summary))
	if err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// ReplaceNode compares the reviewed version under the same lock as the update.
// Replacing a configuration never automatically clears a paused state.
func (s *WireGuardStore) ReplaceNode(ctx context.Context, id, expectedVersion int64, input wgtunnel.NodeInput, box *wgtunnel.SecretBox) error {
	name := strings.TrimSpace(input.Name)
	if name == "" || len([]rune(name)) > 100 {
		return errors.New("WireGuard node name must contain 1..100 characters")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var version int64
	err = tx.QueryRowContext(ctx, `SELECT w.config_version FROM proxies p JOIN wireguard_nodes w ON w.proxy_id=p.id WHERE p.id=$1 AND p.deleted_at IS NULL AND p.protocol='wireguard' FOR UPDATE OF p,w`, id).Scan(&version)
	if err != nil {
		return err
	}
	if version != expectedVersion {
		return errors.New("WireGuard configuration changed; reload before saving")
	}
	if input.Config != "" {
		cfg, err := wgtunnel.Parse(input.Config)
		if err != nil {
			return err
		}
		encrypted, err := box.Seal(id, input.Config)
		if err != nil {
			return err
		}
		summary, err := json.Marshal(cfg.Summary())
		if err != nil {
			return err
		}
		host, port, _ := net.SplitHostPort(cfg.Endpoint)
		n, _ := strconv.Atoi(port)
		if _, err = tx.ExecContext(ctx, `UPDATE wireguard_nodes SET config_ciphertext=$2,public_summary=$3,config_version=config_version+1,updated_at=NOW() WHERE proxy_id=$1`, id, encrypted, string(summary)); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE proxies SET host=$2,port=$3 WHERE id=$1`, id, host, n); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE proxies SET name=$2,expires_at=$3,updated_at=NOW() WHERE id=$1`, id, name, input.ExpiresAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *WireGuardStore) ReadNode(ctx context.Context, id int64) (wgtunnel.NodeView, error) {
	var v wgtunnel.NodeView
	var summary []byte
	err := s.DB.QueryRowContext(ctx, `SELECT p.id,p.name,w.config_version,w.public_summary,p.status='active' AND (p.expires_at IS NULL OR p.expires_at>NOW()),w.paused,w.pause_reason,p.expires_at,
 COALESCE(t.tx_bytes,0)::text,COALESCE(t.rx_bytes,0)::text,t.saved_at,COALESCE(t.incomplete,false)
 FROM proxies p JOIN wireguard_nodes w ON w.proxy_id=p.id LEFT JOIN wireguard_traffic_totals t ON t.proxy_id=p.id
 WHERE p.id=$1 AND p.protocol='wireguard' AND p.deleted_at IS NULL`, id).Scan(&v.ID, &v.Name, &v.Version, &summary, &v.Active, &v.Paused, &v.PauseReason, &v.ExpiresAt, &v.TX, &v.RX, &v.SavedAt, &v.Incomplete)
	if err != nil {
		return v, err
	}
	if err = json.Unmarshal(summary, &v.Summary); err != nil {
		return v, errors.New("invalid stored WireGuard summary")
	}
	v.MetricBasis = wgtunnel.MetricBasis
	return v, nil
}

// Only explicit administrator resume may clear a failure latch. A stale failure
// from an older generation cannot pause a newly configured node.
func (s *WireGuardStore) SetNodePaused(ctx context.Context, id, version int64, paused bool) error {
	reason := ""
	if paused {
		reason = "route_failure"
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE wireguard_nodes SET paused=$3,pause_reason=$4,config_version=config_version+CASE WHEN $3 THEN 0 ELSE 1 END,updated_at=NOW() WHERE proxy_id=$1 AND config_version=$2`, id, version, paused, reason)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("WireGuard configuration changed")
	}
	return nil
}
