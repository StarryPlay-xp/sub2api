package repository

import (
	"context"
	"database/sql"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/egress"
)

// ResolveReference loads routing intent from its owner instead of trusting an
// account's optional hydrated Proxy object or a scheduler cache snapshot.
func (s *WireGuardStore) ResolveReference(ctx context.Context, ref egress.Reference) (egress.Record, error) {
	if !ref.Valid() {
		return egress.Record{}, egress.ErrBlocked
	}
	if ref.ExplicitDirect {
		return egress.Record{Kind: egress.Direct}, nil
	}
	var record egress.Record
	var selected, guard sql.NullInt64
	var protocol, host, status, user, password sql.NullString
	var port sql.NullInt64
	var expires, deleted sql.NullTime
	var err error
	if ref.AccountID > 0 {
		record.AccountID = ref.AccountID
		err = s.DB.QueryRowContext(ctx, `SELECT a.proxy_id,g.proxy_id,COALESCE(r.revision,0),COALESCE(a.parent_account_id,0),p.protocol,p.host,p.port,p.status,p.username,p.password,p.expires_at,p.deleted_at
   FROM accounts a LEFT JOIN account_wireguard_bindings g ON g.account_id=a.id
   LEFT JOIN account_egress_revisions r ON r.account_id=a.id LEFT JOIN proxies p ON p.id=a.proxy_id
   WHERE a.id=$1 AND a.deleted_at IS NULL`, ref.AccountID).Scan(&selected, &guard, &record.Revision, &record.ParentAccountID, &protocol, &host, &port, &status, &user, &password, &expires, &deleted)
	} else {
		selected = sql.NullInt64{Int64: ref.NodeID, Valid: true}
		err = s.DB.QueryRowContext(ctx, `SELECT protocol,host,port,status,username,password,expires_at,deleted_at FROM proxies WHERE id=$1`, ref.NodeID).Scan(&protocol, &host, &port, &status, &user, &password, &expires, &deleted)
	}
	if err != nil {
		return record, egress.ErrBlocked
	}
	if guard.Valid && (!selected.Valid || selected.Int64 != guard.Int64 || protocol.String != "wireguard") {
		return record, egress.ErrBlocked
	}
	if !selected.Valid {
		record.Kind = egress.Direct
		return record, nil
	}
	if !protocol.Valid || deleted.Valid || status.String != "active" || (expires.Valid && !expires.Time.After(time.Now())) {
		return record, egress.ErrBlocked
	}
	record.NodeID = selected.Int64
	if protocol.String == "wireguard" {
		if ref.AccountID > 0 && !guard.Valid {
			return record, egress.ErrBlocked
		}
		record.Kind = egress.WireGuard
		return record, nil
	}
	record.Kind = egress.Proxy
	value := url.URL{Scheme: protocol.String, Host: net.JoinHostPort(host.String, strconv.FormatInt(port.Int64, 10))}
	if user.Valid && user.String != "" {
		value.User = url.UserPassword(user.String, password.String)
	}
	record.ProxyURL = value.String()
	return record, nil
}
