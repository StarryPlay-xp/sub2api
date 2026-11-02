package hosttraffic

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func instant(s string) time.Time {
	t, e := time.Parse(time.RFC3339, s)
	if e != nil {
		panic(e)
	}
	return t
}
func planInput() PlanInput {
	return PlanInput{QuotaValue: "1.5", QuotaUnit: "TB", BillingMode: "both", ResetDay: 1, ResetTime: "00:00", Timezone: "UTC", UnitBase: 1000, Interfaces: []string{"eth0"}, Alerts: PlanAlerts{Enabled: true, Warning: 80, Critical: 95, Projection: true}}
}
func testPlan(t *testing.T) *Plan { p, e := planInput().Validate(); require.NoError(t, e); return p }
func TestHostTrafficCounterStages(t *testing.T) {
	at := instant("2026-11-01T12:00:00Z")
	old := Reading{Name: "eth0", Index: 2, Boot: "a", BootAt: at.Add(-time.Hour), At: at, RX: "100", TX: "200"}
	cur := old
	cur.At = at.Add(time.Minute)
	cur.RX = "150"
	cur.TX = "280"
	s := difference(&old, cur)
	require.Equal(t, "50", s.RX)
	require.Equal(t, "80", s.TX)
	require.Zero(t, s.Lost)
	// Restart of the application, including a long collection outage, keeps stage.
	cur.At = at.Add(3 * time.Hour)
	s = difference(&old, cur)
	require.Equal(t, "50", s.RX)
	require.Zero(t, s.Lost)
	cur.Boot = "b"
	cur.BootAt = at.Add(80 * time.Second)
	s = difference(&old, cur)
	require.Equal(t, 1, s.Minor)
	require.Zero(t, s.Lost)
	require.Equal(t, "150", s.RX)
	cur.BootAt = at.Add(180 * time.Second)
	s = difference(&old, cur)
	require.EqualValues(t, 180, s.Lost)
	require.Zero(t, s.Minor)
	cur.Boot = "a"
	cur.Index = 3
	s = difference(&old, cur)
	require.True(t, s.Uncertain)
	require.Positive(t, s.Lost)
	cur.Index = 2
	cur.RX = "2"
	s = difference(&old, cur)
	require.Equal(t, "2", s.RX)
	require.True(t, s.Uncertain)
}
func TestHostTrafficPeriodBoundaries(t *testing.T) {
	p := testPlan(t)
	p.ResetDay = 31
	start, end := p.Bounds(instant("2026-02-28T01:00:00Z"))
	require.Equal(t, instant("2026-02-28T00:00:00Z"), start)
	require.Equal(t, instant("2026-03-31T00:00:00Z"), end)
	p.Timezone = "Asia/Kathmandu"
	p.ResetTime = "00:15"
	start, end = p.Bounds(instant("2026-03-01T00:00:00Z"))
	require.Equal(t, instant("2026-02-27T18:30:00Z"), start)
	require.Equal(t, instant("2026-03-30T18:30:00Z"), end)
	p.ResetDay = 1
	p.Timezone = "America/New_York"
	p.ResetTime = "00:00"
	start, end = p.Bounds(instant("2026-03-10T00:00:00Z"))
	require.Equal(t, instant("2026-03-01T05:00:00Z"), start)
	require.Equal(t, instant("2026-04-01T04:00:00Z"), end)
	in := planInput()
	in.ResetTime = "00:14"
	_, e := in.Validate()
	require.Error(t, e)
	in.ResetTime = "00:45"
	_, e = in.Validate()
	require.NoError(t, e)
}
func TestHostTrafficBillingAndBoundaryAllocation(t *testing.T) {
	for mode, want := range map[string]string{"both": "350", "inbound": "100", "outbound": "250", "max": "250"} {
		require.Equal(t, want, billed("100", "250", mode))
	}
	p := testPlan(t)
	start, end := p.Bounds(instant("2026-11-01T00:00:00Z"))
	names := []string{"eth0"}
	spans := []Span{{Name: "eth0", From: start.Add(-time.Minute), To: start, RX: "10", TX: "20"}, {Name: "eth0", From: start, To: start.Add(time.Minute), RX: "30", TX: "40"}}
	period := makePeriod(*p, start, end, start.Add(time.Minute), spans, names, nil)
	require.Equal(t, "70", period.Billed)
	require.Equal(t, "exact", period.Bound)
	require.Equal(t, "0", period.Gaps.Unallocated)
	spans = []Span{{Name: "eth0", From: start.Add(-time.Minute), To: start.Add(time.Minute), RX: "100", TX: "200"}}
	period = makePeriod(*p, start, end, start.Add(time.Minute), spans, names, nil)
	require.Equal(t, "0", period.Billed)
	require.Equal(t, "300", period.Gaps.Unallocated)
	require.Equal(t, "lower", period.Bound)
	// rx/tx max is over totals, not the sum of each minute's maximum.
	p.BillingMode = "max"
	spans = []Span{{Name: "eth0", From: start, To: start.Add(time.Minute), RX: "100", TX: "0"}, {Name: "eth0", From: start.Add(time.Minute), To: start.Add(2 * time.Minute), RX: "0", TX: "100"}}
	period = makePeriod(*p, start, end, start.Add(2*time.Minute), spans, names, nil)
	require.Equal(t, "100", period.Billed)
}
func TestHostTrafficCalibrationAndBounds(t *testing.T) {
	p := testPlan(t)
	start, end := p.Bounds(instant("2026-11-01T00:00:00Z"))
	at := start.Add(24 * time.Hour)
	names := []string{"eth0"}
	spans := []Span{{Name: "eth0", From: at.Add(-time.Minute), To: at, RX: "40", TX: "60"}}
	period := makePeriod(*p, start, end, at, spans, names, nil)
	require.Equal(t, "lower", period.Bound)
	fill := calibrate(*p, period, "500", "auto", at, names)
	require.Equal(t, "fill", fill.Kind)
	require.Equal(t, "400", *fill.Adjustment)
	period = makePeriod(*p, start, end, at, spans, names, []Calibration{fill})
	require.Equal(t, "500", period.Billed)
	require.Equal(t, "estimate", period.Bound)
	reconcile := calibrate(*p, period, "900", "auto", at, names)
	require.Equal(t, "reconcile", reconcile.Kind)
	require.Nil(t, reconcile.Adjustment)
	override := calibrate(*p, period, "50", "override", at, names)
	require.Equal(t, "-50", *override.Adjustment)
	period = makePeriod(*p, start, end, at, spans, names, []Calibration{override, reconcile, fill})
	require.Equal(t, "50", period.Billed)
	again := calibrate(*p, period, "70", "override", at, names)
	period = makePeriod(*p, start, end, at, spans, names, []Calibration{again, override, fill})
	require.Equal(t, "70", period.Billed)
	spans = append(spans, Span{Name: "eth0", From: at, To: at.Add(time.Minute), RX: "2", TX: "3", Lost: 60, Uncertain: true})
	period = makePeriod(*p, start, end, at.Add(time.Minute), spans, names, []Calibration{again, override, fill})
	require.Equal(t, "lower", period.Bound)
	spans = []Span{{Name: "eth0", From: start, To: at, RX: "1", TX: "1", Minor: 1}}
	period = makePeriod(*p, start, end, at, spans, names, nil)
	require.Equal(t, "exact", period.Bound)
	require.Equal(t, "reconcile", calibrate(*p, period, "5", "auto", at, names).Kind)
}
func TestHostTrafficCoveragePredictionAndInterfaceIntersection(t *testing.T) {
	p := testPlan(t)
	start, end := p.Bounds(instant("2026-11-01T00:00:00Z"))
	names := []string{"eth0"}
	spans := []Span{}
	for i := 0; i < 3*24*60; i++ {
		a := start.Add(time.Duration(i) * time.Minute)
		spans = append(spans, Span{Name: "eth0", From: a, To: a.Add(time.Minute), RX: "10", TX: "20"})
	}
	before := makePeriod(*p, start, end, start.Add(72*time.Hour-time.Minute), spans, names, nil)
	require.Nil(t, before.Projection)
	at := start.Add(72 * time.Hour)
	period := makePeriod(*p, start, end, at, spans, names, nil)
	require.NotNil(t, period.Projection)
	require.Equal(t, 3., period.Projection.BasisDays)
	require.Equal(t, "1296000", period.Projection.Billed)
	summary := summarize(spans, []string{"eth0", "missing"}, start, at)
	require.Zero(t, summary.observed)
	require.Nil(t, summary.from)
	// A boot after period start has known boot counters but partial beginning.
	period = makePeriod(*p, start, end, at, []Span{{Name: "eth0", From: start.Add(time.Hour), To: at, RX: "100", TX: "200"}}, names, nil)
	require.Equal(t, "partial", period.Coverage)
	require.Equal(t, "300", period.Billed)
}
func TestHostTrafficDockerNeverFallsBack(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "sys/class/net/eth0"), 0755))
	r := Reader{Root: root, OS: "linux", Deployment: "docker"}
	sample := r.Read(time.Now())
	require.Equal(t, "needs_host_mount", sample.Collection.State)
	require.Empty(t, sample.Readings)
	require.Nil(t, sample.Collection.Source)
	r.OS = "darwin"
	require.Equal(t, "unsupported", r.Read(time.Now()).Collection.State)
}
func TestHostTrafficSelectionAndDecimalBytes(t *testing.T) {
	rs := []Reading{{Name: "eth0", Route: &Routes{IPv4: true, IPv6: true}}, {Name: "eth1"}}
	names, selection := selectInterfaces(rs, nil)
	require.Equal(t, []string{"eth0"}, names)
	require.False(t, selection.Ambiguous)
	rs[0].Route.IPv6 = false
	rs[1].Route = &Routes{IPv6: true}
	names, selection = selectInterfaces(rs, nil)
	require.Empty(t, names)
	require.True(t, selection.Ambiguous)
	n, e := Bytes("1.5", "TB", 1000)
	require.NoError(t, e)
	require.Equal(t, "1500000000000", n)
	n, e = Bytes("1.5", "GB", 1024)
	require.NoError(t, e)
	require.Equal(t, "1610612736", n)
	_, e = Bytes("1e5", "TB", 1000)
	require.Error(t, e)
}

func TestHostTrafficInitialBaselineAndRestartDeduplication(t *testing.T) {
	p := testPlan(t)
	start, end := p.Bounds(instant("2026-11-01T00:00:00Z"))
	at := start.Add(time.Hour)
	// An initial absolute count dating before this period is not a measured
	// interruption. Do not label all earlier-boot traffic as this period's gap.
	oldBoot := difference(nil, Reading{Name: "eth0", BootAt: start.Add(-time.Hour), At: at, RX: "100", TX: "200"})
	period := makePeriod(*p, start, end, at, []Span{oldBoot}, []string{"eth0"}, nil)
	require.Equal(t, "lower", period.Bound)
	require.Equal(t, "0", period.Billed)
	require.Equal(t, "0", period.Gaps.Unallocated)
	spans := []Span{{Name: "eth0", From: start, To: at, RX: "100", TX: "200", Minor: 1}, {Name: "eth1", From: start, To: at, RX: "100", TX: "200", Minor: 1}}
	period = makePeriod(*p, start, end, at, spans, []string{"eth0", "eth1"}, nil)
	require.Equal(t, 1, period.Gaps.Minor)
	require.Equal(t, "exact", period.Bound)
}
func TestHostTrafficUnitHintRequiresStableReconciliations(t *testing.T) {
	p := testPlan(t)
	p.QuotaUnit = "GB"
	p.UnitBase = 1024
	require.Nil(t, unitHint(p, []Calibration{{Reconcile: Reconcile{Deviation: ptr(.0737)}}}))
	hint := unitHint(p, []Calibration{{Reconcile: Reconcile{Deviation: ptr(.0737)}}, {Reconcile: Reconcile{Deviation: ptr(.074)}}})
	require.NotNil(t, hint)
	require.Equal(t, 1000, hint.Base)
	require.Nil(t, unitHint(p, []Calibration{{Reconcile: Reconcile{Deviation: ptr(.0737)}}, {Reconcile: Reconcile{Deviation: ptr(.01)}}}))
}

func TestHostTrafficRebootGapIsClippedToEachPeriod(t *testing.T) {
	p := testPlan(t)
	boundary := instant("2026-11-01T00:00:00Z")
	old := Reading{Name: "eth0", Boot: "old", Index: 2, At: boundary.Add(-10 * time.Minute), RX: "100", TX: "100"}
	cur := Reading{Name: "eth0", Boot: "new", Index: 2, BootAt: boundary.Add(10 * time.Minute), At: boundary.Add(11 * time.Minute), RX: "10", TX: "20"}
	gap := difference(&old, cur)
	priorStart, priorEnd := p.Bounds(old.At)
	prior := makePeriod(*p, priorStart, priorEnd, priorEnd, []Span{{Name: "eth0", From: priorStart, To: old.At, RX: "100", TX: "100"}, gap}, []string{"eth0"}, nil)
	require.Equal(t, "lower", prior.Bound)
	require.EqualValues(t, 600, prior.Gaps.Lost)
	start, end := p.Bounds(cur.At)
	current := makePeriod(*p, start, end, cur.At, []Span{gap}, []string{"eth0"}, nil)
	require.EqualValues(t, 600, current.Gaps.Lost)
	require.Equal(t, "30", current.Billed)
}
func TestHostTrafficLinuxReaderUsesMountedCountersAndMinuteLabels(t *testing.T) {
	root := t.TempDir()
	write := func(path, value string) {
		name := filepath.Join(root, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(name), 0755))
		require.NoError(t, os.WriteFile(name, []byte(value), 0600))
	}
	write("proc/sys/kernel/random/boot_id", "00000000-0000-4000-8000-000000000001\n")
	write("proc/uptime", "3600.00 0.00\n")
	write("host/sys/class/net/eth0/ifindex", "2\n")
	write("host/sys/class/net/eth0/statistics/rx_bytes", "100\n")
	write("host/sys/class/net/eth0/statistics/tx_bytes", "200\n")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "host/sys/class/net/eth0/device"), 0755))
	sample := (Reader{Root: root, OS: "linux", Deployment: "docker"}).Read(time.Now().UTC().Truncate(time.Minute).Add(35 * time.Second))
	require.Equal(t, "ok", sample.Collection.State)
	require.Equal(t, "host_mount", *sample.Collection.Source)
	require.Equal(t, 0, sample.At.Second())
	require.Len(t, sample.Readings, 1)
	require.Equal(t, "100", sample.Readings[0].RX)
	require.Nil(t, sample.Readings[0].Route)
	names, selection := selectInterfaces(sample.Readings, nil)
	require.Equal(t, []string{"eth0"}, names)
	require.False(t, selection.Ambiguous)
}
