package hosttraffic

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type Store struct{ DB *sql.DB }
type counters struct{ RX, TX string }
type persisted struct {
	Plan          *Plan
	Started       *time.Time
	Last          *time.Time
	Collection    Collection
	Latest        map[string]Reading
	Previous      map[string]Reading
	Lifetime      map[string]counters
	LifetimeLower bool
	ActiveStart   *time.Time
}
type snapshot struct {
	state        persisted
	spans        []Span
	calibrations []Calibration
	periods      []ClosedPeriod
}

func (s *Store) transaction(ctx context.Context, write bool, fn func(*sql.Tx, *snapshot) error) error {
	opts := &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: !write}
	if write {
		opts.Isolation = sql.LevelReadCommitted
	}
	tx, e := s.DB.BeginTx(ctx, opts)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback() }()
	q := `SELECT data FROM host_traffic_state WHERE id=1`
	if write {
		q += ` FOR UPDATE`
	}
	var raw []byte
	var snap snapshot
	if e = tx.QueryRowContext(ctx, q).Scan(&raw); e != nil {
		return e
	}
	if e = json.Unmarshal(raw, &snap.state); e != nil {
		return e
	}
	if snap.state.Latest == nil {
		snap.state.Latest = map[string]Reading{}
	}
	if snap.state.Previous == nil {
		snap.state.Previous = map[string]Reading{}
	}
	if snap.state.Lifetime == nil {
		snap.state.Lifetime = map[string]counters{}
	}
	if e = fn(tx, &snap); e != nil {
		return e
	}
	if write {
		raw, e = json.Marshal(snap.state)
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE host_traffic_state SET data=$1 WHERE id=1`, string(raw)); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func load(ctx context.Context, tx *sql.Tx, snap *snapshot, from time.Time, selection ...[]string) error {
	query := `SELECT spans FROM host_traffic_hours WHERE hour >= $1`
	args := []any{from.Truncate(time.Hour)}
	if len(selection) > 0 {
		query += ` AND interface_name = ANY($2)`
		args = append(args, pq.Array(selection[0]))
	}
	query += ` ORDER BY hour,interface_name`
	rows, e := tx.QueryContext(ctx, query, args...)
	if e != nil {
		return e
	}
	for rows.Next() {
		var raw []byte
		var spans []Span
		if e = rows.Scan(&raw); e != nil {
			break
		}
		if e = json.Unmarshal(raw, &spans); e != nil {
			break
		}
		snap.spans = append(snap.spans, spans...)
	}
	rowErr := rows.Err()
	_ = rows.Close()
	if e != nil {
		return e
	}
	if rowErr != nil {
		return rowErr
	}
	rows, e = tx.QueryContext(ctx, `SELECT id,accounting_key,adjustment,data FROM host_traffic_calibrations ORDER BY at DESC,id DESC`)
	if e != nil {
		return e
	}
	for rows.Next() {
		var raw []byte
		var c Calibration
		var id int64
		if e = rows.Scan(&id, &c.Key, &c.Adjustment, &raw); e != nil {
			break
		}
		if e = json.Unmarshal(raw, &c); e != nil {
			break
		}
		c.ID = id
		snap.calibrations = append(snap.calibrations, c)
	}
	rowErr = rows.Err()
	_ = rows.Close()
	if e != nil {
		return e
	}
	if rowErr != nil {
		return rowErr
	}
	rows, e = tx.QueryContext(ctx, `SELECT data FROM host_traffic_periods ORDER BY period_start DESC LIMIT 12`)
	if e != nil {
		return e
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var raw []byte
		var p ClosedPeriod
		if e = rows.Scan(&raw); e != nil {
			return e
		}
		if e = json.Unmarshal(raw, &p); e != nil {
			return e
		}
		snap.periods = append(snap.periods, p)
	}
	return rows.Err()
}
func (s *Store) Save(ctx context.Context, sample Sample) error {
	return s.transaction(ctx, true, func(tx *sql.Tx, snap *snapshot) error {
		state := &snap.state
		if state.Last != nil && !sample.At.After(*state.Last) {
			return nil
		} // first writer wins for each minute
		first := state.Started == nil
		if first {
			state.Started = ptr(sample.At)
		}
		for _, r := range sample.Readings {
			var prev *Reading
			if old, ok := state.Latest[r.Name]; ok {
				prev = &old
			}
			span := difference(prev, r)
			if prev == nil && !first && state.Last != nil {
				span.Baseline = false
				span.From = *state.Last
				span.Lost = max(1, int64(r.At.Sub(*state.Last).Seconds()))
				span.Uncertain = true
				span.GapFrom = ptr(*state.Last)
				span.GapTo = ptr(r.At)
			}
			raw, e := json.Marshal(r)
			if e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, `INSERT INTO host_traffic_samples VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, r.Boot, r.Name, r.Index, r.At, string(raw)); e != nil {
				return e
			}
			raw, e = json.Marshal([]Span{span})
			if e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, `INSERT INTO host_traffic_hours VALUES($1,$2,$3) ON CONFLICT(hour,interface_name) DO UPDATE SET spans=host_traffic_hours.spans || EXCLUDED.spans`, r.At.Truncate(time.Hour), r.Name, string(raw)); e != nil {
				return e
			}
			if !first {
				total := state.Lifetime[r.Name]
				total.RX = add(total.RX, span.RX)
				total.TX = add(total.TX, span.TX)
				state.Lifetime[r.Name] = total
				state.LifetimeLower = state.LifetimeLower || span.Lost > 0 || span.Uncertain
			}
			if prev != nil {
				state.Previous[r.Name] = *prev
			}
			state.Latest[r.Name] = r
		}
		state.Last = ptr(sample.At)
		state.Collection = sample.Collection
		from := sample.At.AddDate(0, -13, 0)
		if state.Plan != nil {
			current, _ := state.Plan.Bounds(sample.At)
			if state.ActiveStart == nil || state.ActiveStart.Before(current) {
				historyFrom := current
				if state.ActiveStart != nil && state.ActiveStart.Before(historyFrom) {
					historyFrom = *state.ActiveStart
				}
				if historyFrom.Before(from) {
					historyFrom = from
				}
				if e := load(ctx, tx, snap, historyFrom); e != nil {
					return e
				}
				if e := closePeriods(ctx, tx, snap, sample.At); e != nil {
					return e
				}
			}
		}
		if sample.At.Minute() != 0 {
			return nil
		}
		if _, e := tx.ExecContext(ctx, `DELETE FROM host_traffic_samples WHERE sampled_at<$1`, sample.At.AddDate(0, 0, -7)); e != nil {
			return e
		}
		if _, e := tx.ExecContext(ctx, `DELETE FROM host_traffic_hours WHERE hour<$1`, from.Truncate(time.Hour)); e != nil {
			return e
		}
		return nil
	})
}
func readings(state persisted) []Reading {
	result := []Reading{}
	for _, r := range state.Latest {
		if state.Last != nil && r.At.Equal(*state.Last) {
			result = append(result, r)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}
func closePeriods(ctx context.Context, tx *sql.Tx, snap *snapshot, now time.Time) error {
	p := snap.state.Plan
	if p == nil {
		return nil
	}
	current, _ := p.Bounds(now)
	if snap.state.ActiveStart == nil {
		snap.state.ActiveStart = &current
		return nil
	}
	start := *snap.state.ActiveStart
	names, _ := selectInterfaces(readings(snap.state), p)
	// Empty old periods are frozen as lower bounds after long outages.
	// Only the last 13 months of differences remain available.
	for start.Before(current) {
		_, end := p.Bounds(start)
		if !end.After(start) {
			return errors.New("invalid host traffic period")
		}
		period := makePeriod(*p, start, end, end, snap.spans, names, snap.calibrations)
		closed := ClosedPeriod{Period: *period, BillingMode: p.BillingMode, ClosedAt: now}
		raw, e := json.Marshal(closed)
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO host_traffic_periods VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, start, end, string(raw)); e != nil {
			return e
		}
		start = end
	}
	snap.state.ActiveStart = &current
	return nil
}
func ready(snap *snapshot, names []string, now time.Time) bool {
	if len(names) == 0 || snap.state.Last == nil || snap.state.Last.Before(now.Add(-3*time.Minute)) || snap.state.Last.After(now) {
		return false
	}
	for _, name := range names {
		r, ok := snap.state.Latest[name]
		if !ok || !r.At.Equal(*snap.state.Last) {
			return false
		}
	}
	return true
}
func insertCalibration(ctx context.Context, tx *sql.Tx, snap *snapshot, amount, mode string, now time.Time) (string, error) {
	p := snap.state.Plan
	if p == nil {
		return "", ErrNoPlan
	}
	names, _ := selectInterfaces(readings(snap.state), p)
	if !ready(snap, names, now) {
		return "", ErrNotReady
	}
	at := *snap.state.Last
	start, end := p.Bounds(now)
	// A pre-reset sample is not a baseline for the new period.
	if at.Before(start) {
		return "", ErrNotReady
	}
	period := makePeriod(*p, start, end, at, snap.spans, names, snap.calibrations)
	c := calibrate(*p, period, amount, mode, at, names)
	raw, e := json.Marshal(c)
	if e != nil {
		return "", e
	}
	e = tx.QueryRowContext(ctx, `INSERT INTO host_traffic_calibrations(at,period_start,accounting_key,adjustment,data) VALUES($1,$2,$3,$4,$5) RETURNING id`, c.At, c.PeriodStart, c.Key, c.Adjustment, string(raw)).Scan(&c.ID)
	if e != nil {
		return "", e
	}
	snap.calibrations = append([]Calibration{c}, snap.calibrations...)
	return c.Kind, nil
}
func (s *Store) UpdatePlan(ctx context.Context, in PlanInput, now time.Time) error {
	plan, e := in.Validate()
	if e != nil {
		return e
	}
	return s.transaction(ctx, true, func(tx *sql.Tx, snap *snapshot) error {
		old := snap.state.Plan
		if (old == nil && in.Version != nil) || (old != nil && (in.Version == nil || *in.Version != old.Version)) {
			return ErrConflict
		}
		historyFrom := now.AddDate(0, 0, -40)
		if snap.state.ActiveStart != nil && snap.state.ActiveStart.Before(historyFrom) {
			historyFrom = *snap.state.ActiveStart
		}
		if e := load(ctx, tx, snap, historyFrom); e != nil {
			return e
		}
		if e := closePeriods(ctx, tx, snap, now); e != nil {
			return e
		}
		plan.Version = uuid.NewString()
		plan.UpdatedAt = now.UTC()
		snap.state.Plan = plan
		start, _ := plan.Bounds(now)
		snap.state.ActiveStart = &start
		if in.CurrentUsed != nil {
			amount, _ := Bytes(in.CurrentUsed.Value, in.CurrentUsed.Unit, plan.UnitBase)
			if _, e := insertCalibration(ctx, tx, snap, amount, "auto", now); e != nil {
				return e
			}
		}
		return syncRules(ctx, tx, plan, now)
	})
}
func (s *Store) Calibrate(ctx context.Context, in CalibrationInput, now time.Time) (string, error) {
	kind := ""
	if in.Mode != "auto" && in.Mode != "override" {
		return "", ErrCalibration
	}
	e := s.transaction(ctx, true, func(tx *sql.Tx, snap *snapshot) error {
		p := snap.state.Plan
		if p == nil {
			return ErrNoPlan
		}
		if p.Version != in.PlanVersion {
			return ErrConflict
		}
		amount, e := Bytes(in.Value, in.Unit, p.UnitBase)
		if e != nil {
			return ErrCalibration
		}
		if e = load(ctx, tx, snap, now.AddDate(0, 0, -40)); e != nil {
			return e
		}
		kind, e = insertCalibration(ctx, tx, snap, amount, in.Mode, now)
		return e
	})
	return kind, e
}
func syncRules(ctx context.Context, tx *sql.Tx, p *Plan, now time.Time) error {
	for _, r := range []struct {
		key, level, metric string
		threshold          float64
		enabled            bool
	}{
		{"usage_warning", "P1", "host_traffic_quota_used_percent", p.Alerts.Warning, p.Alerts.Enabled},
		{"usage_critical", "P0", "host_traffic_quota_used_percent", p.Alerts.Critical, p.Alerts.Enabled},
		{"projection", "P1", "host_traffic_quota_projected_percent", 100, p.Alerts.Enabled && p.Alerts.Projection},
	} {
		_, e := tx.ExecContext(ctx, `INSERT INTO ops_alert_rules(name,description,enabled,severity,metric_type,operator,threshold,window_minutes,sustained_minutes,cooldown_minutes,notify_email,managed_by)
 VALUES($1,'Managed by the local host traffic plan', $2,$3,$4,'>=',$5,1,1,60,true,$6)
 ON CONFLICT(managed_by) WHERE managed_by IS NOT NULL DO UPDATE SET enabled=EXCLUDED.enabled,severity=EXCLUDED.severity,metric_type=EXCLUDED.metric_type,operator=EXCLUDED.operator,threshold=EXCLUDED.threshold,updated_at=$7`, "host_traffic:"+r.key, r.enabled, r.level, r.metric, r.threshold, "host_traffic:"+r.key, now)
		if e != nil {
			return e
		}
	}
	_, e := tx.ExecContext(ctx, `UPDATE ops_alert_events SET status='resolved',resolved_at=$1 WHERE status='firing' AND rule_id IN (SELECT id FROM ops_alert_rules WHERE managed_by LIKE 'host_traffic:%' AND NOT enabled)`, now)
	return e
}
