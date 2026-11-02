package hosttraffic

import (
	"math"
	"math/big"
	"sort"
	"time"
)

// Spans retain differences, never absolute counters. A gap crossing a boundary
// cannot be split without inventing usage. Such bytes are explicitly unallocated.
type Span struct {
	Name      string
	From      time.Time
	To        time.Time
	RX        string
	TX        string
	Lost      int64
	Minor     int
	Uncertain bool
	Baseline  bool
	GapFrom   *time.Time
	GapTo     *time.Time
}

func difference(prev *Reading, cur Reading) Span {
	s := Span{Name: cur.Name, From: cur.BootAt, To: cur.At, RX: cur.RX, TX: cur.TX, Baseline: prev == nil}
	if prev == nil {
		return s
	}
	if cur.Boot == prev.Boot && cur.Index == prev.Index && integer(cur.RX).Cmp(integer(prev.RX)) >= 0 && integer(cur.TX).Cmp(integer(prev.TX)) >= 0 {
		s.From = prev.At
		s.RX = sub(cur.RX, prev.RX)
		s.TX = sub(cur.TX, prev.TX)
		return s
	}
	if cur.Boot != prev.Boot && cur.BootAt.After(prev.At) {
		s.GapFrom = ptr(prev.At)
		s.GapTo = ptr(cur.BootAt)
		loss := int64(cur.BootAt.Sub(prev.At).Seconds())
		if loss <= 120 {
			s.Minor = 1
		} else {
			s.Lost = loss
		}
		return s
	}
	// A one-sided reset must not count the other direction twice.
	if cur.Boot == prev.Boot && cur.Index == prev.Index {
		if integer(cur.RX).Cmp(integer(prev.RX)) >= 0 {
			s.RX = sub(cur.RX, prev.RX)
		}
		if integer(cur.TX).Cmp(integer(prev.TX)) >= 0 {
			s.TX = sub(cur.TX, prev.TX)
		}
	}
	// Neither ifindex changes nor counter decreases reveal when the reset happened.
	s.From = prev.At
	s.GapFrom = ptr(prev.At)
	s.GapTo = ptr(cur.At)
	s.Lost = max(1, int64(cur.At.Sub(prev.At).Seconds()))
	s.Uncertain = true
	return s
}

type summary struct {
	rx, tx, unallocated string
	observed            int64
	from                *time.Time
	lost                int64
	minor               int
	uncertain           bool
}

func summarize(spans []Span, names []string, start, end time.Time) summary {
	out := summary{rx: "0", tx: "0", unallocated: "0"}
	if !end.After(start) {
		return out
	}
	losses := map[time.Time]int64{}
	restarts := map[time.Time]bool{}
	selected := map[string]bool{}
	for _, n := range names {
		selected[n] = true
	}
	type interval struct{ a, b time.Time }
	intervals := map[string][]interval{}
	minorGaps := map[string][]interval{}
	first := map[string]time.Time{}
	last := map[string]time.Time{}
	for _, s := range spans {
		if !selected[s.Name] {
			continue
		}
		if s.GapFrom != nil && s.GapTo != nil && s.GapTo.After(start) && s.GapFrom.Before(end) {
			if s.Minor > 0 {
				restarts[*s.GapTo] = true
				minorGaps[s.Name] = append(minorGaps[s.Name], interval{*s.GapFrom, *s.GapTo})
			} else {
				a, b := *s.GapFrom, *s.GapTo
				if a.Before(start) {
					a = start
				}
				if b.After(end) {
					b = end
				}
				losses[*s.GapTo] = max(losses[*s.GapTo], int64(b.Sub(a).Seconds()))
			}
		}
		if !s.To.After(start) || !s.From.Before(end) {
			continue
		}
		if s.To.After(end) || s.From.Before(start) {
			if !s.Baseline {
				out.unallocated = add(out.unallocated, add(s.RX, s.TX))
			}
			out.uncertain = true
			continue
		}
		out.rx = add(out.rx, s.RX)
		out.tx = add(out.tx, s.TX)
		if s.GapFrom == nil {
			losses[s.To] = max(losses[s.To], s.Lost)
		}
		if s.Minor > 0 && s.GapFrom == nil {
			restarts[s.To] = true
		}
		out.uncertain = out.uncertain || s.Uncertain
		if t, ok := first[s.Name]; !ok || s.From.Before(t) {
			first[s.Name] = s.From
		}
		if t, ok := last[s.Name]; !ok || s.To.After(t) {
			last[s.Name] = s.To
		}
		intervals[s.Name] = append(intervals[s.Name], interval{s.From, s.To})
	}
	for _, loss := range losses {
		out.lost += loss
	}
	out.minor = len(restarts)
	// Coverage requires every selected interface. Observed seconds are the
	// intersection of their intervals, not the sum of interface-seconds.
	points := []time.Time{start, end}
	// Merge intervals first; a sweep keeps month-long detail queries linearithmic.
	events := map[time.Time]int{}
	for _, name := range names {
		list := intervals[name]
		sort.Slice(list, func(i, j int) bool { return list[i].a.Before(list[j].a) })
		merged := []interval{}
		for _, v := range list {
			if len(merged) > 0 && !v.a.After(merged[len(merged)-1].b) {
				if v.b.After(merged[len(merged)-1].b) {
					merged[len(merged)-1].b = v.b
				}
			} else {
				merged = append(merged, v)
			}
		}
		for _, v := range merged {
			points = append(points, v.a, v.b)
			events[v.a]++
			events[v.b]--
		}
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Before(points[j]) })
	active := 0
	previous := start
	for _, p := range points {
		if active == len(names) && len(names) > 0 && p.After(previous) {
			out.observed += int64(p.Sub(previous).Seconds())
		}
		active += events[p]
		delete(events, p)
		previous = p
	}
	for _, name := range names {
		t, ok := first[name]
		if !ok {
			out.from = nil
			out.uncertain = true
			return out
		}
		if last[name].Before(end) {
			tail := int64(end.Sub(last[name]).Seconds())
			// Only a proven restart gap on this interface covering this entire
			// tail is exempt, never an unrelated restart elsewhere in the period.
			minorTail := false
			for _, gap := range minorGaps[name] {
				if !gap.a.After(last[name]) && !gap.b.Before(end) {
					minorTail = true
					break
				}
			}
			if !minorTail {
				out.uncertain = true
				out.lost = max(out.lost, tail)
			}
		}
		if out.from == nil || t.After(*out.from) {
			out.from = ptr(t)
		}
	}
	return out
}
func makePeriod(p Plan, start, end, now time.Time, spans []Span, names []string, cals []Calibration) *Period {
	until := now
	if end.Before(until) {
		until = end
	}
	s := summarize(spans, names, start, until)
	result := &Period{Start: start, End: end, ObservedFrom: s.from, Coverage: "partial", Bound: "lower", RX: s.rx, TX: s.tx, Measured: billed(s.rx, s.tx, p.BillingMode), Quota: p.QuotaBytes, Gaps: Gaps{Lost: s.lost, Minor: s.minor, Unallocated: s.unallocated}}
	if s.from != nil && !s.from.After(start.Add(2*time.Minute)) {
		result.Coverage = "complete"
	}
	// Same-stage sampling outages preserve bytes. Only unknowable resets, missing
	// selected interfaces and unallocated boundary intervals reduce confidence.
	key := p.key(start, end, names)
	var adjustmentAt *time.Time
	for i := len(cals) - 1; i >= 0; i-- {
		c := cals[i]
		if c.Key != key {
			continue
		}
		result.LastReconcile = &c.Reconcile
		if c.Adjustment != nil {
			result.Adjustment = c.Adjustment
			result.AdjustmentKind = ptr(c.Kind)
			adjustmentAt = ptr(c.At)
		}
	}
	if result.Coverage == "complete" && !s.uncertain && s.lost == 0 {
		result.Bound = "exact"
	}
	if adjustmentAt != nil {
		after := summarize(spans, names, *adjustmentAt, until)
		if !after.uncertain && after.lost == 0 {
			result.Bound = "estimate"
		}
	}
	result.Billed = result.Measured
	if result.Adjustment != nil {
		result.Billed = nonnegative(add(result.Billed, *result.Adjustment))
	}
	result.UsedRatio = ratio(result.Billed, p.QuotaBytes)
	remaining := sub(p.QuotaBytes, result.Billed)
	if integer(remaining).Sign() >= 0 {
		result.Remaining = &remaining
	} else {
		result.Overage = ptr(sub(result.Billed, p.QuotaBytes))
	}
	if s.observed >= 3*86400 && s.from != nil {
		basisStart := until.Add(-7 * 24 * time.Hour)
		if basisStart.Before(*s.from) {
			basisStart = *s.from
		}
		recent := summarize(spans, names, basisStart, until)
		days := float64(recent.observed) / 86400
		if days >= 3 {
			rate := new(big.Rat).SetFrac(integer(billed(recent.rx, recent.tx, p.BillingMode)), big.NewInt(recent.observed))
			seconds := max(0, int64(end.Sub(until).Seconds()))
			rate.Mul(rate, new(big.Rat).SetInt64(seconds))
			extra := new(big.Int).Quo(rate.Num(), rate.Denom()).String()
			projected := add(result.Billed, extra)
			result.Projection = &Projection{Billed: projected, Ratio: ratio(projected, p.QuotaBytes), BasisDays: days}
		}
	}
	return result
}
func calibrate(p Plan, period *Period, amount, mode string, at time.Time, names []string) Calibration {
	c := Calibration{Reconcile: Reconcile{At: at, Provider: amount, Measured: period.Measured}, Kind: "reconcile", PeriodStart: period.Start, Key: p.key(period.Start, period.End, names)}
	if integer(period.Measured).Sign() > 0 {
		c.Deviation = ptr(ratio(sub(amount, period.Measured), period.Measured))
	}
	if mode == "override" {
		c.Kind = "override"
		c.Adjustment = ptr(sub(amount, period.Measured))
	} else if period.Coverage == "partial" && period.Adjustment == nil {
		c.Kind = "fill"
		c.Adjustment = ptr(nonnegative(sub(amount, period.Measured)))
	}
	return c
}
func unitHint(p *Plan, cals []Calibration) *UnitHint {
	if p == nil {
		return nil
	}
	deviations := []float64{}
	for _, c := range cals {
		if c.Deviation != nil {
			deviations = append(deviations, *c.Deviation)
		}
		if len(deviations) == 2 {
			break
		}
	}
	if len(deviations) < 2 || math.Abs(deviations[0]-deviations[1]) > 0.015 {
		return nil
	}
	power := 3.
	if p.QuotaUnit == "TB" {
		power = 4
	}
	factor := math.Pow(1024./1000, power)
	expected := factor - 1
	suggested := 1000
	if p.UnitBase == 1000 {
		expected = 1/factor - 1
		suggested = 1024
	}
	if math.Abs(deviations[0]-expected) < 0.02 && math.Abs(deviations[1]-expected) < 0.02 {
		return &UnitHint{Base: suggested, Samples: 2}
	}
	return nil
}
