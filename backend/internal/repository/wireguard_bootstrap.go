package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
)

const wireGuardBootstrapKey = "deployment.wireguard.bootstrap.v1"

// A private, deployment-owned one-time plan. Never include it in logs, API
// responses, the image or the repository. Explicit IDs freeze the authorized
// account set; later accounts and administrator changes are never rebound.
type wireGuardBootstrapPlan struct {
	Name       string  `json:"name"`
	AccountIDs []int64 `json:"account_ids"`
	Config     string  `json:"config"`
}

func (wireGuardBootstrapPlan) String() string   { return "[WireGuard bootstrap plan redacted]" }
func (wireGuardBootstrapPlan) GoString() string { return "[WireGuard bootstrap plan redacted]" }

func readWireGuardBootstrapPlan(path string) (wireGuardBootstrapPlan, error) {
	var plan wireGuardBootstrapPlan
	f, err := os.Open(path)
	if err != nil {
		return plan, errors.New("WG bootstrap plan cannot be opened")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > wgtunnel.MaxConfigBytes+4096 {
		return plan, errors.New("WG bootstrap plan must be a small owner-only regular file")
	}
	decoder := json.NewDecoder(io.LimitReader(f, wgtunnel.MaxConfigBytes+4097))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&plan) != nil {
		return wireGuardBootstrapPlan{}, errors.New("Invalid WG bootstrap plan")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return wireGuardBootstrapPlan{}, errors.New("Invalid WG bootstrap plan")
	}
	plan.Name = strings.TrimSpace(plan.Name)
	if plan.Name == "" || len([]rune(plan.Name)) > 100 || len(plan.AccountIDs) == 0 || len(plan.AccountIDs) > 100 {
		return wireGuardBootstrapPlan{}, errors.New("WG bootstrap requires a name and explicit account IDs")
	}
	sort.Slice(plan.AccountIDs, func(i, j int) bool { return plan.AccountIDs[i] < plan.AccountIDs[j] })
	for i, id := range plan.AccountIDs {
		if id <= 0 || (i > 0 && plan.AccountIDs[i-1] == id) {
			return wireGuardBootstrapPlan{}, errors.New("Invalid WG bootstrap account IDs")
		}
	}
	return plan, nil
}

// BootstrapWireGuard executes before schedulers or HTTP services are created.
// Node creation, all account bindings and the completion marker share one
// transaction. A validation/trigger failure leaves no half-applied deployment.
func (s *WireGuardStore) BootstrapWireGuard(ctx context.Context, path string, box *wgtunnel.SecretBox) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return errors.New("WG bootstrap database unavailable")
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(7267426371251)`); err != nil {
		return errors.New("WG bootstrap lock failed")
	}
	var existing string
	err = tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=$1`, wireGuardBootstrapKey).Scan(&existing)
	if err == nil {
		if box == nil {
			return errors.New("Completed WG bootstrap still requires its stable encryption key")
		}
		return nil
	} // Successful first deployment is never replayed.
	if !errors.Is(err, sql.ErrNoRows) {
		return errors.New("WG bootstrap state unavailable")
	}
	if box == nil {
		return errors.New("WG bootstrap requires the stable encryption key")
	}
	plan, err := readWireGuardBootstrapPlan(path)
	if err != nil {
		return err
	}
	defer func() { plan.Config = "" }()
	cfg, err := wgtunnel.Parse(plan.Config)
	if err != nil {
		return errors.New("WG bootstrap configuration is not supported")
	}
	// Stabilize the first-deployment account set, including inserts by another
	// application instance. Existing non-empty egress is never silently replaced.
	if _, err = tx.ExecContext(ctx, `LOCK TABLE accounts IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return errors.New("WG bootstrap account lock failed")
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,proxy_id,parent_account_id FROM accounts WHERE deleted_at IS NULL ORDER BY id FOR UPDATE`)
	if err != nil {
		return errors.New("WG bootstrap account inspection failed")
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		var proxy, parent sql.NullInt64
		if err = rows.Scan(&id, &proxy, &parent); err != nil {
			break
		}
		if proxy.Valid || parent.Valid {
			err = errors.New("existing egress or shadow account")
			break
		}
		ids = append(ids, id)
	}
	rowErr := rows.Err()
	rows.Close()
	if err != nil || rowErr != nil || len(ids) != len(plan.AccountIDs) {
		return errors.New("WG bootstrap account set or existing egress changed; review deployment plan")
	}
	for i, id := range ids {
		if id != plan.AccountIDs[i] {
			return errors.New("WG bootstrap account set changed; review deployment plan")
		}
	}
	var duplicate bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM proxies WHERE name=$1 AND deleted_at IS NULL)`, plan.Name).Scan(&duplicate); err != nil || duplicate {
		return errors.New("WG bootstrap node name is already used or cannot be checked")
	}
	host, port, _ := net.SplitHostPort(cfg.Endpoint)
	n, _ := strconv.Atoi(port)
	var id int64
	if err = tx.QueryRowContext(ctx, `INSERT INTO proxies(name,protocol,host,port,status,fallback_mode,expiry_warn_days) VALUES($1,'wireguard',$2,$3,'active','none',7) RETURNING id`, plan.Name, host, n).Scan(&id); err != nil {
		return errors.New("WG bootstrap node creation failed")
	}
	cipher, err := box.Seal(id, plan.Config)
	if err != nil {
		return errors.New("WG bootstrap encryption failed")
	}
	summary, _ := json.Marshal(cfg.Summary())
	if _, err = tx.ExecContext(ctx, `INSERT INTO wireguard_nodes(proxy_id,config_ciphertext,public_summary) VALUES($1,$2,$3)`, id, cipher, string(summary)); err != nil {
		return errors.New("WG bootstrap configuration save failed")
	}
	for _, accountID := range ids {
		if _, err = tx.ExecContext(ctx, `SELECT set_config('sub2api.explicit_egress_change',$1,true)`, strconv.FormatInt(accountID, 10)); err != nil {
			return errors.New("WG bootstrap binding authorization failed")
		}
		result, err := tx.ExecContext(ctx, `UPDATE accounts SET proxy_id=$1,updated_at=NOW() WHERE id=$2 AND deleted_at IS NULL AND proxy_id IS NULL`, id, accountID)
		if err != nil {
			return errors.New("WG bootstrap binding rejected; check account transport compatibility")
		}
		count, err := result.RowsAffected()
		if err != nil || count != 1 {
			return errors.New("WG bootstrap account changed")
		}
	}
	marker, _ := json.Marshal(struct {
		NodeID     int64   `json:"node_id"`
		AccountIDs []int64 `json:"account_ids"`
	}{id, ids})
	if _, err = tx.ExecContext(ctx, `INSERT INTO settings(key,value,updated_at) VALUES($1,$2,NOW())`, wireGuardBootstrapKey, string(marker)); err != nil {
		return errors.New("WG bootstrap completion could not be saved")
	}
	if err = tx.Commit(); err != nil {
		return errors.New("WG bootstrap commit failed; inspect deployment state before retrying")
	}
	return nil
}
