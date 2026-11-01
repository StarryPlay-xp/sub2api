package service

import (
	"context"
	"math"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
)

var (
	ErrBandwidthLimitInvalid  = infraerrors.BadRequest("BANDWIDTH_LIMIT_INVALID", "Invalid bandwidth limit or version")
	ErrBandwidthLimitConflict = infraerrors.Conflict("BANDWIDTH_LIMIT_CONFLICT", "Bandwidth limit changed; refresh before saving")
	ErrBandwidthNodeNotFound  = infraerrors.NotFound("BANDWIDTH_NODE_NOT_FOUND", "WireGuard node not found")
)

// Bandwidth limits are shared display settings, not traffic shaping controls.
// Empty version means no saved value; clearing a value retains a new version.
type BandwidthLimit struct {
	Mbps    *float64 `json:"mbps"`
	Version string   `json:"version"`
}

type BandwidthLimits struct {
	Host  BandwidthLimit            `json:"host"`
	Nodes map[string]BandwidthLimit `json:"nodes"`
}

type BandwidthLimitRepository interface {
	ReadBandwidthLimits(context.Context) (BandwidthLimits, error)
	UpdateBandwidthLimit(ctx context.Context, nodeID int64, expectedVersion string, mbps *float64) (BandwidthLimit, error)
}

func ValidateBandwidthLimit(nodeID int64, version string, mbps *float64) error {
	if nodeID < 0 || (mbps != nil && (math.IsNaN(*mbps) || math.IsInf(*mbps, 0) || *mbps <= 0 || *mbps > 1_000_000)) {
		return ErrBandwidthLimitInvalid
	}
	if version != "" {
		if _, err := uuid.Parse(version); err != nil {
			return ErrBandwidthLimitInvalid
		}
	}
	return nil
}
