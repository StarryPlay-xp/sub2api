//go:build integration

package repository

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/egress"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
	_ "github.com/lib/pq"
)

func wireguardTestDB(t *testing.T) *sql.DB {
	return wireguardTestDBVersion(t, true)
}

func wireguardTestDBVersion(t *testing.T, upgraded bool) *sql.DB {
	t.Helper()
	dsn := os.Getenv("SUB2API_WG_TEST_DSN")
	if dsn == "" {
		t.Skip("SUB2API_WG_TEST_DSN is required")
	}
	db, e := sql.Open("postgres", dsn)
	if e != nil {
		t.Fatal(e)
	}
	var name string
	if e = db.QueryRow("SELECT current_database()").Scan(&name); e != nil {
		db.Close()
		t.Fatal(e)
	}
	if name != "wireguard_test" {
		db.Close()
		t.Fatal("refusing to run against a non-test database")
	}
	schema := fmt.Sprintf("wg_test_%d", time.Now().UnixNano())
	if _, e = db.Exec("CREATE SCHEMA " + schema); e != nil {
		db.Close()
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Exec("DROP SCHEMA " + schema + " CASCADE"); db.Close() })
	isolated, e := sql.Open("postgres", dsn+" search_path="+schema)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { isolated.Close() })
	_, e = isolated.Exec(`CREATE TABLE proxies(id BIGSERIAL PRIMARY KEY,name TEXT NOT NULL,protocol TEXT NOT NULL,host TEXT NOT NULL DEFAULT '',port INTEGER NOT NULL DEFAULT 0,expiry_warn_days INTEGER NOT NULL DEFAULT 7,updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),status TEXT NOT NULL DEFAULT 'active',deleted_at TIMESTAMPTZ,expires_at TIMESTAMPTZ,fallback_mode TEXT NOT NULL DEFAULT 'none',backup_proxy_id BIGINT,username TEXT,password TEXT);
        CREATE TABLE sub2api_plugin_bindings(id BIGSERIAL PRIMARY KEY,enabled BOOLEAN DEFAULT FALSE,rollout_percent INTEGER DEFAULT 100,capability TEXT);
        CREATE TABLE accounts(id BIGSERIAL PRIMARY KEY,platform TEXT DEFAULT 'openai',type TEXT DEFAULT 'apikey',extra JSONB,parent_account_id BIGINT,updated_at TIMESTAMPTZ DEFAULT NOW(),deleted_at TIMESTAMPTZ,proxy_id BIGINT REFERENCES proxies(id) ON DELETE SET NULL);`)
	if e != nil {
		t.Fatal(e)
	}
	migration, e := os.ReadFile(filepath.Join("..", "..", "migrations", "250_wireguard_egress.sql"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = isolated.Exec(string(migration)); e != nil {
		t.Fatal(e)
	}
	if _, e = isolated.Exec(`INSERT INTO proxies(id,name,protocol) VALUES(1,'test','wireguard'); INSERT INTO wireguard_nodes(proxy_id,config_ciphertext,public_summary) VALUES(1,'test-only','{}'); SELECT setval('proxies_id_seq',1);`); e != nil {
		t.Fatal(e)
	}
	peakMigration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "253_service_traffic_peaks.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = isolated.Exec(string(peakMigration)); err != nil {
		t.Fatal(err)
	}
	if upgraded {
		applyWireGuardLifecycleMigration(t, isolated)
	}
	return isolated
}

func applyWireGuardLifecycleMigration(t *testing.T, db *sql.DB) {
	t.Helper()
	migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "252_wireguard_deleted_account_guard.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
}

func TestWireGuardCheckpointIdempotencyAndRollover(t *testing.T) {
	db := wireguardTestDB(t)
	store := &WireGuardStore{DB: db}
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Minute)
	c := wgtunnel.Checkpoint{NodeID: 1, RuntimeID: "first", Version: 1, Seq: 1, At: at, TX: 100, RX: 200, Buckets: []wgtunnel.Bucket{{Start: at, TX: 100, RX: 200}}}
	for i := 0; i < 2; i++ {
		if e := store.SaveCheckpoint(ctx, c, false); e != nil {
			t.Fatal(e)
		}
	}
	c.Seq = 2
	c.TX = 150
	c.Buckets[0].TX = 150
	if e := store.SaveCheckpoint(ctx, c, false); e != nil {
		t.Fatal(e)
	}
	stale := c
	stale.Seq = 1
	stale.TX = 100
	stale.Buckets = nil
	if e := store.SaveCheckpoint(ctx, stale, false); e != nil {
		t.Fatal(e)
	}
	c.Seq = 3
	c.Closed = true
	if e := store.SaveCheckpoint(ctx, c, false); e != nil {
		t.Fatal(e)
	}
	c.RuntimeID = "second"
	c.Version = 2
	c.Seq = 1
	c.Closed = false
	c.TX = 10
	c.RX = 20
	c.Buckets = []wgtunnel.Bucket{{Start: at, TX: 10, RX: 20}}
	if e := store.SaveCheckpoint(ctx, c, false); e != nil {
		t.Fatal(e)
	}
	var tx, rx string
	if e := db.QueryRow(`SELECT tx_bytes::text,rx_bytes::text FROM wireguard_traffic_totals WHERE proxy_id=1`).Scan(&tx, &rx); e != nil {
		t.Fatal(e)
	}
	if tx != "160" || rx != "220" {
		t.Fatalf("incorrect lifetime counters: %s/%s", tx, rx)
	}
	if e := db.QueryRow(`SELECT tx_bytes::text,rx_bytes::text FROM wireguard_traffic_hourly WHERE proxy_id=1`).Scan(&tx, &rx); e != nil {
		t.Fatal(e)
	}
	if tx != "160" || rx != "220" {
		t.Fatal("hourly counters diverged")
	}
}

func TestWireGuardBindingCannotBeSilentlyCleared(t *testing.T) {
	db := wireguardTestDB(t)
	if _, e := db.Exec(`INSERT INTO accounts(id,proxy_id) VALUES(10,1)`); e != nil {
		t.Fatal(e)
	}
	for _, q := range []string{
		`UPDATE accounts SET proxy_id=NULL WHERE id=10`,
		`UPDATE proxies SET fallback_mode='direct' WHERE id=1`,
		`UPDATE proxies SET protocol='http' WHERE id=1`,
		`UPDATE proxies SET deleted_at=NOW() WHERE id=1`,
		`DELETE FROM proxies WHERE id=1`,
	} {
		if _, e := db.Exec(q); e == nil {
			t.Fatal("unsafe mutation accepted", q)
		}
	}
	tx, e := db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	if _, e = tx.Exec(`SELECT set_config('sub2api.explicit_egress_change','10',true)`); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(`UPDATE accounts SET proxy_id=NULL WHERE id=10`); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	var n int
	if e = db.QueryRow(`SELECT COUNT(*) FROM account_wireguard_bindings`).Scan(&n); e != nil || n != 0 {
		t.Fatal("explicit detach did not clear guard", e)
	}
}

func TestWireGuardNodeSecretsAndVersionChecks(t *testing.T) {
	db := wireguardTestDB(t)
	store := &WireGuardStore{DB: db}
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 32)))
	raw := "[Interface]\nPrivateKey = " + key + "\nAddress = 10.77.0.2/32\nDNS = 1.1.1.1\n[Peer]\nPublicKey = " + key + "\nEndpoint = test.invalid:51820\nAllowedIPs = 0.0.0.0/0\n"
	box, err := wgtunnel.NewSecretBox([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	id, err := store.CreateNode(ctx, wgtunnel.NodeInput{Name: "isolated", Config: raw}, box)
	if err != nil {
		t.Fatal(err)
	}
	n, err := store.LoadNode(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(n.Ciphertext, key) {
		t.Fatal("database contains plaintext configuration")
	}
	plain, err := box.Open(id, n.Ciphertext)
	if err != nil || plain != raw {
		t.Fatal("encrypted configuration cannot be restored")
	}
	view, err := store.ReadNode(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(view)
	if err != nil || strings.Contains(string(data), key) || strings.Contains(string(data), "private_key") {
		t.Fatal("public node view leaked a key")
	}
	if err = store.SetNodePaused(ctx, id, 1, true); err != nil {
		t.Fatal(err)
	}
	if err = store.ReplaceNode(ctx, id, 1, wgtunnel.NodeInput{Name: "reconfigured", Config: raw}, box); err != nil {
		t.Fatal(err)
	}
	n, err = store.LoadNode(ctx, id)
	if err != nil || n.Version != 2 || !n.Paused {
		t.Fatal("configuration replacement cleared failure latch")
	}
	if err = store.SetNodePaused(ctx, id, 1, false); err == nil {
		t.Fatal("stale resume accepted")
	}
	if err = store.ReplaceNode(ctx, id, 1, wgtunnel.NodeInput{Name: "stale", Config: raw}, box); err == nil {
		t.Fatal("stale configuration accepted")
	}
	if err = store.SetNodePaused(ctx, id, 2, false); err != nil {
		t.Fatal(err)
	}
}

func TestAuthoritativeRouteSurvivesMissingHydration(t *testing.T) {
	db := wireguardTestDB(t)
	ctx := context.Background()
	store := &WireGuardStore{DB: db}
	if _, err := db.Exec(`INSERT INTO accounts(id,proxy_id) VALUES(10,1),(11,NULL)`); err != nil {
		t.Fatal(err)
	}
	bound, err := store.ResolveReference(ctx, egress.Reference{AccountID: 10})
	if err != nil || bound.Kind != egress.WireGuard || bound.NodeID != 1 {
		t.Fatal("durable WG intent was not resolved")
	}
	direct, err := store.ResolveReference(ctx, egress.Reference{AccountID: 11})
	if err != nil || direct.Kind != egress.Direct {
		t.Fatal("explicit direct account was not resolved")
	}
	if _, err = store.ResolveReference(ctx, egress.Reference{}); err == nil {
		t.Fatal("ambiguous empty reference became direct")
	}
	if _, err = db.Exec(`UPDATE proxies SET status='inactive' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if _, err = store.ResolveReference(ctx, egress.Reference{AccountID: 10}); err == nil {
		t.Fatal("disabled WG binding became usable")
	}
	if _, err = db.Exec(`UPDATE proxies SET status='active' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SELECT set_config('sub2api.explicit_egress_change','10',true)`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE accounts SET proxy_id=NULL WHERE id=10`); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	detached, err := store.ResolveReference(ctx, egress.Reference{AccountID: 10})
	if err != nil || detached.Kind != egress.Direct || detached.Revision <= bound.Revision {
		t.Fatal("explicit detach did not invalidate account route generation")
	}
	if _, err = db.Exec(`UPDATE accounts SET proxy_id=1 WHERE id=10`); err != nil {
		t.Fatal(err)
	}
	rebound, err := store.ResolveReference(ctx, egress.Reference{AccountID: 10})
	if err != nil || rebound.Revision <= detached.Revision {
		t.Fatal("reattach reused old route generation")
	}
}

func TestWireGuardParentAndShadowChangeAtomically(t *testing.T) {
	db := wireguardTestDB(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,proxy_id) VALUES(10,1); INSERT INTO accounts(id,parent_account_id,proxy_id) VALUES(11,10,1)`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`SELECT set_config('sub2api.explicit_egress_change','11',true)`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE accounts SET proxy_id=NULL WHERE id=11`); err == nil {
		t.Fatal("shadow detached independently")
	}
	tx.Rollback()
	tx, err = db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`SELECT set_config('sub2api.explicit_egress_change','10',true)`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE accounts SET proxy_id=NULL WHERE id=10`); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM accounts WHERE id IN(10,11) AND proxy_id IS NULL`).Scan(&count); err != nil || count != 2 {
		t.Fatal("parent and shadow diverged inside transaction")
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM account_wireguard_bindings WHERE account_id IN(10,11)`).Scan(&count); err != nil || count != 2 {
		t.Fatal("rollback lost WG binding intent")
	}
	tx, err = db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SELECT set_config('sub2api.explicit_egress_accounts','["10"]',true)`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE accounts SET proxy_id=NULL WHERE id=10`); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM account_wireguard_bindings WHERE account_id IN(10,11)`).Scan(&count); err != nil || count != 0 {
		t.Fatal("explicit parent edit left stale shadow intent")
	}
}

func TestWireGuardLeaseExclusionAndCrashRecovery(t *testing.T) {
	db := wireguardTestDB(t)
	s := &WireGuardStore{DB: db}
	ctx := context.Background()
	if err := s.ClaimRuntime(ctx, 1, 1, "owner-a"); err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimRuntime(ctx, 1, 1, "owner-b"); err == nil {
		t.Fatal("two processes acquired one node")
	}
	if err := s.RenewRuntime(ctx, 1, 1, "owner-b"); err == nil {
		t.Fatal("foreign lease renewed")
	}
	if err := s.RenewRuntime(ctx, 1, 1, "owner-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE wireguard_runtime_leases SET expires_at=NOW()-INTERVAL '1 second'`); err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimRuntime(ctx, 1, 1, "owner-b"); err == nil {
		t.Fatal("crashed runtime resumed automatically")
	}
	var paused, incomplete bool
	if err := db.QueryRow(`SELECT n.paused,t.incomplete FROM wireguard_nodes n JOIN wireguard_traffic_totals t ON t.proxy_id=n.proxy_id WHERE n.proxy_id=1`).Scan(&paused, &incomplete); err != nil || !paused || !incomplete {
		t.Fatal("missing crash latch or incomplete marker", err)
	}
	if err := s.SetNodePaused(ctx, 1, 1, false); err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimRuntime(ctx, 1, 2, "owner-b"); err != nil {
		t.Fatal(err)
	}
	if err := s.RenewRuntime(ctx, 1, 1, "owner-a"); err == nil {
		t.Fatal("old owner resurrected")
	}
	if err := s.SaveCheckpoint(ctx, wgtunnel.Checkpoint{NodeID: 1, RuntimeID: "owner-b", Version: 2, Seq: 1, At: time.Now(), Closed: true}, false); err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimRuntime(ctx, 1, 2, "owner-c"); err != nil {
		t.Fatal("clean shutdown prevented takeover", err)
	}
}

func TestWireGuardRejectsExternalTransports(t *testing.T) {
	db := wireguardTestDB(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,platform,type,proxy_id,extra) VALUES(10,'anthropic','oauth',1,'{"custom_base_url_enabled":true}')`); err == nil {
		t.Fatal("external relay accepted WG binding")
	}
	if _, err := db.Exec(`INSERT INTO accounts(id,platform,type,proxy_id) VALUES(11,'openai','oauth',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sub2api_plugin_bindings(enabled,rollout_percent,capability) VALUES(TRUE,100,'openai.oauth.outbound_transport.v1')`); err == nil {
		t.Fatal("plugin enabled over WG binding")
	}
	if _, err := db.Exec(`DELETE FROM accounts WHERE id=11; INSERT INTO sub2api_plugin_bindings(enabled,rollout_percent,capability) VALUES(TRUE,100,'openai.oauth.outbound_transport.v1')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO accounts(id,platform,type,proxy_id) VALUES(12,'openai','oauth',1)`); err == nil {
		t.Fatal("WG binding bypassed enabled plugin")
	}
}

func TestEgressAttemptAliasesAndRetention(t *testing.T) {
	db := wireguardTestDB(t)
	store := &WireGuardStore{DB: db}
	ctx := context.Background()
	now := time.Now().UTC()
	for _, a := range []egress.Attempt{
		{ID: "fresh", RequestID: "raw", CorrelationIDs: []string{"client:raw", "local:local-id"}, RouteKey: "direct:test", StartedAt: now, FinishedAt: now, Snapshot: egress.Snapshot{Route: egress.Direct}, Outcome: "complete"},
		{ID: "old", RequestID: "raw", RouteKey: "direct:test", StartedAt: now.Add(-91 * 24 * time.Hour), FinishedAt: now.Add(-91 * 24 * time.Hour), Snapshot: egress.Snapshot{Route: egress.Direct}, Outcome: "complete"},
	} {
		if err := store.SaveAttempt(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	attempts, err := store.ReadAttempts(ctx, "client:raw")
	if err != nil || len(attempts) != 1 || attempts[0].ID != "fresh" {
		t.Fatal("alias lookup failed", err)
	}
	if err := store.PruneTrafficHistory(ctx, now); err != nil {
		t.Fatal(err)
	}
	attempts, err = store.ReadAttempts(ctx, "raw")
	if err != nil || len(attempts) != 1 || attempts[0].ID != "fresh" {
		t.Fatal("retention removed current evidence or retained old evidence", err)
	}
}

func TestEgressSummariesBatchAccountIsolation(t *testing.T) {
	store := &WireGuardStore{DB: wireguardTestDB(t)}
	ctx := context.Background()
	now := time.Now().UTC()
	for i, entry := range []struct {
		account   int64
		state, ip string
	}{
		{7, "observed", "203.0.113.9"},
		{7, "observed", "203.0.113.9"},
		{7, "unobserved", ""},
		{7, "expired", "203.0.113.10"},
		{8, "observed", "203.0.113.11"},
	} {
		a := egress.Attempt{ID: fmt.Sprint(i), RequestID: "raw", CorrelationIDs: []string{"local:alias"}, AccountID: entry.account, RouteKey: "direct:test", StartedAt: now, FinishedAt: now, Snapshot: egress.Snapshot{Evidence: wgtunnel.Evidence{State: entry.state, IP: entry.ip, ExpiresAt: now.Add(-time.Hour)}}, Outcome: "complete"}
		if err := store.SaveAttempt(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	result, err := store.ReadExitSummaries(ctx, []egress.AttemptQuery{
		{RequestID: "local:alias", AccountID: 7},
		{RequestID: "raw", AccountID: 8},
		{RequestID: "missing", AccountID: 7},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 3 || len(result[0].IPs) != 1 || result[0].IPs[0] != "203.0.113.9" || !result[0].Unobserved || !result[0].Expired {
		t.Fatalf("invalid alias/dedup/state summary: %+v", result)
	}
	if len(result[1].IPs) != 1 || result[1].IPs[0] != "203.0.113.11" || result[1].Unobserved || result[1].Expired {
		t.Fatalf("account snapshots were mixed: %+v", result[1])
	}
	if result[2].Recorded || len(result[2].IPs) != 0 {
		t.Fatalf("missing request was fabricated: %+v", result[2])
	}
}
