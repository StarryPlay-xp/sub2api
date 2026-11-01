package service

import (
	"context"
	"errors"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
)

var ErrTrafficSubjectNotFound = errors.New("traffic subject not found")

type TrafficIdentity struct {
	SubjectType   string  `json:"subject_type"`
	UserID        int64   `json:"user_id"`
	UserEmail     string  `json:"user_email"`
	Username      string  `json:"username"`
	UserDeleted   bool    `json:"user_deleted"`
	APIKeyID      *int64  `json:"api_key_id"`
	APIKeyName    *string `json:"api_key_name"`
	APIKeyDeleted *bool   `json:"api_key_deleted"`
	GroupID       *int64  `json:"group_id"`
	GroupName     *string `json:"group_name"`
}

func (i TrafficIdentity) ID() int64 {
	if i.APIKeyID != nil {
		return *i.APIKeyID
	}
	return i.UserID
}

type TrafficPrevious struct {
	TotalBytes string        `json:"total_bytes"`
	Peaks      traffic.Peaks `json:"peaks"`
}
type TrafficSubjectStats struct {
	TrafficIdentity
	Bytes           traffic.Bytes    `json:"bytes"`
	InBytes         string           `json:"in_bytes"`
	OutBytes        string           `json:"out_bytes"`
	TotalBytes      string           `json:"total_bytes"`
	Share           *float64         `json:"share"`
	Requests        uint64           `json:"requests"`
	BytesPerRequest *string          `json:"bytes_per_request"`
	ActiveSeconds   uint64           `json:"active_seconds"`
	ActiveAverage   *float64         `json:"active_avg_bytes_per_second"`
	Peaks           traffic.Peaks    `json:"peaks"`
	ActiveAPIKeys   *uint64          `json:"active_api_keys"`
	Previous        *TrafficPrevious `json:"previous"`
	GrowthBytes     *string          `json:"growth_bytes"`
}
type TrafficRankingWindow struct {
	wgtunnel.TrafficWindow
	MetricBasis          string     `json:"metric_basis"`
	AttributionStartedAt *time.Time `json:"attribution_started_at"`
	HasRecords           bool       `json:"has_records"`
	Incomplete           bool       `json:"incomplete"`
	ConcurrentRuntimes   bool       `json:"concurrent_runtimes"`
	SubjectOverflow      bool       `json:"subject_overflow"`
}
type TrafficPreviousWindow struct {
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	Complete bool      `json:"complete"`
}
type TrafficAttributedTotal struct {
	Bytes      traffic.Bytes `json:"bytes"`
	TotalBytes string        `json:"total_bytes"`
	Requests   uint64        `json:"requests"`
}
type TrafficUnattributedTotal struct {
	Bytes      traffic.Bytes `json:"bytes"`
	TotalBytes string        `json:"total_bytes"`
}
type TrafficSubjectTotals struct {
	Attributed    TrafficAttributedTotal   `json:"attributed"`
	Unattributed  TrafficUnattributedTotal `json:"unattributed"`
	AllTotalBytes string                   `json:"all_total_bytes"`
}
type TrafficConcentration struct {
	Subjects int      `json:"subjects"`
	Top1     *float64 `json:"top1"`
	Top5     *float64 `json:"top5"`
	Top10    *float64 `json:"top10"`
}
type TrafficRanking struct {
	Window         TrafficRankingWindow   `json:"window"`
	PreviousWindow *TrafficPreviousWindow `json:"previous_window"`
	Subject        string                 `json:"subject"`
	Sort           string                 `json:"sort"`
	Order          string                 `json:"order"`
	Page           int                    `json:"page"`
	PageSize       int                    `json:"page_size"`
	Total          int                    `json:"total"`
	Items          []TrafficSubjectStats  `json:"items"`
	Totals         TrafficSubjectTotals   `json:"totals"`
	Concentration  TrafficConcentration   `json:"concentration"`
}
type TrafficSubjectHistory struct {
	Start         time.Time     `json:"start"`
	Interval      string        `json:"interval"`
	Bytes         traffic.Bytes `json:"bytes"`
	Requests      uint64        `json:"requests"`
	ActiveSeconds uint64        `json:"active_seconds"`
	Peaks         traffic.Peaks `json:"peaks"`
}
type TrafficSubjectDetail struct {
	Window         TrafficRankingWindow    `json:"window"`
	PreviousWindow *TrafficPreviousWindow  `json:"previous_window"`
	Summary        TrafficSubjectStats     `json:"summary"`
	History        []TrafficSubjectHistory `json:"history"`
}
type TrafficSubjectQuery struct {
	Window                  wgtunnel.TrafficWindow
	Subject, Sort, Order, Q string
	Compare                 bool
	Page, PageSize          int
	UserID, ID              int64
}
type TrafficSubjectData struct {
	Window            TrafficRankingWindow
	PreviousWindow    *TrafficPreviousWindow
	Current, Previous []TrafficSubjectStats
	Unattributed      traffic.Bytes
	History           []TrafficSubjectHistory
	Identity          *TrafficIdentity
}
type TrafficSubjectRepository interface {
	SubjectData(context.Context, TrafficSubjectQuery) (TrafficSubjectData, error)
	SubjectIdentities(context.Context, string, []int64) (map[int64]TrafficIdentity, error)
}

func trafficNumber(n uint64) string { return strconv.FormatUint(n, 10) }
func trafficRatio(n, d uint64) *float64 {
	if d == 0 {
		return nil
	}
	v := float64(n) / float64(d)
	return &v
}
func fillTrafficStats(s *TrafficSubjectStats, total uint64, previous *TrafficSubjectStats, compare bool) {
	v := s.Bytes.Vector()
	s.InBytes = trafficNumber(v[4])
	s.OutBytes = trafficNumber(v[5])
	s.TotalBytes = trafficNumber(v[6])
	s.Share = trafficRatio(v[6], total)
	s.ActiveAverage = trafficRatio(v[6], s.ActiveSeconds)
	if s.Requests > 0 {
		b := trafficNumber(v[6] / s.Requests)
		s.BytesPerRequest = &b
	}
	if len(s.Peaks) == 0 {
		s.Peaks = nil
	}
	if s.SubjectType == "user" && s.ActiveAPIKeys == nil {
		zero := uint64(0)
		s.ActiveAPIKeys = &zero
	}
	if compare {
		var before uint64
		var peaks traffic.Peaks
		if previous != nil {
			before = previous.Bytes.Vector()[6]
			peaks = previous.Peaks
		}
		s.Previous = &TrafficPrevious{trafficNumber(before), peaks}
		diff := new(big.Int).Sub(new(big.Int).SetUint64(v[6]), new(big.Int).SetUint64(before)).String()
		s.GrowthBytes = &diff
	}
}
func combineTrafficSubjects(q TrafficSubjectQuery, d TrafficSubjectData) ([]TrafficSubjectStats, TrafficSubjectTotals) {
	current := map[int64]TrafficSubjectStats{}
	previous := map[int64]TrafficSubjectStats{}
	totals := TrafficSubjectTotals{Unattributed: TrafficUnattributedTotal{Bytes: d.Unattributed}}
	for _, s := range d.Current {
		current[s.ID()] = s
		totals.Attributed.Bytes.Add(s.Bytes)
		totals.Attributed.Requests += s.Requests
	}
	for _, s := range d.Previous {
		previous[s.ID()] = s
		if _, ok := current[s.ID()]; !ok {
			current[s.ID()] = TrafficSubjectStats{TrafficIdentity: s.TrafficIdentity}
		}
	}
	attributed := totals.Attributed.Bytes.Vector()[6]
	unattributed := d.Unattributed.Vector()[6]
	total := attributed + unattributed
	totals.Attributed.TotalBytes = trafficNumber(attributed)
	totals.Unattributed.TotalBytes = trafficNumber(unattributed)
	totals.AllTotalBytes = trafficNumber(total)
	items := []TrafficSubjectStats{}
	for id, s := range current {
		p := previous[id]
		fillTrafficStats(&s, total, &p, q.Compare)
		if q.UserID == 0 || s.UserID == q.UserID {
			items = append(items, s)
		}
	}
	return items, totals
}
func (s *ServiceTrafficService) Ranking(ctx context.Context, q TrafficSubjectQuery) (TrafficRanking, error) {
	d, err := s.Subjects.SubjectData(ctx, q)
	if err != nil {
		return TrafficRanking{}, err
	}
	items, totals := combineTrafficSubjects(q, d)
	result := TrafficRanking{Window: d.Window, PreviousWindow: d.PreviousWindow, Subject: q.Subject, Sort: q.Sort, Order: q.Order, Page: q.Page, PageSize: q.PageSize, Total: len(items), Totals: totals, Items: []TrafficSubjectStats{}}
	sort.Slice(items, func(i, j int) bool {
		a, b := items[i].Bytes.Vector()[6], items[j].Bytes.Vector()[6]
		if a == b {
			return items[i].ID() < items[j].ID()
		}
		return a > b
	})
	denom := totals.Attributed.Bytes.Vector()[6] + totals.Unattributed.Bytes.Vector()[6]
	result.Concentration.Subjects = len(items)
	sum := uint64(0)
	for i, item := range items {
		sum += item.Bytes.Vector()[6]
		if i == 0 {
			result.Concentration.Top1 = trafficRatio(sum, denom)
		}
		if i < 5 {
			result.Concentration.Top5 = trafficRatio(sum, denom)
		}
		if i < 10 {
			result.Concentration.Top10 = trafficRatio(sum, denom)
		}
	}
	if len(items) == 0 {
		result.Concentration.Top1 = trafficRatio(0, denom)
		result.Concentration.Top5 = trafficRatio(0, denom)
		result.Concentration.Top10 = trafficRatio(0, denom)
	}
	filtered := items[:0]
	needle := strings.ToLower(q.Q)
	for _, item := range items {
		name := item.UserEmail + "\n" + item.Username
		if item.APIKeyName != nil {
			name += "\n" + *item.APIKeyName
		}
		if strings.Contains(strings.ToLower(name), needle) {
			filtered = append(filtered, item)
		}
	}
	items = filtered
	sort.Slice(items, func(i, j int) bool {
		a, b := trafficSortValue(items[i], q.Sort), trafficSortValue(items[j], q.Sort)
		if a == nil || b == nil {
			if a == nil && b == nil {
				return items[i].ID() < items[j].ID()
			}
			return a != nil
		}
		cmp := a.Cmp(b)
		if cmp == 0 {
			return items[i].ID() < items[j].ID()
		}
		if q.Order == "asc" {
			return cmp < 0
		}
		return cmp > 0
	})
	// Avoid integer overflow from an otherwise valid very large page number.
	if q.Page > 0 && q.PageSize > 0 && q.Page-1 <= len(items)/q.PageSize {
		start := (q.Page - 1) * q.PageSize
		end := start + q.PageSize
		if end > len(items) {
			end = len(items)
		}
		result.Items = items[start:end]
	}
	return result, nil
}
func trafficSortValue(s TrafficSubjectStats, key string) *big.Rat {
	v := s.Bytes.Vector()
	n := v[6]
	d := uint64(1)
	switch key {
	case "in_bytes":
		n = v[4]
	case "out_bytes":
		n = v[5]
	case "requests":
		n = s.Requests
	case "bytes_per_request":
		d = s.Requests
	case "active_avg_rate":
		d = s.ActiveSeconds
	case "growth":
		if s.GrowthBytes == nil {
			return nil
		}
		r, _ := new(big.Rat).SetString(*s.GrowthBytes)
		return r
	case "peak_total", "peak_in", "peak_out":
		p, ok := s.Peaks[strings.TrimPrefix(key, "peak_")]
		if !ok {
			return nil
		}
		return new(big.Rat).SetFloat64(p.BytesPerSecond)
	}
	if d == 0 {
		return nil
	}
	return new(big.Rat).SetFrac(new(big.Int).SetUint64(n), new(big.Int).SetUint64(d))
}
func (s *ServiceTrafficService) SubjectDetail(ctx context.Context, q TrafficSubjectQuery) (TrafficSubjectDetail, error) {
	d, err := s.Subjects.SubjectData(ctx, q)
	if err != nil {
		return TrafficSubjectDetail{}, err
	}
	if d.Identity == nil {
		return TrafficSubjectDetail{}, ErrTrafficSubjectNotFound
	}
	items, totals := combineTrafficSubjects(q, d)
	summary := TrafficSubjectStats{TrafficIdentity: *d.Identity}
	fillTrafficStats(&summary, totals.Attributed.Bytes.Vector()[6]+totals.Unattributed.Bytes.Vector()[6], nil, q.Compare)
	for _, item := range items {
		if item.ID() == q.ID {
			summary = item
			break
		}
	}
	return TrafficSubjectDetail{d.Window, d.PreviousWindow, summary, d.History}, nil
}

type TrafficLiveItem struct {
	TrafficIdentity
	Rate           traffic.RateTriple `json:"rate"`
	Average        traffic.RateTriple `json:"avg_60s"`
	AverageSeconds float64            `json:"avg_60s_seconds"`
	InFlight       int64              `json:"in_flight"`
	LastActiveAt   *time.Time         `json:"last_active_at"`
}
type TrafficLiveRates struct {
	Rate    traffic.RateTriple `json:"rate"`
	Average traffic.RateTriple `json:"avg_60s"`
}
type TrafficLiveResult struct {
	Subject       string            `json:"subject"`
	State         string            `json:"state"`
	SampledAt     time.Time         `json:"sampled_at"`
	WindowSeconds float64           `json:"window_seconds"`
	Host          TrafficLiveRates  `json:"host"`
	Unattributed  TrafficLiveRates  `json:"unattributed"`
	Items         []TrafficLiveItem `json:"items"`
}
type trafficIdentityCacheKey struct {
	subject string
	id      int64
}
type trafficIdentityCacheEntry struct {
	value   TrafficIdentity
	expires time.Time
}
type trafficIdentityCache struct {
	mu      sync.Mutex
	entries map[trafficIdentityCacheKey]trafficIdentityCacheEntry
}

func (s *ServiceTrafficService) LiveSubjects(ctx context.Context, subject, order string, limit int) (TrafficLiveResult, error) {
	live := s.Collector.Meter.Live(subject)
	result := TrafficLiveResult{Subject: subject, State: live.Current.State, SampledAt: live.Current.At, WindowSeconds: live.Current.WindowSeconds, Host: TrafficLiveRates{live.Host.Rate, live.Host.Average}, Unattributed: TrafficLiveRates{live.Unattributed.Rate, live.Unattributed.Average}, Items: []TrafficLiveItem{}}
	sort.Slice(live.Items, func(i, j int) bool {
		a, b := live.Items[i].Average.Total, live.Items[j].Average.Total
		if order == "rate" {
			a, b = live.Items[i].Rate.Total, live.Items[j].Rate.Total
		}
		if a == b {
			return live.Items[i].ID < live.Items[j].ID
		}
		return a > b
	})
	if len(live.Items) > limit {
		live.Items = live.Items[:limit]
	}
	s.identityCache.mu.Lock()
	defer s.identityCache.mu.Unlock()
	if s.identityCache.entries == nil {
		s.identityCache.entries = map[trafficIdentityCacheKey]trafficIdentityCacheEntry{}
	}
	now := time.Now()
	for k, v := range s.identityCache.entries {
		if !v.expires.After(now) {
			delete(s.identityCache.entries, k)
		}
	}
	ids := []int64{}
	for _, v := range live.Items {
		if _, ok := s.identityCache.entries[trafficIdentityCacheKey{subject, v.ID}]; !ok {
			ids = append(ids, v.ID)
		}
	}
	if len(ids) > 0 {
		identities, err := s.Subjects.SubjectIdentities(ctx, subject, ids)
		if err != nil {
			return result, err
		}
		for _, v := range live.Items {
			key := trafficIdentityCacheKey{subject, v.ID}
			if _, cached := s.identityCache.entries[key]; cached {
				continue
			}
			identity, ok := identities[v.ID]
			if !ok {
				identity = TrafficIdentity{SubjectType: subject, UserID: v.UserID, UserDeleted: true}
				if subject == "api_key" {
					id := v.ID
					deleted := true
					identity.APIKeyID = &id
					identity.APIKeyDeleted = &deleted
				}
			}
			s.identityCache.entries[key] = trafficIdentityCacheEntry{identity, now.Add(time.Minute)}
		}
	}
	for _, v := range live.Items {
		identity := s.identityCache.entries[trafficIdentityCacheKey{subject, v.ID}].value
		result.Items = append(result.Items, TrafficLiveItem{identity, v.Rate, v.Average, v.AverageSeconds, v.InFlight, v.LastActiveAt})
	}
	return result, nil
}
