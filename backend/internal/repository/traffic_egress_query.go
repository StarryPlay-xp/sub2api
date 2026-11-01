package repository

import (
	"context"
	"database/sql"
	"errors"
	"math/big"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

func egressAggregateColumns() string {
	cols := []string{"SUM(upstream_out)::text", "SUM(upstream_in)::text", "SUM(attempts)"}
	for _, key := range []string{"out", "in", "total"} {
		col := "peak_" + key
		cols = append(cols, "MAX("+col+")", "(array_agg("+col+"_at ORDER BY "+col+" DESC NULLS LAST,"+col+"_at))[1]", "(array_agg("+col+"_window ORDER BY "+col+" DESC NULLS LAST,"+col+"_at))[1]")
	}
	return strings.Join(cols, ",")
}
func scanEgressAggregate(rows *sql.Rows, prefix []any) (traffic.EgressBytes, uint64, traffic.Peaks, error) {
	var b traffic.EgressBytes
	var attempts uint64
	var rates, seconds [3]sql.NullFloat64
	var at [3]sql.NullTime
	args := append(prefix, &b.Out, &b.In, &attempts)
	for i := range rates {
		args = append(args, &rates[i], &at[i], &seconds[i])
	}
	if err := rows.Scan(args...); err != nil {
		return b, 0, nil, err
	}
	var peaks traffic.Peaks
	for i, key := range []string{"out", "in", "total"} {
		if rates[i].Valid && at[i].Valid && seconds[i].Valid {
			if peaks == nil {
				peaks = traffic.Peaks{}
			}
			peaks[key] = traffic.Peak{BytesPerSecond: rates[i].Float64, At: at[i].Time, WindowSeconds: seconds[i].Float64}
		}
	}
	return b, attempts, peaks, nil
}
func readEgressRows(ctx context.Context, db trafficSQLReader, w wgtunnel.TrafficWindow, concurrent bool) ([]service.TrafficEgressRow, error) {
	rows, err := db.QueryContext(ctx, `SELECT route_type,route_id,account_id,`+egressAggregateColumns()+` FROM service_traffic_egress_buckets WHERE bucket_start >= $1 AND bucket_start < $2 GROUP BY route_type,route_id,account_id HAVING SUM(upstream_out+upstream_in)>0 OR SUM(attempts)>0`, w.Start, w.End)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := []service.TrafficEgressRow{}
	for rows.Next() {
		var id traffic.EgressIdentity
		b, n, p, e := scanEgressAggregate(rows, []any{&id.Type, &id.ID, &id.AccountID})
		if e != nil {
			return nil, e
		}
		if concurrent {
			p = nil
		}
		result = append(result, service.TrafficEgressRow{EgressIdentity: id, Bytes: b, Attempts: n, Peaks: p})
	}
	return result, rows.Err()
}
func egressWindowMetadata(ctx context.Context, db trafficSQLReader, w wgtunnel.TrafficWindow) (service.TrafficEgressWindow, error) {
	result := service.TrafficEgressWindow{TrafficWindow: w, MetricBasis: traffic.MetricBasis}
	if err := db.QueryRowContext(ctx, `SELECT (SELECT started_at FROM service_traffic_egress_metadata WHERE id=TRUE),COUNT(*)>0,COALESCE(BOOL_OR(overflow),FALSE),COALESCE(BOOL_OR(invalid_samples>0),FALSE) FROM service_traffic_egress_buckets WHERE bucket_start >= $1 AND bucket_start < $2`, w.Start, w.End).Scan(&result.EgressStartedAt, &result.HasRecords, &result.Overflow, &result.Incomplete); err != nil {
		return result, err
	}
	rows, err := db.QueryContext(ctx, `SELECT started_at,saved_at,closed,incomplete FROM service_traffic_runtimes WHERE started_at < $2 AND saved_at > $1 ORDER BY started_at`, w.Start, w.End)
	if err != nil {
		return result, err
	}
	defer func() { _ = rows.Close() }()
	var latestEnd time.Time
	for rows.Next() {
		var start, end time.Time
		var closed, incomplete bool
		if err = rows.Scan(&start, &end, &closed, &incomplete); err != nil {
			return result, err
		}
		if start.Before(latestEnd) {
			result.ConcurrentRuntimes = true
		}
		if end.After(latestEnd) {
			latestEnd = end
		}
		if incomplete || (!closed && end.Before(w.AsOf.Add(-15*time.Second))) {
			result.Incomplete = true
		}
	}
	if result.ConcurrentRuntimes || result.Overflow || result.EgressStartedAt == nil || w.Start.Before(*result.EgressStartedAt) {
		result.Incomplete = true
	}
	return result, rows.Err()
}
func (s *ServiceTrafficStore) EgressData(ctx context.Context, q service.TrafficEgressQuery) (service.TrafficEgressData, error) {
	result := service.TrafficEgressData{Routes: map[traffic.EgressIdentity]service.TrafficRouteIdentity{}, Accounts: map[int64]service.TrafficEgressAccount{}, WireGuard: map[int64]service.TrafficEgressWG{}, History: []service.TrafficEgressHistory{}}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	result.Window, err = egressWindowMetadata(ctx, tx, q.Window)
	if err != nil {
		return result, err
	}
	result.Current, err = readEgressRows(ctx, tx, q.Window, result.Window.ConcurrentRuntimes)
	if err != nil {
		return result, err
	}
	if q.Compare {
		previous := q.Window
		previous.End = q.Window.Start
		end := q.Window.End.UTC().Truncate(time.Hour)
		if end.Before(q.Window.End) {
			end = end.Add(time.Hour)
		}
		previous.Start = previous.End.Add(-end.Sub(q.Window.Start))
		meta, e := egressWindowMetadata(ctx, tx, previous)
		if e != nil {
			return result, e
		}
		result.PreviousWindow = &service.TrafficPreviousWindow{Start: previous.Start, End: previous.End, Complete: !meta.Incomplete && !previous.Start.Before(q.Window.RetainedFrom) && meta.EgressStartedAt != nil && !previous.Start.Before(*meta.EgressStartedAt)}
		if previous.Start.Before(q.Window.RetainedFrom) {
			previous.Start = q.Window.RetainedFrom
		}
		result.Previous, err = readEgressRows(ctx, tx, previous, meta.ConcurrentRuntimes)
		if err != nil {
			return result, err
		}
	}
	if err = readEgressIdentities(ctx, tx, q, &result); err != nil {
		return result, err
	}
	if err = readEgressWG(ctx, tx, q.Window, &result); err != nil {
		return result, err
	}
	if q.Detail {
		result.History, err = readEgressHistory(ctx, tx, q, result.Window.ConcurrentRuntimes)
		if err != nil {
			return result, err
		}
	}
	return result, tx.Commit()
}
func readEgressIdentities(ctx context.Context, db trafficSQLReader, q service.TrafficEgressQuery, d *service.TrafficEgressData) error {
	observed := map[traffic.EgressIdentity]bool{}
	accountIDs := map[int64]bool{}
	for _, rows := range [][]service.TrafficEgressRow{d.Current, d.Previous} {
		for _, r := range rows {
			if r.Type == "unidentified" {
				continue
			}
			observed[traffic.EgressIdentity{Type: r.Type, ID: r.ID}] = true
			if r.AccountID > 0 {
				accountIDs[r.AccountID] = true
			}
		}
	}
	requested := traffic.EgressIdentity{Type: q.RouteType, ID: q.RouteID}
	if q.Detail && !observed[requested] {
		var exists bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM service_traffic_egress_buckets WHERE route_type=$1 AND route_id=$2)`, q.RouteType, q.RouteID).Scan(&exists); err != nil {
			return err
		}
		observed[requested] = exists
	}
	proxyIDs := map[int64]bool{}
	for key, history := range observed {
		if key.Type == "direct" {
			d.Routes[key] = service.TrafficRouteIdentity{RouteType: "direct"}
			continue
		}
		proxyIDs[key.ID] = true
		if history {
			id := key.ID
			d.Routes[key] = service.TrafficRouteIdentity{RouteType: key.Type, RouteID: &id, RouteDeleted: true}
		}
	}
	if len(proxyIDs) > 0 {
		ids := make([]int64, 0, len(proxyIDs))
		for id := range proxyIDs {
			ids = append(ids, id)
		}
		rows, err := db.QueryContext(ctx, `SELECT id,name,protocol,deleted_at IS NOT NULL FROM proxies WHERE id=ANY($1)`, pq.Array(ids))
		if err != nil {
			return err
		}
		for rows.Next() {
			var id int64
			var name, protocol string
			var deleted bool
			if err = rows.Scan(&id, &name, &protocol, &deleted); err != nil {
				_ = rows.Close()
				return err
			}
			for key, history := range observed {
				if key.ID == id && (history || (key.Type == "wireguard" && protocol == "wireguard") || (key.Type == "proxy" && protocol != "wireguard")) {
					copyID := id
					copyName, copyProtocol := name, protocol
					d.Routes[key] = service.TrafficRouteIdentity{RouteType: key.Type, RouteID: &copyID, RouteName: &copyName, Protocol: &copyProtocol, RouteDeleted: deleted}
				}
			}
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return err
		}
	}
	if len(accountIDs) > 0 {
		ids := make([]int64, 0, len(accountIDs))
		for id := range accountIDs {
			ids = append(ids, id)
		}
		rows, err := db.QueryContext(ctx, `SELECT id,name,platform,type,deleted_at IS NOT NULL FROM accounts WHERE id=ANY($1)`, pq.Array(ids))
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var a service.TrafficEgressAccount
			if err = rows.Scan(&a.AccountID, &a.AccountName, &a.Platform, &a.AccountType, &a.AccountDeleted); err != nil {
				return err
			}
			d.Accounts[a.AccountID] = a
		}
		if err = rows.Err(); err != nil {
			return err
		}
	}
	return nil
}
func readEgressWG(ctx context.Context, db trafficSQLReader, w wgtunnel.TrafficWindow, d *service.TrafficEgressData) error {
	ids := []int64{}
	for key := range d.Routes {
		if key.Type == "wireguard" {
			ids = append(ids, key.ID)
			d.WireGuard[key.ID] = service.TrafficEgressWG{TX: "0", RX: "0", Total: "0"}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := db.QueryContext(ctx, `SELECT proxy_id,SUM(tx_bytes)::text,SUM(rx_bytes)::text FROM wireguard_traffic_hourly WHERE proxy_id=ANY($1) AND bucket_start >= $2 AND bucket_start < $3 GROUP BY proxy_id`, pq.Array(ids), w.Start, w.End)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var wg service.TrafficEgressWG
		if err = rows.Scan(&id, &wg.TX, &wg.RX); err != nil {
			return err
		}
		tx, okTX := new(big.Int).SetString(wg.TX, 10)
		rx, okRX := new(big.Int).SetString(wg.RX, 10)
		if !okTX || !okRX {
			return errors.New("invalid WireGuard aggregate bytes")
		}
		wg.Total = new(big.Int).Add(tx, rx).String()
		wg.HasRecords = true
		d.WireGuard[id] = wg
	}
	return rows.Err()
}
func readEgressHistory(ctx context.Context, db trafficSQLReader, q service.TrafficEgressQuery, concurrent bool) ([]service.TrafficEgressHistory, error) {
	rows, err := db.QueryContext(ctx, `SELECT bucket_start,`+egressAggregateColumns()+` FROM service_traffic_egress_buckets WHERE bucket_start >= $1 AND bucket_start < $2 AND route_type=$3 AND route_id=$4 AND account_id=0 GROUP BY bucket_start ORDER BY bucket_start`, q.Window.Start, q.Window.End, q.RouteType, q.RouteID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := []service.TrafficEgressHistory{}
	for rows.Next() {
		var start time.Time
		b, n, p, e := scanEgressAggregate(rows, []any{&start})
		if e != nil {
			return nil, e
		}
		if concurrent {
			p = nil
		}
		result = append(result, service.TrafficEgressHistory{Start: start, Interval: "hour", Bytes: b, Attempts: n, Peaks: p})
	}
	return result, rows.Err()
}
