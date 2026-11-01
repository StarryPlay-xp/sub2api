package wgtunnel

import (
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"time"
)

const TrafficRetention = 90 * 24 * time.Hour
const MinuteTrafficRetention = 7 * 24 * time.Hour

type TrafficQuery struct {
	Range    string
	Interval string
	Start    string
	End      string
}

type TrafficQueryError struct {
	Reason  string
	Message string
}

func (e *TrafficQueryError) Error() string { return e.Message }

func trafficQueryError(reason, message string) error {
	return &TrafficQueryError{Reason: reason, Message: message}
}

// TrafficWindow is half-open [Start, End). Bucket-aligned bounds avoid silently
// billing a whole hour for a partly selected hour. Presets omit the oldest
// partial bucket; the current bucket contains only already persisted samples.
type TrafficWindow struct {
	Range          string    `json:"range"`
	Interval       string    `json:"interval"`
	Start          time.Time `json:"start"`
	End            time.Time `json:"end"`
	RequestedStart time.Time `json:"requested_start"`
	RequestedEnd   time.Time `json:"requested_end"`
	RetainedFrom   time.Time `json:"retained_from"`
	AsOf           time.Time `json:"as_of"`
	EndClamped     bool      `json:"end_clamped"`
}

func ceilBucket(at time.Time, size time.Duration) time.Time {
	floor := at.UTC().Truncate(size)
	if floor.Before(at) {
		return floor.Add(size)
	}
	return floor
}

func ResolveTrafficWindow(query TrafficQuery, now time.Time) (TrafficWindow, error) {
	now = now.UTC()
	w := TrafficWindow{Range: query.Range, Interval: query.Interval, AsOf: now, End: now, RequestedEnd: now}
	if w.Range == "" {
		w.Range = "24h"
	}
	if w.Interval == "" {
		w.Interval = "hour"
		if w.Range == "24h" {
			w.Interval = "minute"
		}
	}
	grain, retention := time.Hour, TrafficRetention
	if w.Interval == "minute" {
		grain, retention = time.Minute, MinuteTrafficRetention
	} else if w.Interval != "hour" {
		return w, trafficQueryError("TRAFFIC_INTERVAL_INVALID", "interval must be minute or hour")
	}
	w.RetainedFrom = ceilBucket(now.Add(-retention), grain)
	if w.Range == "custom" {
		var err error
		w.Start, err = time.Parse(time.RFC3339Nano, query.Start)
		if err != nil {
			return w, trafficQueryError("TRAFFIC_DATE_INVALID", "custom start must be an RFC3339 timestamp with a timezone")
		}
		w.Start = w.Start.UTC()
		w.RequestedStart = w.Start
		if query.End != "" {
			w.RequestedEnd, err = time.Parse(time.RFC3339Nano, query.End)
			if err != nil {
				return w, trafficQueryError("TRAFFIC_DATE_INVALID", "end must be an RFC3339 timestamp with a timezone")
			}
			w.RequestedEnd = w.RequestedEnd.UTC()
			if w.RequestedEnd.After(now.Add(25 * time.Hour)) {
				return w, trafficQueryError("TRAFFIC_DATE_INVALID", "end cannot be more than 25 hours in the future")
			}
			w.End = w.RequestedEnd
			if w.End.After(now) {
				w.End, w.EndClamped = now, true
			}
		}
		if !w.Start.Equal(w.Start.Truncate(grain)) || (w.End.Before(now) && !w.End.Equal(w.End.Truncate(grain))) {
			return w, trafficQueryError("TRAFFIC_BOUNDARY_UNALIGNED", fmt.Sprintf("custom bounds must align to UTC %s buckets; omit end to query through now", w.Interval))
		}
	} else {
		if query.Start != "" || query.End != "" {
			return w, trafficQueryError("TRAFFIC_RANGE_INVALID", "start and end require range=custom")
		}
		durations := map[string]time.Duration{"24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour, "90d": TrafficRetention}
		duration, ok := durations[w.Range]
		if !ok {
			return w, trafficQueryError("TRAFFIC_RANGE_INVALID", "range must be 24h, 7d, 30d, 90d or custom")
		}
		w.RequestedStart = now.Add(-duration)
		w.Start = ceilBucket(w.RequestedStart, grain)
	}
	if !w.Start.Before(w.End) {
		return w, trafficQueryError("TRAFFIC_RANGE_INVALID", "start must be before end and before now")
	}
	if w.Start.Before(w.RetainedFrom) {
		return w, trafficQueryError("TRAFFIC_RETENTION_EXCEEDED", fmt.Sprintf("%s data is available from %s", w.Interval, w.RetainedFrom.Format(time.RFC3339)))
	}
	return w, nil
}

type TrafficReport struct {
	PeakHistory       *traffic.PeakHistory `json:"peak_history"`
	PeakWindowSeconds int                  `json:"peak_window_seconds"`
	TrafficWindow
	TX            string     `json:"tx_bytes"`
	RX            string     `json:"rx_bytes"`
	Total         string     `json:"total_bytes"`
	HasRecords    bool       `json:"has_records"`
	BucketCount   int        `json:"bucket_count"`
	FirstRecordAt *time.Time `json:"first_record_at,omitempty"`
	LastRecordAt  *time.Time `json:"last_record_at,omitempty"`
	SavedAt       *time.Time `json:"saved_at,omitempty"`
	Incomplete    bool       `json:"incomplete"`
	MetricBasis   string     `json:"metric_basis"`
	History       []Bucket   `json:"history"`
}
