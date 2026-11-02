package hosttraffic

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func mockStore(t *testing.T) (*Store, sqlmock.Sqlmock) {
	db, m, e := sqlmock.New()
	require.NoError(t, e)
	t.Cleanup(func() { require.NoError(t, m.ExpectationsWereMet()); _ = db.Close() })
	return &Store{DB: db}, m
}
func stateRow(m sqlmock.Sqlmock, state persisted, write bool) {
	raw, _ := json.Marshal(state)
	m.ExpectBegin()
	q := "SELECT data FROM host_traffic_state WHERE id=1"
	if write {
		q += " FOR UPDATE"
	}
	m.ExpectQuery(q).WillReturnRows(sqlmock.NewRows([]string{"data"}).AddRow(raw))
}
func emptyHistory(m sqlmock.Sqlmock) {
	m.ExpectQuery("SELECT spans FROM host_traffic_hours").WillReturnRows(sqlmock.NewRows([]string{"spans"}))
	m.ExpectQuery("SELECT id,accounting_key,adjustment,data").WillReturnRows(sqlmock.NewRows([]string{"id", "accounting_key", "adjustment", "data"}))
	m.ExpectQuery("SELECT data FROM host_traffic_periods").WillReturnRows(sqlmock.NewRows([]string{"data"}))
}
func TestHostTrafficPlanConflictAndAtomicNotReady(t *testing.T) {
	now := instant("2026-11-02T00:00:00Z")
	ctx := context.Background()
	for _, tc := range []struct {
		name    string
		old     *Plan
		version *string
	}{
		{"creation conflict", testPlan(t), nil}, {"update conflict", testPlan(t), ptr("different")}, {"missing existing plan", nil, ptr("old")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, m := mockStore(t)
			stateRow(m, persisted{Plan: tc.old}, true)
			m.ExpectRollback()
			in := planInput()
			in.Version = tc.version
			require.ErrorIs(t, s.UpdatePlan(ctx, in, now), ErrConflict)
		})
	}
	t.Run("current used rolls back plan and rules", func(t *testing.T) {
		s, m := mockStore(t)
		stateRow(m, persisted{}, true)
		emptyHistory(m)
		m.ExpectRollback()
		in := planInput()
		in.CurrentUsed = &Amount{Value: "1", Unit: "GB"}
		require.ErrorIs(t, s.UpdatePlan(ctx, in, now), ErrNotReady)
	})
	t.Run("successful creation manages three rules in transaction", func(t *testing.T) {
		s, m := mockStore(t)
		stateRow(m, persisted{}, true)
		emptyHistory(m)
		for i := 0; i < 3; i++ {
			m.ExpectExec("INSERT INTO ops_alert_rules").WillReturnResult(sqlmock.NewResult(1, 1))
		}
		m.ExpectExec("UPDATE ops_alert_events").WillReturnResult(sqlmock.NewResult(0, 0))
		m.ExpectExec("UPDATE host_traffic_state").WillReturnResult(sqlmock.NewResult(0, 1))
		m.ExpectCommit()
		require.NoError(t, s.UpdatePlan(ctx, planInput(), now))
	})
}
func TestHostTrafficDuplicateAndOrderedRetry(t *testing.T) {
	s, m := mockStore(t)
	now := instant("2026-11-02T01:01:00Z")
	state := persisted{Last: &now}
	stateRow(m, state, true)
	// A repeated minute writes no raw readings or differences.
	m.ExpectExec("UPDATE host_traffic_state").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectCommit()
	require.NoError(t, s.Save(context.Background(), Sample{At: now, Readings: []Reading{{Name: "eth0", RX: "999", TX: "999"}}}))
	old := Reading{Name: "eth0", Boot: "boot", Index: 2, BootAt: now.Add(-time.Hour), At: now, RX: "100", TX: "200"}
	state.Latest = map[string]Reading{"eth0": old}
	state.Started = &now
	stateRow(m, state, true)
	m.ExpectExec("INSERT INTO host_traffic_samples").WillReturnResult(sqlmock.NewResult(1, 1))
	m.ExpectExec("INSERT INTO host_traffic_hours").WillReturnResult(sqlmock.NewResult(1, 1))
	m.ExpectExec("UPDATE host_traffic_state").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectCommit()
	cur := old
	cur.At = now.Add(time.Minute)
	cur.RX = "150"
	cur.TX = "250"
	require.NoError(t, s.Save(context.Background(), Sample{At: cur.At, Readings: []Reading{cur}}))
}
func TestHostTrafficCalibrationIDsAndMetric(t *testing.T) {
	now := instant("2026-11-02T01:01:00Z")
	p := testPlan(t)
	start, end := p.Bounds(now)
	reading := Reading{Name: "eth0", At: now, RX: "50", TX: "50"}
	state := persisted{Plan: p, Started: &start, Last: &now, Latest: map[string]Reading{"eth0": reading}}
	span := Span{Name: "eth0", From: start, To: now, RX: "500", TX: "500"}
	rawSpans, _ := json.Marshal([]Span{span})
	cal := Calibration{Reconcile: Reconcile{At: now, Provider: "1000", Measured: "1000"}, Kind: "reconcile", PeriodStart: start}
	rawCal, _ := json.Marshal(cal)
	s, m := mockStore(t)
	stateRow(m, state, false)
	m.ExpectQuery("SELECT spans").WillReturnRows(sqlmock.NewRows([]string{"spans"}).AddRow(rawSpans))
	m.ExpectQuery("SELECT id,accounting_key").WillReturnRows(sqlmock.NewRows([]string{"id", "accounting_key", "adjustment", "data"}).AddRow(42, p.key(start, end, []string{"eth0"}), nil, rawCal))
	m.ExpectQuery("SELECT data FROM host_traffic_periods").WillReturnRows(sqlmock.NewRows([]string{"data"}))
	m.ExpectCommit()
	detail, e := s.Detail(context.Background(), now)
	require.NoError(t, e)
	require.EqualValues(t, 42, detail.Calibrations[0].ID)
	for _, metric := range []string{"host_traffic_quota_used_percent", "host_traffic_quota_projected_percent"} {
		stateRow(m, state, false)
		m.ExpectQuery("SELECT spans").WillReturnRows(sqlmock.NewRows([]string{"spans"}).AddRow(rawSpans))
		m.ExpectQuery("SELECT id,accounting_key").WillReturnRows(sqlmock.NewRows([]string{"id", "accounting_key", "adjustment", "data"}))
		m.ExpectQuery("SELECT data FROM host_traffic_periods").WillReturnRows(sqlmock.NewRows([]string{"data"}))
		m.ExpectQuery("SELECT CASE WHEN e.severity").WillReturnRows(sqlmock.NewRows([]string{"severity", "kind", "fired_at"}))
		m.ExpectCommit()
		value, ok := s.Metric(context.Background(), metric, now, Collection{State: "ok"})
		require.True(t, ok)
		if metric == "host_traffic_quota_used_percent" {
			require.InDelta(t, 1000./1500000000000*100, value, 1e-15)
		} else {
			require.Zero(t, value)
		}
	}
}
func TestHostTrafficSampleFreshness(t *testing.T) {
	now := time.Now().UTC()
	snap := snapshot{state: persisted{Last: ptr(now.Add(-3 * time.Minute)), Latest: map[string]Reading{"eth0": {At: now.Add(-3 * time.Minute)}}}}
	require.True(t, ready(&snap, []string{"eth0"}, now))
	require.False(t, ready(&snap, []string{"eth0", "missing"}, now))
	require.False(t, ready(&snap, []string{"eth0"}, now.Add(time.Second)))
}

func TestHostTrafficClosedPeriodIsImmutable(t *testing.T) {
	s, m := mockStore(t)
	ctx := context.Background()
	p := testPlan(t)
	start := instant("2026-10-01T00:00:00Z")
	end := instant("2026-11-01T00:00:00Z")
	stateRow(m, persisted{Plan: p, ActiveStart: &start, Last: &end}, true)
	// A repeated freeze must not update an existing closed record.
	m.ExpectExec("INSERT INTO host_traffic_periods VALUES.*ON CONFLICT DO NOTHING").WithArgs(start, end, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	m.ExpectExec("UPDATE host_traffic_state").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectCommit()
	require.NoError(t, s.transaction(ctx, true, func(tx *sql.Tx, snap *snapshot) error { return closePeriods(ctx, tx, snap, end) }))
}
