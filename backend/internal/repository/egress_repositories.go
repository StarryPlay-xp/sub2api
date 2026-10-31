package repository

import (
	"database/sql"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/egress"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func NewRoutedAccountRepository(client *dbent.Client, db *sql.DB, cache service.SchedulerCache, router *egress.Router) service.AccountRepository {
	r := newAccountRepositoryWithSQL(client, db, cache)
	r.egressRouter = router
	return r
}
func NewRoutedAdminAccountRepository(client *dbent.Client, db *sql.DB, cache service.SchedulerCache, router *egress.Router) service.AdminAccountRepository {
	r := newAccountRepositoryWithSQL(client, db, cache)
	r.egressRouter = router
	return r
}
func NewRoutedProxyRepository(client *dbent.Client, db *sql.DB, router *egress.Router) service.ProxyRepository {
	r := newProxyRepositoryWithSQL(client, db)
	r.egressRouter = router
	return r
}
func (r *accountRepository) EgressRouter() *egress.Router { return r.egressRouter }
func (r *proxyRepository) EgressRouter() *egress.Router   { return r.egressRouter }
