package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
)

type ServiceTrafficRepository interface {
	Report(context.Context, wgtunnel.TrafficWindow) (ServiceTrafficReport, error)
}
type ServiceTrafficService struct {
	Collector     *traffic.Collector
	Store         ServiceTrafficRepository
	Limits        BandwidthLimitRepository
	Subjects      TrafficSubjectRepository
	identityCache trafficIdentityCache
}

func (s *ServiceTrafficService) Close() {
	if s != nil {
		s.Collector.Close()
	}
}

type ServiceTrafficReport struct {
	wgtunnel.TrafficWindow
	MetricBasis        string           `json:"metric_basis"`
	Scope              string           `json:"scope"`
	PeakWindowSeconds  int              `json:"peak_window_seconds"`
	Bytes              traffic.Bytes    `json:"bytes"`
	InBytes            string           `json:"in_bytes"`
	OutBytes           string           `json:"out_bytes"`
	TotalBytes         string           `json:"total_bytes"`
	Peaks              traffic.Peaks    `json:"peaks"`
	Quality            traffic.Quality  `json:"quality"`
	History            []traffic.Bucket `json:"history"`
	HasRecords         bool             `json:"has_records"`
	Incomplete         bool             `json:"incomplete"`
	ConcurrentRuntimes bool             `json:"concurrent_runtimes"`
	SavedAt            *time.Time       `json:"saved_at"`
}
