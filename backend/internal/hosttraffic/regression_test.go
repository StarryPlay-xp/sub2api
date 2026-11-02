package hosttraffic

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	apperrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type captureJSON struct{ into any }

func (c captureJSON) Match(value driver.Value) bool {
	s, ok := value.(string)
	return ok && json.Unmarshal([]byte(s), c.into) == nil
}

func expectHistory(t *testing.T, m sqlmock.Sqlmock, spans []Span) {
	t.Helper()
	raw, err := json.Marshal(spans)
	require.NoError(t, err)
	m.ExpectQuery("SELECT spans").WillReturnRows(sqlmock.NewRows([]string{"spans"}).AddRow(raw))
	m.ExpectQuery("SELECT id,accounting_key").WillReturnRows(sqlmock.NewRows([]string{"id", "accounting_key", "adjustment", "data"}))
	m.ExpectQuery("SELECT data FROM host_traffic_periods").WillReturnRows(sqlmock.NewRows([]string{"data"}))
}

func TestHostTrafficPlanWaitsForBoundaryBeforeFreezing(t *testing.T) {
	for _, delayed := range []bool{false, true} {
		name := "exact boundary"
		if delayed {
			name = "recovery crosses boundary"
		}
		t.Run(name, func(t *testing.T) {
			s, m := mockStore(t)
			ctx := context.Background()
			p := testPlan(t)
			p.Version = "old-version"
			start := instant("2026-10-01T00:00:00Z")
			end := instant("2026-11-01T00:00:00Z")
			last := end.Add(-time.Minute)
			old := Reading{Name: "eth0", Boot: "test-boot", Index: 2, BootAt: start, At: last, RX: "100", TX: "0"}
			state := persisted{Plan: p, Started: &start, Last: &last, ActiveStart: &start, Latest: map[string]Reading{"eth0": old}}
			spans := []Span{{Name: "eth0", From: start, To: last, RX: "100", TX: "0"}}
			in := planInput()
			in.Version = &p.Version
			in.QuotaValue = "2"
			// Both ordinary edits and current_used must leave all persisted data
			// untouched while a boundary reading is pending (including after restart).
			for _, amount := range []*Amount{nil, {Value: "1", Unit: "GB"}} {
				stateRow(m, state, true)
				expectHistory(t, m, spans)
				m.ExpectRollback()
				in.CurrentUsed = amount
				require.ErrorIs(t, s.UpdatePlan(ctx, in, end.Add(time.Second)), ErrNotReady)
			}
			cur := old
			cur.At = end
			if delayed {
				cur.At = end.Add(time.Minute)
			}
			cur.RX = "300"
			spans = append(spans, difference(&old, cur))
			stateRow(m, state, true)
			m.ExpectExec("INSERT INTO host_traffic_samples").WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec("INSERT INTO host_traffic_hours").WillReturnResult(sqlmock.NewResult(1, 1))
			expectHistory(t, m, spans)
			var frozen ClosedPeriod
			m.ExpectExec("INSERT INTO host_traffic_periods").WithArgs(start, end, captureJSON{&frozen}).WillReturnResult(sqlmock.NewResult(1, 1))
			if !delayed {
				m.ExpectExec("DELETE FROM host_traffic_samples").WillReturnResult(sqlmock.NewResult(0, 0))
				m.ExpectExec("DELETE FROM host_traffic_hours").WillReturnResult(sqlmock.NewResult(0, 0))
			}
			m.ExpectExec("UPDATE host_traffic_state").WithArgs(captureJSON{&state}).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectCommit()
			require.NoError(t, s.Save(ctx, Sample{At: cur.At, Readings: []Reading{cur}}))
			require.Equal(t, p.QuotaBytes, frozen.Quota)
			require.Equal(t, end, *state.ActiveStart)
			if delayed {
				require.Equal(t, "100", frozen.Billed)
				require.Equal(t, "200", frozen.Gaps.Unallocated)
				require.Equal(t, "lower", frozen.Bound)
			} else {
				require.Equal(t, "300", frozen.Billed)
				require.Equal(t, "0", frozen.Gaps.Unallocated)
				require.Equal(t, "exact", frozen.Bound)
			}
			// Retry succeeds under the old version, without rewriting the snapshot.
			in.CurrentUsed = nil
			stateRow(m, state, true)
			expectHistory(t, m, spans)
			for i := 0; i < 3; i++ {
				m.ExpectExec("INSERT INTO ops_alert_rules").WillReturnResult(sqlmock.NewResult(1, 1))
			}
			m.ExpectExec("UPDATE ops_alert_events").WillReturnResult(sqlmock.NewResult(0, 0))
			m.ExpectExec("UPDATE host_traffic_state").WithArgs(captureJSON{&state}).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectCommit()
			require.NoError(t, s.UpdatePlan(ctx, in, cur.At.Add(time.Second)))
			require.Equal(t, "2000000000000", state.Plan.QuotaBytes)
			require.NotEqual(t, "old-version", state.Plan.Version)
		})
	}
}

func TestHostTrafficResetTimeRequiresCanonicalFormat(t *testing.T) {
	for _, value := range []string{"1:15", "0:00", "9:45", "24:00", "01:14", "01:15:00", " 01:15"} {
		t.Run(value, func(t *testing.T) {
			in := planInput()
			in.ResetTime = value
			_, err := in.Validate()
			require.Error(t, err)
			require.Equal(t, 400, apperrors.Code(err))
			require.Equal(t, "HOST_TRAFFIC_PLAN_INVALID", apperrors.Reason(err))
			require.Equal(t, "reset_time", apperrors.FromError(err).Metadata["field"])
		})
	}
	for _, value := range []string{"00:00", "01:15", "09:30", "23:45"} {
		in := planInput()
		in.ResetTime = value
		p, err := in.Validate()
		require.NoError(t, err)
		start, _ := p.Bounds(instant("2026-11-02T00:00:00Z"))
		require.Equal(t, instant("2026-11-01T"+value+":00Z"), start)
	}
	// Legacy noncanonical values may already be persisted. Reading them must
	// not silently interpret 1:15 as 00:05; new writes must use 01:15.
	p := testPlan(t)
	p.ResetTime = "1:15"
	start, _ := p.Bounds(instant("2026-11-02T00:00:00Z"))
	require.Equal(t, instant("2026-11-01T01:15:00Z"), start)
}

func TestHostTrafficFreezeRollbackPreservesEarlierPeriods(t *testing.T) {
	s, m := mockStore(t)
	p := testPlan(t)
	start := instant("2026-10-01T00:00:00Z")
	end := instant("2026-11-01T00:00:00Z")
	now := instant("2026-12-01T00:00:01Z")
	// October can be closed but November's boundary has not been persisted.
	// The entire edit must roll back, including the first snapshot insert.
	stateRow(m, persisted{Plan: p, ActiveStart: &start, Last: &end}, true)
	emptyHistory(m)
	m.ExpectExec("INSERT INTO host_traffic_periods").WithArgs(start, end, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	m.ExpectRollback()
	in := planInput()
	in.Version = &p.Version
	require.ErrorIs(t, s.UpdatePlan(context.Background(), in, now), ErrNotReady)
}

func TestHostTrafficMetricsRequireSelectedCurrentReadings(t *testing.T) {
	start := instant("2026-11-01T00:00:00Z")
	p := testPlan(t)
	spans := []Span{{Name: "eth0", From: start, To: start.Add(3 * 24 * time.Hour), RX: "300000000000", TX: "0"}}
	prior := makePeriod(*p, start, instant("2026-12-01T00:00:00Z"), spans[0].To, spans, p.Interfaces, nil)
	require.NotNil(t, prior.Projection)
	require.Greater(t, prior.Projection.Ratio, 1.0)
	for _, tc := range []struct {
		name          string
		now, last, at time.Time
		wantOK        bool
	}{
		{"missing selected interface", start.Add(11 * 24 * time.Hour), start.Add(11 * 24 * time.Hour), spans[0].To, false},
		{"stale collection", start.Add(10 * time.Minute), start.Add(time.Minute), start.Add(time.Minute), false},
		{"previous period sample", start, start.Add(-time.Minute), start.Add(-time.Minute), false},
		{"new period boundary sample", start, start, start, true},
		{"new period fresh sample", start.Add(time.Minute), start.Add(time.Minute), start.Add(time.Minute), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, metric := range []string{"host_traffic_quota_used_percent", "host_traffic_quota_projected_percent"} {
				s, m := mockStore(t)
				state := persisted{Plan: p, Started: &start, Last: &tc.last, Latest: map[string]Reading{
					"eth0": {Name: "eth0", At: tc.at}, "eth1": {Name: "eth1", At: tc.last},
				}}
				stateRow(m, state, false)
				expectHistory(t, m, spans)
				m.ExpectQuery("SELECT CASE WHEN e.severity").WillReturnRows(sqlmock.NewRows([]string{"severity", "kind", "fired_at"}))
				m.ExpectCommit()
				value, ok := s.Metric(context.Background(), metric, tc.now, Collection{State: "ok"})
				require.Equal(t, tc.wantOK, ok)
				require.Zero(t, value)
			}
		})
	}
}

func TestHostTrafficMinorRestartOnlyExemptsItsOwnTail(t *testing.T) {
	p := testPlan(t)
	start := instant("2026-10-01T00:00:00Z")
	end := instant("2026-11-01T00:00:00Z")
	before := start.Add(time.Hour)
	old := Reading{Name: "eth0", Boot: "old", Index: 2, At: before, RX: "100", TX: "0"}
	cur := Reading{Name: "eth0", Boot: "new", Index: 2, BootAt: before.Add(time.Minute), At: before.Add(2 * time.Minute), RX: "10", TX: "0"}
	spans := []Span{
		{Name: "eth0", From: start, To: before, RX: "100", TX: "0"}, difference(&old, cur),
		{Name: "eth0", From: cur.At, To: end.Add(-time.Minute), RX: "100", TX: "0"},
		{Name: "eth1", From: start, To: end, RX: "100", TX: "0"},
	}
	got := makePeriod(*p, start, end, end, spans, []string{"eth0", "eth1"}, nil)
	require.Equal(t, 1, got.Gaps.Minor)
	require.EqualValues(t, 60, got.Gaps.Lost)
	require.Equal(t, "lower", got.Bound)
	// The reboot is now on eth1. It cannot excuse eth0's missing tail either.
	for i := range spans {
		if i < 3 {
			spans[i].Name = "eth1"
		} else {
			spans[i].Name = "eth0"
		}
	}
	spans[2].To = end
	spans[3].To = end.Add(-time.Minute)
	got = makePeriod(*p, start, end, end, spans, []string{"eth0", "eth1"}, nil)
	require.Equal(t, "lower", got.Bound)
	require.EqualValues(t, 60, got.Gaps.Lost)
	// A proven short reboot actually spanning the boundary remains exempt.
	old.At = end.Add(-time.Minute)
	cur.BootAt = end.Add(30 * time.Second)
	cur.At = end.Add(time.Minute)
	spans = []Span{{Name: "eth0", From: start, To: old.At, RX: "100", TX: "0"}, difference(&old, cur)}
	got = makePeriod(*p, start, end, end, spans, []string{"eth0"}, nil)
	require.Equal(t, "exact", got.Bound)
	require.Zero(t, got.Gaps.Lost)
	require.Equal(t, 1, got.Gaps.Minor)
}
