//go:build integration

package repository

import (
	"context"
	"database/sql"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// v0.3.0 contains exactly the candidate's migration set except 253–255. Build that
// schema first, then verify the additive upgrade against administrator-edited
// state, not only an empty database or an already-upgraded schema.
func TestServiceTrafficUpgradePreservesConfiguredState(t *testing.T) {
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, selectDockerImage(ctx, postgresImageTag),
		tcpostgres.WithDatabase("traffic_upgrade_test"), tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"), tcpostgres.BasicWaitStrategies())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(ctx)) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	previous := fstest.MapFS{}
	names, err := fs.Glob(migrations.FS, "*.sql")
	require.NoError(t, err)
	for _, name := range names {
		if name == "253_service_traffic_peaks.sql" || name == "254_service_traffic_subjects.sql" || name == "255_service_traffic_egress.sql" {
			continue
		}
		data, readErr := migrations.FS.ReadFile(name)
		require.NoError(t, readErr)
		previous[name] = &fstest.MapFile{Data: data}
	}
	require.NoError(t, applyMigrationsFS(ctx, db, previous))
	box, err := wgtunnel.NewSecretBox(make([]byte, 32))
	require.NoError(t, err)
	ciphertext, err := box.Seal(1, "synthetic administrator-replaced peer configuration")
	require.NoError(t, err)
	_, err = db.Exec(`
INSERT INTO users(id,email,password_hash) VALUES(1,'upgrade@example.invalid','synthetic');
INSERT INTO api_keys(user_id,key,name) VALUES(1,'synthetic-upgrade-key','retained key');
INSERT INTO proxies(id,name,protocol,host,port) VALUES
 (1,'administrator renamed node','wireguard','',0),(2,'ordinary proxy','http','192.0.2.2',8080);
INSERT INTO wireguard_nodes(proxy_id,config_ciphertext,config_version,public_summary)
 VALUES(1,'temporary synthetic value',4,'{}');
INSERT INTO accounts(id,name,platform,type,proxy_id) VALUES
 (1,'bound account','openai','apikey',1),(2,'ordinary account','openai','apikey',2);
UPDATE wireguard_nodes SET paused=TRUE,pause_reason='administrator' WHERE proxy_id=1;
INSERT INTO settings(key,value) VALUES('deployment.wireguard.bootstrap.v1','{"node_id":1,"account_ids":[99]}');
INSERT INTO wireguard_traffic_totals(proxy_id,tx_bytes,rx_bytes) VALUES(1,123,456);
INSERT INTO wireguard_traffic_hourly(proxy_id,bucket_start,tx_bytes,rx_bytes) VALUES(1,date_trunc('hour',NOW()),123,456);`)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE wireguard_nodes SET config_ciphertext=$1 WHERE proxy_id=1`, ciphertext)
	require.NoError(t, err)

	const snapshotSQL = `SELECT jsonb_build_object(
 'users',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM users t),
 'keys',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM api_keys t),
 'accounts',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM accounts t),
 'proxies',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM proxies t),
 'nodes',(SELECT jsonb_agg(to_jsonb(t) ORDER BY proxy_id) FROM wireguard_nodes t),
 'bindings',(SELECT jsonb_agg(to_jsonb(t) ORDER BY account_id) FROM account_wireguard_bindings t),
 'totals',(SELECT jsonb_agg(to_jsonb(t) ORDER BY proxy_id) FROM wireguard_traffic_totals t),
 'bootstrap',(SELECT value FROM settings WHERE key='deployment.wireguard.bootstrap.v1'))::text`
	var before string
	require.NoError(t, db.QueryRow(snapshotSQL).Scan(&before))
	for i := 0; i < 2; i++ {
		require.NoError(t, ApplyMigrations(ctx, db))
		// A stale/missing import plan must never replay after completion.
		require.NoError(t, (&WireGuardStore{DB: db}).BootstrapWireGuard(ctx, "/missing/stale-plan", box))
		var after, retainedCiphertext string
		require.NoError(t, db.QueryRow(snapshotSQL).Scan(&after))
		require.JSONEq(t, before, after)
		require.NoError(t, db.QueryRow(`SELECT config_ciphertext FROM wireguard_nodes WHERE proxy_id=1`).Scan(&retainedCiphertext))
		plain, openErr := box.Open(1, retainedCiphertext)
		require.NoError(t, openErr)
		require.Equal(t, "synthetic administrator-replaced peer configuration", plain)
	}
	var tx, rx string
	var unknownPeak bool
	require.NoError(t, db.QueryRow(`SELECT tx_bytes::text,rx_bytes::text,peak_data IS NULL FROM wireguard_traffic_hourly WHERE proxy_id=1`).Scan(&tx, &rx, &unknownPeak))
	require.Equal(t, "123", tx)
	require.Equal(t, "456", rx)
	require.True(t, unknownPeak)
}
