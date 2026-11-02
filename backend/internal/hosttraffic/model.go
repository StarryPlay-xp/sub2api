// Package hosttraffic measures this Linux host's network counters. It never
// treats a Docker network namespace as the host and never contacts a provider.
package hosttraffic

import (
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"time"
	_ "time/tzdata"

	apperrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrConflict    = apperrors.Conflict("HOST_TRAFFIC_PLAN_CONFLICT", "Plan changed; refresh before saving")
	ErrNotReady    = apperrors.Conflict("HOST_TRAFFIC_NOT_READY", "A fresh reading of the selected interfaces is required")
	ErrNoPlan      = apperrors.Conflict("HOST_TRAFFIC_NO_PLAN", "Configure a traffic plan first")
	ErrCalibration = apperrors.BadRequest("HOST_TRAFFIC_CALIBRATION_INVALID", "Invalid calibration")
)

func invalid(field string) error {
	return apperrors.BadRequest("HOST_TRAFFIC_PLAN_INVALID", "Invalid traffic plan").WithMetadata(map[string]string{"field": field})
}

type PlanAlerts struct {
	Enabled    bool    `json:"enabled"`
	Warning    float64 `json:"warning_percent"`
	Critical   float64 `json:"critical_percent"`
	Projection bool    `json:"projection"`
}
type Plan struct {
	Version     string     `json:"version"`
	QuotaValue  string     `json:"quota_value"`
	QuotaUnit   string     `json:"quota_unit"`
	QuotaBytes  string     `json:"quota_bytes"`
	BillingMode string     `json:"billing_mode"`
	ResetDay    int        `json:"reset_day"`
	ResetTime   string     `json:"reset_time"`
	Timezone    string     `json:"timezone"`
	UnitBase    int        `json:"unit_base"`
	Interfaces  []string   `json:"interfaces"`
	Alerts      PlanAlerts `json:"alerts"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
type Amount struct {
	Value string `json:"value"`
	Unit  string `json:"unit"`
}
type PlanInput struct {
	Version     *string    `json:"version"`
	QuotaValue  string     `json:"quota_value"`
	QuotaUnit   string     `json:"quota_unit"`
	BillingMode string     `json:"billing_mode"`
	ResetDay    int        `json:"reset_day"`
	ResetTime   string     `json:"reset_time"`
	Timezone    string     `json:"timezone"`
	UnitBase    int        `json:"unit_base"`
	Interfaces  []string   `json:"interfaces"`
	Alerts      PlanAlerts `json:"alerts"`
	CurrentUsed *Amount    `json:"current_used"`
}
type CalibrationInput struct {
	Value       string `json:"value"`
	Unit        string `json:"unit"`
	Mode        string `json:"mode"`
	PlanVersion string `json:"plan_version"`
}
type Routes struct {
	IPv4 bool `json:"ipv4"`
	IPv6 bool `json:"ipv6"`
}
type Interface struct {
	Name         string   `json:"name"`
	Selected     bool     `json:"selected"`
	Physical     *bool    `json:"physical"`
	DefaultRoute *Routes  `json:"default_route"`
	RXRate       *float64 `json:"rx_bytes_per_second"`
	TXRate       *float64 `json:"tx_bytes_per_second"`
}
type Selection struct {
	Mode      string `json:"mode"`
	Ambiguous bool   `json:"ambiguous"`
}
type Collection struct {
	State        string     `json:"state"`
	Deployment   string     `json:"deployment"`
	Source       *string    `json:"source"`
	StartedAt    *time.Time `json:"started_at"`
	LastSampleAt *time.Time `json:"last_sample_at"`
	HostBootAt   *time.Time `json:"host_boot_at"`
	ErrorReason  *string    `json:"error_reason"`
}
type Totals struct {
	From  time.Time `json:"from"`
	RX    string    `json:"rx_bytes"`
	TX    string    `json:"tx_bytes"`
	Total string    `json:"total_bytes"`
	Bound string    `json:"bound"`
}
type Gaps struct {
	Lost        int64  `json:"lost_seconds"`
	Minor       int    `json:"minor_restarts"`
	Unallocated string `json:"unallocated_bytes"`
}
type Projection struct {
	Billed    string  `json:"billed_bytes"`
	Ratio     float64 `json:"ratio"`
	BasisDays float64 `json:"basis_days"`
}
type Reconcile struct {
	At        time.Time `json:"at"`
	Provider  string    `json:"provider_bytes"`
	Measured  string    `json:"measured_bytes"`
	Deviation *float64  `json:"deviation_ratio"`
}
type Period struct {
	Start          time.Time   `json:"start"`
	End            time.Time   `json:"end"`
	ObservedFrom   *time.Time  `json:"observed_from"`
	Coverage       string      `json:"coverage"`
	Bound          string      `json:"bound"`
	RX             string      `json:"rx_bytes"`
	TX             string      `json:"tx_bytes"`
	Measured       string      `json:"measured_billed_bytes"`
	Adjustment     *string     `json:"adjustment_bytes"`
	AdjustmentKind *string     `json:"adjustment_kind"`
	Billed         string      `json:"billed_bytes"`
	Quota          string      `json:"quota_bytes"`
	Remaining      *string     `json:"remaining_bytes"`
	Overage        *string     `json:"overage_bytes"`
	UsedRatio      float64     `json:"used_ratio"`
	Gaps           Gaps        `json:"gaps"`
	Projection     *Projection `json:"projection"`
	LastReconcile  *Reconcile  `json:"last_reconcile"`
}
type Firing struct {
	Level string    `json:"level"`
	Kind  string    `json:"kind"`
	Since time.Time `json:"since"`
}
type Alerts struct {
	EmailEnabled bool     `json:"email_enabled"`
	Firing       []Firing `json:"firing"`
}
type UnitHint struct {
	Base    int `json:"suggested_base"`
	Samples int `json:"samples"`
}
type Status struct {
	AsOf       time.Time   `json:"as_of"`
	Collection Collection  `json:"collection"`
	Interfaces []Interface `json:"interfaces"`
	Selection  Selection   `json:"interface_selection"`
	SinceStart *Totals     `json:"since_start"`
	Plan       *Plan       `json:"plan"`
	Period     *Period     `json:"period"`
	Alerts     Alerts      `json:"alerts"`
	UnitHint   *UnitHint   `json:"unit_hint"`
}
type Calibration struct {
	ID int64 `json:"id"`
	Reconcile
	Kind        string    `json:"kind"`
	PeriodStart time.Time `json:"period_start"`
	Key         string    `json:"-"`
	Adjustment  *string   `json:"-"`
}
type Day struct {
	Date     string    `json:"date"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	RX       string    `json:"rx_bytes"`
	TX       string    `json:"tx_bytes"`
	Billed   string    `json:"billed_bytes"`
	Observed int64     `json:"observed_seconds"`
	Expected int64     `json:"expected_seconds"`
}
type ClosedPeriod struct {
	Period
	BillingMode string    `json:"billing_mode"`
	ClosedAt    time.Time `json:"closed_at"`
}
type Detail struct {
	Timezone     string         `json:"days_timezone"`
	Days         []Day          `json:"days"`
	Periods      []ClosedPeriod `json:"periods"`
	Calibrations []Calibration  `json:"calibrations"`
}
type Window struct {
	Range        string    `json:"range"`
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
	AsOf         time.Time `json:"as_of"`
	RetainedFrom time.Time `json:"retained_from"`
}
type Usage struct {
	Window     Window   `json:"window"`
	State      string   `json:"state"`
	Interfaces []string `json:"interfaces"`
	HasRecords bool     `json:"has_records"`
	RX         string   `json:"rx_bytes"`
	TX         string   `json:"tx_bytes"`
	Total      string   `json:"total_bytes"`
	Observed   int64    `json:"observed_seconds"`
	Expected   int64    `json:"expected_seconds"`
}

var decimal = regexp.MustCompile(`^(0|[1-9][0-9]{0,20})(\.[0-9]{1,9})?$`)
var interfaceName = regexp.MustCompile(`^[a-zA-Z0-9_.:-]{1,15}$`)

func Bytes(value, unit string, base int) (string, error) {
	if !decimal.MatchString(value) || (base != 1000 && base != 1024) {
		return "", ErrCalibration
	}
	power := map[string]int{"MB": 2, "GB": 3, "TB": 4}[unit]
	if power == 0 {
		return "", ErrCalibration
	}
	n, _ := new(big.Rat).SetString(value)
	factor := new(big.Int).Exp(big.NewInt(int64(base)), big.NewInt(int64(power)), nil)
	n.Mul(n, new(big.Rat).SetInt(factor))
	result := new(big.Int).Quo(n.Num(), n.Denom())
	if result.BitLen() > 63 {
		return "", ErrCalibration
	}
	return result.String(), nil
}
func (in PlanInput) Validate() (*Plan, error) {
	if in.UnitBase != 1000 && in.UnitBase != 1024 {
		return nil, invalid("unit_base")
	}
	q, e := Bytes(in.QuotaValue, in.QuotaUnit, in.UnitBase)
	if e != nil || q == "0" || in.QuotaUnit == "MB" {
		return nil, invalid("quota")
	}
	switch in.BillingMode {
	case "both", "inbound", "outbound", "max":
	default:
		return nil, invalid("billing_mode")
	}
	if in.ResetDay < 1 || in.ResetDay > 31 {
		return nil, invalid("reset_day")
	}
	t, e := time.Parse("15:04", in.ResetTime)
	if e != nil || t.Minute()%15 != 0 {
		return nil, invalid("reset_time")
	}
	if in.Timezone == "" || in.Timezone == "Local" {
		return nil, invalid("timezone")
	}
	if _, e = time.LoadLocation(in.Timezone); e != nil {
		return nil, invalid("timezone")
	}
	if len(in.Interfaces) > 16 {
		return nil, invalid("interfaces")
	}
	seen := map[string]bool{}
	for _, n := range in.Interfaces {
		if !interfaceName.MatchString(n) || excluded(n) || seen[n] {
			return nil, invalid("interfaces")
		}
		seen[n] = true
	}
	if !(in.Alerts.Warning > 0 && in.Alerts.Warning < in.Alerts.Critical && in.Alerts.Critical <= 1000) {
		return nil, invalid("alerts")
	}
	if in.CurrentUsed != nil {
		if _, e = Bytes(in.CurrentUsed.Value, in.CurrentUsed.Unit, in.UnitBase); e != nil {
			return nil, invalid("current_used")
		}
	}
	names := append([]string{}, in.Interfaces...)
	sort.Strings(names)
	return &Plan{QuotaValue: in.QuotaValue, QuotaUnit: in.QuotaUnit, QuotaBytes: q, BillingMode: in.BillingMode, ResetDay: in.ResetDay, ResetTime: in.ResetTime, Timezone: in.Timezone, UnitBase: in.UnitBase, Interfaces: names, Alerts: in.Alerts}, nil
}

// Month boundaries are always derived from the original reset day, never from a
// February-clamped boundary. Go's timezone database determines DST transitions.
func (p Plan) boundary(year int, month time.Month) time.Time {
	loc, _ := time.LoadLocation(p.Timezone)
	monthStart := time.Date(year, month, 1, 0, 0, 0, 0, loc)
	last := monthStart.AddDate(0, 1, -1).Day()
	day := min(p.ResetDay, last)
	h, _ := strconv.Atoi(p.ResetTime[:2])
	m, _ := strconv.Atoi(p.ResetTime[3:])
	return time.Date(monthStart.Year(), monthStart.Month(), day, h, m, 0, 0, loc).UTC()
}
func (p Plan) Bounds(now time.Time) (time.Time, time.Time) {
	loc, _ := time.LoadLocation(p.Timezone)
	local := now.In(loc)
	start := p.boundary(local.Year(), local.Month())
	if now.Before(start) {
		return p.boundary(local.Year(), local.Month()-1), start
	}
	return start, p.boundary(local.Year(), local.Month()+1)
}
func integer(s string) *big.Int {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return new(big.Int)
	}
	return n
}
func add(a, b string) string { return new(big.Int).Add(integer(a), integer(b)).String() }
func sub(a, b string) string { return new(big.Int).Sub(integer(a), integer(b)).String() }
func nonnegative(s string) string {
	if integer(s).Sign() < 0 {
		return "0"
	}
	return s
}
func ratio(a, b string) float64 {
	if integer(b).Sign() == 0 {
		return 0
	}
	v, _ := new(big.Rat).SetFrac(integer(a), integer(b)).Float64()
	return v
}
func billed(rx, tx, mode string) string {
	switch mode {
	case "inbound":
		return rx
	case "outbound":
		return tx
	case "max":
		if integer(rx).Cmp(integer(tx)) > 0 {
			return rx
		}
		return tx
	default:
		return add(rx, tx)
	}
}
func ptr[T any](v T) *T { return &v }
func (p Plan) key(start, end time.Time, names []string) string {
	return fmt.Sprintf("%s/%s/%s/%v", start.Format(time.RFC3339), end.Format(time.RFC3339), p.BillingMode, names)
}
