//go:build integration

package repository

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestWireGuardRepositoryBulkAuthorizationScope(t *testing.T) {
	db := wireguardTestDB(t)
	_, err := db.Exec(`ALTER TABLE accounts ADD COLUMN credentials JSONB;
 CREATE TABLE scheduler_outbox(event_type TEXT,account_id BIGINT,group_id BIGINT,payload JSONB,dedup_key TEXT);
 CREATE UNIQUE INDEX outbox_dedup ON scheduler_outbox(dedup_key) WHERE dedup_key IS NOT NULL;
 INSERT INTO accounts(id,proxy_id) VALUES(77,1),(79,1);
 INSERT INTO accounts(id,parent_account_id,proxy_id) VALUES(78,77,1);
 INSERT INTO proxies(id,name,protocol) VALUES(2,'second','wireguard');
 INSERT INTO wireguard_nodes(proxy_id,config_ciphertext,public_summary) VALUES(2,'test-only','{}');`)
	require.NoError(t, err)
	repo := &accountRepository{client: dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))}
	direct, other := int64(0), int64(2)
	updates := service.AccountBulkUpdate{ProxyID: &direct}
	_, err = repo.BulkUpdate(context.Background(), []int64{77}, updates)
	require.ErrorContains(t, err, "explicit administrator")
	// A filtered capability alone is not a wildcard at the persistence boundary.
	_, err = repo.BulkUpdate(service.WithExplicitFilteredEgressEdit(context.Background()), []int64{77}, updates)
	require.ErrorContains(t, err, "explicit administrator")
	ctx := service.WithExplicitEgressEdit(context.Background(), 77)
	_, err = repo.BulkUpdate(ctx, []int64{77, 79}, updates)
	require.ErrorContains(t, err, "explicit administrator", "authorization must not leak to an additional target")
	var proxy int64
	require.NoError(t, db.QueryRow(`SELECT proxy_id FROM accounts WHERE id=77`).Scan(&proxy))
	require.Equal(t, int64(1), proxy, "failed batch must roll back")
	_, err = repo.BulkUpdate(ctx, []int64{77}, service.AccountBulkUpdate{ProxyID: &other})
	require.NoError(t, err)
	require.NoError(t, db.QueryRow(`SELECT proxy_id FROM accounts WHERE id=78`).Scan(&proxy))
	require.Equal(t, other, proxy, "parent update propagates atomically")
	_, err = repo.BulkUpdate(service.WithExplicitEgressEdit(context.Background(), 78), []int64{78}, updates)
	require.ErrorContains(t, err, "shadow binding must follow its parent")
	_, err = repo.BulkUpdate(ctx, []int64{77}, updates)
	require.NoError(t, err)
	_, err = repo.BulkUpdate(context.Background(), []int64{79}, updates)
	require.ErrorContains(t, err, "explicit administrator", "transaction-local grant must not leak")
}
