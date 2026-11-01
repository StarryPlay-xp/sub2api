package service

import (
	"context"
	"errors"
	"math/big"
	"sort"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
)

var ErrTrafficRouteNotFound = errors.New("traffic route not found")

type TrafficEgressWindow struct {
	wgtunnel.TrafficWindow
	MetricBasis        string     `json:"metric_basis"`
	EgressStartedAt    *time.Time `json:"egress_started_at"`
	HasRecords         bool       `json:"has_records"`
	Incomplete         bool       `json:"incomplete"`
	ConcurrentRuntimes bool       `json:"concurrent_runtimes"`
	Overflow           bool       `json:"overflow"`
}
type TrafficRouteIdentity struct {
	RouteType    string  `json:"route_type"`
	RouteID      *int64  `json:"route_id"`
	RouteName    *string `json:"route_name"`
	Protocol     *string `json:"protocol"`
	RouteDeleted bool    `json:"route_deleted"`
}
type TrafficEgressPrevious struct {
	TotalBytes string `json:"total_bytes"`
}
type TrafficEgressWG struct {
	TX         string `json:"tx_bytes"`
	RX         string `json:"rx_bytes"`
	Total      string `json:"total_bytes"`
	HasRecords bool   `json:"has_records"`
}
type TrafficRouteStats struct {
	TrafficRouteIdentity
	Bytes        traffic.EgressBytes    `json:"bytes"`
	TotalBytes   string                 `json:"total_bytes"`
	Share        *float64               `json:"share"`
	Attempts     uint64                 `json:"attempts"`
	Peaks        traffic.Peaks          `json:"peaks"`
	AccountCount int                    `json:"account_count"`
	WireGuard    *TrafficEgressWG       `json:"wireguard"`
	Previous     *TrafficEgressPrevious `json:"previous"`
	GrowthBytes  *string                `json:"growth_bytes"`
}
type TrafficEgressAccount struct {
	AccountID      int64                  `json:"account_id"`
	AccountName    string                 `json:"account_name"`
	Platform       string                 `json:"platform"`
	AccountType    string                 `json:"account_type"`
	AccountDeleted bool                   `json:"account_deleted"`
	Bytes          traffic.EgressBytes    `json:"bytes"`
	TotalBytes     string                 `json:"total_bytes"`
	Share          *float64               `json:"share"`
	Attempts       uint64                 `json:"attempts"`
	Peaks          traffic.Peaks          `json:"peaks"`
	Previous       *TrafficEgressPrevious `json:"previous"`
	GrowthBytes    *string                `json:"growth_bytes"`
}
type TrafficEgressTotal struct {
	Bytes      traffic.EgressBytes `json:"bytes"`
	TotalBytes string              `json:"total_bytes"`
}
type TrafficEgressTotals struct {
	Upstream     TrafficEgressTotal `json:"upstream"`
	Unidentified TrafficEgressTotal `json:"unidentified"`
}
type TrafficEgressReport struct {
	Window         TrafficEgressWindow    `json:"window"`
	PreviousWindow *TrafficPreviousWindow `json:"previous_window"`
	Routes         []TrafficRouteStats    `json:"routes"`
	Totals         TrafficEgressTotals    `json:"totals"`
}
type TrafficEgressHistory struct {
	Start    time.Time           `json:"start"`
	Interval string              `json:"interval"`
	Bytes    traffic.EgressBytes `json:"bytes"`
	Attempts uint64              `json:"attempts"`
	Peaks    traffic.Peaks       `json:"peaks"`
}
type TrafficEgressResidual struct {
	Bytes      traffic.EgressBytes `json:"bytes"`
	TotalBytes string              `json:"total_bytes"`
	Attempts   uint64              `json:"attempts"`
}
type TrafficEgressDetail struct {
	Window              TrafficEgressWindow    `json:"window"`
	PreviousWindow      *TrafficPreviousWindow `json:"previous_window"`
	Summary             TrafficRouteStats      `json:"summary"`
	History             []TrafficEgressHistory `json:"history"`
	Accounts            []TrafficEgressAccount `json:"accounts"`
	UnattributedAccount TrafficEgressResidual  `json:"unattributed_account"`
}
type TrafficEgressQuery struct {
	Window          wgtunnel.TrafficWindow
	Compare, Detail bool
	RouteType       string
	RouteID         int64
}
type TrafficEgressRow struct {
	traffic.EgressIdentity
	Bytes    traffic.EgressBytes
	Attempts uint64
	Peaks    traffic.Peaks
}
type TrafficEgressData struct {
	Window            TrafficEgressWindow
	PreviousWindow    *TrafficPreviousWindow
	Current, Previous []TrafficEgressRow
	Routes            map[traffic.EgressIdentity]TrafficRouteIdentity
	Accounts          map[int64]TrafficEgressAccount
	WireGuard         map[int64]TrafficEgressWG
	History           []TrafficEgressHistory
}
type TrafficEgressRepository interface {
	EgressData(context.Context, TrafficEgressQuery) (TrafficEgressData, error)
}

func egressComparison(now, before uint64, compare bool) (*TrafficEgressPrevious, *string) {
	if !compare {
		return nil, nil
	}
	diff := new(big.Int).Sub(new(big.Int).SetUint64(now), new(big.Int).SetUint64(before)).String()
	return &TrafficEgressPrevious{trafficNumber(before)}, &diff
}
func egressRouteKey(row TrafficEgressRow) traffic.EgressIdentity {
	return traffic.EgressIdentity{Type: row.Type, ID: row.ID}
}
func egressMaps(d TrafficEgressData) (map[traffic.EgressIdentity]TrafficEgressRow, map[traffic.EgressIdentity]TrafficEgressRow, TrafficEgressTotals) {
	current := map[traffic.EgressIdentity]TrafficEgressRow{}
	previous := map[traffic.EgressIdentity]TrafficEgressRow{}
	totals := TrafficEgressTotals{}
	for _, r := range d.Current {
		current[r.EgressIdentity] = r
		if r.AccountID == 0 {
			totals.Upstream.Bytes.Add(r.Bytes)
			if r.Type == "unidentified" {
				totals.Unidentified.Bytes.Add(r.Bytes)
			}
		}
	}
	for _, r := range d.Previous {
		previous[r.EgressIdentity] = r
		if _, ok := current[r.EgressIdentity]; !ok {
			current[r.EgressIdentity] = TrafficEgressRow{EgressIdentity: r.EgressIdentity}
		}
	}
	totals.Upstream.TotalBytes = trafficNumber(totals.Upstream.Bytes.Total())
	totals.Unidentified.TotalBytes = trafficNumber(totals.Unidentified.Bytes.Total())
	return current, previous, totals
}
func egressSummary(d TrafficEgressData, q TrafficEgressQuery, r, p TrafficEgressRow, total uint64) TrafficRouteStats {
	identity, ok := d.Routes[egressRouteKey(r)]
	if !ok {
		identity = TrafficRouteIdentity{RouteType: r.Type, RouteDeleted: r.Type != "direct"}
		if r.Type != "direct" {
			id := r.ID
			identity.RouteID = &id
		}
	}
	result := TrafficRouteStats{TrafficRouteIdentity: identity, Bytes: r.Bytes, TotalBytes: trafficNumber(r.Bytes.Total()), Attempts: r.Attempts, Peaks: r.Peaks, Share: trafficRatio(r.Bytes.Total(), total)}
	for _, row := range d.Current {
		if row.Type == r.Type && row.ID == r.ID && row.AccountID > 0 && row.Bytes.Total() > 0 {
			result.AccountCount++
		}
	}
	if r.Type == "wireguard" {
		wg, ok := d.WireGuard[r.ID]
		if !ok {
			wg = TrafficEgressWG{TX: "0", RX: "0", Total: "0"}
		}
		result.WireGuard = &wg
	}
	result.Previous, result.GrowthBytes = egressComparison(r.Bytes.Total(), p.Bytes.Total(), q.Compare)
	return result
}
func (s *ServiceTrafficService) EgressReport(ctx context.Context, q TrafficEgressQuery) (TrafficEgressReport, error) {
	d, err := s.Egress.EgressData(ctx, q)
	if err != nil {
		return TrafficEgressReport{}, err
	}
	current, previous, totals := egressMaps(d)
	result := TrafficEgressReport{Window: d.Window, PreviousWindow: d.PreviousWindow, Routes: []TrafficRouteStats{}, Totals: totals}
	for key, row := range current {
		if key.AccountID == 0 && key.Type != "unidentified" && (row.Bytes.Total() > 0 || row.Attempts > 0 || previous[key].Bytes.Total() > 0 || previous[key].Attempts > 0) {
			result.Routes = append(result.Routes, egressSummary(d, q, row, previous[key], totals.Upstream.Bytes.Total()))
		}
	}
	sort.Slice(result.Routes, func(i, j int) bool {
		a, b := result.Routes[i], result.Routes[j]
		if a.Bytes.Total() != b.Bytes.Total() {
			return a.Bytes.Total() > b.Bytes.Total()
		}
		if a.RouteType != b.RouteType {
			return a.RouteType < b.RouteType
		}
		if a.RouteID == nil {
			return false
		}
		return *a.RouteID < *b.RouteID
	})
	return result, nil
}
func (s *ServiceTrafficService) EgressDetail(ctx context.Context, q TrafficEgressQuery) (TrafficEgressDetail, error) {
	d, err := s.Egress.EgressData(ctx, q)
	if err != nil {
		return TrafficEgressDetail{}, err
	}
	key := traffic.EgressIdentity{Type: q.RouteType, ID: q.RouteID}
	if _, ok := d.Routes[key]; !ok {
		return TrafficEgressDetail{}, ErrTrafficRouteNotFound
	}
	current, previous, totals := egressMaps(d)
	row := current[key]
	row.EgressIdentity = key
	result := TrafficEgressDetail{Window: d.Window, PreviousWindow: d.PreviousWindow, Summary: egressSummary(d, q, row, previous[key], totals.Upstream.Bytes.Total()), History: d.History, Accounts: []TrafficEgressAccount{}, UnattributedAccount: TrafficEgressResidual{TotalBytes: "0"}}
	for id, r := range current {
		if id.Type != q.RouteType || id.ID != q.RouteID || id.AccountID == 0 {
			continue
		}
		if id.AccountID < 0 {
			result.UnattributedAccount = TrafficEgressResidual{r.Bytes, trafficNumber(r.Bytes.Total()), r.Attempts}
			continue
		}
		before := previous[id]
		if r.Bytes.Total() == 0 && r.Attempts == 0 && before.Bytes.Total() == 0 && before.Attempts == 0 {
			continue
		}
		a, ok := d.Accounts[id.AccountID]
		if !ok {
			a = TrafficEgressAccount{AccountID: id.AccountID, AccountDeleted: true}
		}
		a.Bytes = r.Bytes
		a.TotalBytes = trafficNumber(r.Bytes.Total())
		a.Attempts = r.Attempts
		a.Peaks = r.Peaks
		a.Share = trafficRatio(r.Bytes.Total(), row.Bytes.Total())
		a.Previous, a.GrowthBytes = egressComparison(r.Bytes.Total(), before.Bytes.Total(), q.Compare)
		result.Accounts = append(result.Accounts, a)
	}
	sort.Slice(result.Accounts, func(i, j int) bool {
		a, b := result.Accounts[i], result.Accounts[j]
		if a.Bytes.Total() == b.Bytes.Total() {
			return a.AccountID < b.AccountID
		}
		return a.Bytes.Total() > b.Bytes.Total()
	})
	return result, nil
}
