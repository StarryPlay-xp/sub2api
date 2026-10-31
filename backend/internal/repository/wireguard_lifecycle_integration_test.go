//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
	"github.com/stretchr/testify/require"
)

func TestWireGuardDeletedAccountUpgrade(t *testing.T) {
	db := wireguardTestDBVersion(t, false)
	// Existing installations may already have both deleted and active bindings.
	_, err := db.Exec(`INSERT INTO accounts(id,proxy_id) VALUES(77,1);
 INSERT INTO accounts(id,parent_account_id,proxy_id) VALUES(78,77,1);
 UPDATE accounts SET deleted_at=NOW() WHERE id=77`)
	require.NoError(t, err)
	applyWireGuardLifecycleMigration(t, db)
	_, err = db.Exec(`UPDATE proxies SET deleted_at=NOW() WHERE id=1`)
	require.ErrorContains(t, err, "WireGuard node is in use", "active shadow must still block deletion")
	_, err = db.Exec(`UPDATE accounts SET deleted_at=NOW() WHERE id=78`)
	require.NoError(t, err)
	store := &WireGuardStore{DB: db}
	at := time.Now().UTC().Truncate(time.Minute)
	c := wgtunnel.Checkpoint{NodeID: 1, RuntimeID: "deleted-account", Version: 1, Seq: 1, At: at, TX: 100, RX: 200, Closed: true, Buckets: []wgtunnel.Bucket{{Start: at, TX: 100, RX: 200}}}
	require.NoError(t, store.SaveCheckpoint(context.Background(), c, false))
	_, err = db.Exec(`UPDATE proxies SET deleted_at=NOW() WHERE id=1`)
	require.NoError(t, err)
	for _, id := range []int{77, 78} {
		_, err = db.Exec(`UPDATE accounts SET deleted_at=NULL WHERE id=$1`, id)
		require.ErrorContains(t, err, "Selected egress node is missing", "restore must not revive an unavailable route")
	}
	var bindings int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM account_wireguard_bindings`).Scan(&bindings))
	require.Equal(t, 2, bindings, "retain fail-closed binding intent")
	var tx, rx string
	require.NoError(t, db.QueryRow(`SELECT tx_bytes::text,rx_bytes::text FROM wireguard_traffic_totals WHERE proxy_id=1`).Scan(&tx, &rx))
	require.Equal(t, "100", tx)
	require.Equal(t, "200", rx)
	var buckets int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM wireguard_traffic_buckets`).Scan(&buckets))
	require.Equal(t, 1, buckets)
}

func TestWireGuardActiveAndExplicitUnbindingAfterUpgrade(t *testing.T) {
	db := wireguardTestDB(t)
	_, err := db.Exec(`INSERT INTO accounts(id,proxy_id) VALUES(77,1);
 INSERT INTO accounts(id,parent_account_id,proxy_id) VALUES(78,77,1)`)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE proxies SET deleted_at=NOW() WHERE id=1`)
	require.ErrorContains(t, err, "WireGuard node is in use")
	_, err = db.Exec(`UPDATE accounts SET proxy_id=NULL WHERE id=77`)
	require.ErrorContains(t, err, "explicit administrator")
	tx, err := db.Begin()
	require.NoError(t, err)
	defer tx.Rollback()
	_, err = tx.Exec(`SELECT set_config('sub2api.explicit_egress_accounts','["77"]',true);
 UPDATE accounts SET proxy_id=NULL WHERE id=77`)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	var bindings int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM account_wireguard_bindings`).Scan(&bindings))
	require.Zero(t, bindings)
	_, err = db.Exec(`UPDATE proxies SET deleted_at=NOW() WHERE id=1`)
	require.NoError(t, err)
}
