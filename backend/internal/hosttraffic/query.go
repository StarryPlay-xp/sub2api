package hosttraffic

import (
	"context"
	"database/sql"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
)

func status(snap *snapshot, now time.Time, health Collection) Status {
	state := snap.state
	result := Status{AsOf: now.UTC(), Collection: health, Interfaces: []Interface{}, Plan: state.Plan, Alerts: Alerts{Firing: []Firing{}}}
	result.Collection.StartedAt = state.Started
	result.Collection.LastSampleAt = state.Last
	if state.Last != nil {
		result.Collection.HostBootAt = state.Collection.HostBootAt
	}
	if health.State == "ok" || health.State == "starting" {
		if state.Last == nil {
			result.Collection.State = "starting"
		} else if state.Last.Before(now.Add(-3 * time.Minute)) {
			result.Collection.State = "stale"
		} else {
			result.Collection.State = "ok"
		}
	}
	rs := readings(state)
	names, selection := selectInterfaces(rs, state.Plan)
	result.Selection = selection
	selected := map[string]bool{}
	for _, n := range names {
		selected[n] = true
	}
	for _, r := range rs {
		i := Interface{Name: r.Name, Selected: selected[r.Name], Physical: r.Physical, DefaultRoute: r.Route}
		if old, ok := state.Previous[r.Name]; ok && old.Boot == r.Boot && old.Index == r.Index && integer(r.RX).Cmp(integer(old.RX)) >= 0 && integer(r.TX).Cmp(integer(old.TX)) >= 0 && r.At.After(old.At) && result.Collection.State == "ok" {
			seconds := r.At.Sub(old.At).Seconds()
			i.RXRate = ptr(ratio(sub(r.RX, old.RX), "1") / seconds)
			i.TXRate = ptr(ratio(sub(r.TX, old.TX), "1") / seconds)
		}
		result.Interfaces = append(result.Interfaces, i)
	}
	if state.Started != nil {
		totals := &Totals{From: *state.Started, RX: "0", TX: "0", Total: "0", Bound: "exact"}
		for _, name := range names {
			total := state.Lifetime[name]
			totals.RX = add(totals.RX, total.RX)
			totals.TX = add(totals.TX, total.TX)
		}
		totals.Total = add(totals.RX, totals.TX)
		if state.LifetimeLower || !ready(snap, names, now) {
			totals.Bound = "lower"
		}
		result.SinceStart = totals
	}
	if state.Plan != nil {
		start, end := state.Plan.Bounds(now)
		until := now
		if state.Last != nil && state.Last.Before(until) {
			until = *state.Last
		}
		if until.Before(start) {
			until = start
		}
		result.Period = makePeriod(*state.Plan, start, end, until, snap.spans, names, snap.calibrations)
		if len(names) == 0 || !ready(snap, names, now) {
			result.Period.Bound = "lower"
		}
	}
	if result.Period != nil {
		relevant := []Calibration{}
		key := state.Plan.key(result.Period.Start, result.Period.End, names)
		for _, c := range snap.calibrations {
			if c.Key == key {
				relevant = append(relevant, c)
			}
		}
		result.UnitHint = unitHint(state.Plan, relevant)
	}
	return result
}
func (s *Store) Status(ctx context.Context, now time.Time, health Collection) (Status, error) {
	var result Status
	e := s.transaction(ctx, false, func(tx *sql.Tx, snap *snapshot) error {
		names, _ := selectInterfaces(readings(snap.state), snap.state.Plan)
		from := now
		if snap.state.Plan != nil {
			from, _ = snap.state.Plan.Bounds(now)
		}
		if e := load(ctx, tx, snap, from, names); e != nil {
			return e
		}
		result = status(snap, now, health)
		rows, e := tx.QueryContext(ctx, `SELECT CASE WHEN e.severity='P0' THEN 'critical' ELSE 'warning' END,CASE WHEN r.metric_type='host_traffic_quota_projected_percent' THEN 'projection' ELSE 'usage' END,e.fired_at
 FROM ops_alert_events e JOIN ops_alert_rules r ON r.id=e.rule_id WHERE e.status='firing' AND r.enabled AND r.managed_by LIKE 'host_traffic:%' ORDER BY e.fired_at`)
		if e != nil {
			return e
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var f Firing
			if e = rows.Scan(&f.Level, &f.Kind, &f.Since); e != nil {
				return e
			}
			result.Alerts.Firing = append(result.Alerts.Firing, f)
		}
		return rows.Err()
	})
	return result, e
}
func (s *Store) Usage(ctx context.Context, rangeName string, now time.Time, health Collection) (Usage, error) {
	result := Usage{Interfaces: []string{}, RX: "0", TX: "0", Total: "0"}
	// Reuse the preset alignment (minute for 24h, hour for other presets).
	if rangeName == "custom" {
		return result, &wgtunnel.TrafficQueryError{Reason: "TRAFFIC_RANGE_INVALID", Message: "range must be 24h, 7d, 30d or 90d"}
	}
	window, e := wgtunnel.ResolveTrafficWindow(wgtunnel.TrafficQuery{Range: rangeName}, now)
	if e != nil {
		return result, e
	}
	result.Window = Window{Range: window.Range, Start: window.Start, End: window.End, AsOf: window.AsOf, RetainedFrom: now.AddDate(0, -13, 0).Truncate(time.Hour)}
	e = s.transaction(ctx, false, func(tx *sql.Tx, snap *snapshot) error {
		names, _ := selectInterfaces(readings(snap.state), snap.state.Plan)
		if e := load(ctx, tx, snap, window.Start, names); e != nil {
			return e
		}
		st := status(snap, now, health)
		result.State = st.Collection.State
		result.Interfaces, _ = selectInterfaces(readings(snap.state), snap.state.Plan)
		v := summarize(snap.spans, result.Interfaces, window.Start, window.End)
		result.RX = v.rx
		result.TX = v.tx
		result.Total = add(v.rx, v.tx)
		result.Observed = v.observed
		result.Expected = int64(window.End.Sub(window.Start).Seconds())
		result.HasRecords = v.from != nil
		return nil
	})
	return result, e
}
func (s *Store) Detail(ctx context.Context, now time.Time) (Detail, error) {
	result := Detail{Timezone: "UTC", Days: []Day{}, Periods: []ClosedPeriod{}, Calibrations: []Calibration{}}
	e := s.transaction(ctx, false, func(tx *sql.Tx, snap *snapshot) error {
		p := snap.state.Plan
		start := now.UTC().AddDate(0, 0, -29).Truncate(24 * time.Hour)
		end := now
		mode := "both"
		if p != nil {
			result.Timezone = p.Timezone
			start, end = p.Bounds(now)
			mode = p.BillingMode
		}
		names, _ := selectInterfaces(readings(snap.state), p)
		if e := load(ctx, tx, snap, start, names); e != nil {
			return e
		}
		result.Periods = append(result.Periods, snap.periods...)
		result.Calibrations = append(result.Calibrations, snap.calibrations[:min(20, len(snap.calibrations))]...)
		loc, _ := time.LoadLocation(result.Timezone)
		for a := start; a.Before(end); {
			local := a.In(loc)
			b := time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, loc).UTC()
			if b.After(end) {
				b = end
			}
			until := b
			if until.After(now) {
				until = now
			}
			v := summarize(snap.spans, names, a, until)
			result.Days = append(result.Days, Day{Date: local.Format("2006-01-02"), Start: a, End: b, RX: v.rx, TX: v.tx, Billed: billed(v.rx, v.tx, mode), Observed: v.observed, Expected: max(0, int64(until.Sub(a).Seconds()))})
			a = b
		}
		return nil
	})
	return result, e
}

// Metric is shared by managed and manually-created operations alert rules.
// An unavailable projection yields zero so the evaluator can resolve a prior
// period's event. Unavailable collection yields no value, never a false recovery.
func (s *Store) Metric(ctx context.Context, metric string, now time.Time, health Collection) (float64, bool) {
	st, e := s.Status(ctx, now, health)
	if e != nil || st.Period == nil || st.Collection.State != "ok" || st.Selection.Ambiguous {
		return 0, false
	}
	if metric == "host_traffic_quota_used_percent" {
		return st.Period.UsedRatio * 100, true
	}
	if metric == "host_traffic_quota_projected_percent" {
		if st.Period.Projection == nil {
			return 0, true
		}
		return st.Period.Projection.Ratio * 100, true
	}
	return 0, false
}
