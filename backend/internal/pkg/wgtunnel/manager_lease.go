package wgtunnel

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

// PostgreSQL grants a 30-second lease. Stop locally after at most 20 seconds
// from the start of the last confirmed operation, retaining a 10-second margin.
// This deadline is not extended by failed calls or delayed acknowledgements.
const runtimeLeaseGrace = 20 * time.Second

func controlPlaneRejected(err error) bool {
	return errors.Is(err, ErrBlocked) || errors.Is(err, sql.ErrNoRows)
}

func (n *managedRuntime) hasLease() bool {
	n.leaseMu.Lock()
	defer n.leaseMu.Unlock()
	return !n.leaseDeadline.IsZero()
}

func (n *managedRuntime) leaseValid(now time.Time) bool {
	n.leaseMu.Lock()
	defer n.leaseMu.Unlock()
	if n.leaseDeadline.IsZero() {
		return true // Non-leasing stores retain their original fail-closed behavior.
	}
	if !now.Before(n.leaseDeadline) {
		n.leaseExpired = true
	}
	return !n.leaseExpired
}

func (n *managedRuntime) confirmLease(started, now time.Time) bool {
	n.leaseMu.Lock()
	defer n.leaseMu.Unlock()
	if n.leaseExpired || !now.Before(n.leaseDeadline) || !now.Before(started.Add(runtimeLeaseGrace)) {
		n.leaseExpired = true
		return false
	}
	deadline := started.Add(runtimeLeaseGrace)
	if deadline.After(n.leaseDeadline) {
		n.leaseDeadline = deadline
	}
	select {
	case n.renew <- struct{}{}:
	default:
	}
	return true
}

// Independent of the manager lock and database calls. A late successful call
// cannot revive a runtime whose local safety deadline has already elapsed.
func (n *managedRuntime) leaseWatchdog(nodeID int64) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-n.runtime.ctx.Done():
			return
		case <-n.renew:
		case <-timer.C:
		}
		n.leaseMu.Lock()
		remaining := time.Until(n.leaseDeadline)
		if remaining <= 0 {
			n.leaseExpired = true
		}
		expired := n.leaseExpired
		n.leaseMu.Unlock()
		if expired {
			// Close UDP before logging; logging must not extend the safety window.
			n.runtime.Close()
			logControlEvent("wireguard.lease_watchdog_expired", nodeID, n.version, "lease_watchdog", context.DeadlineExceeded, runtimeLeaseGrace)
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(remaining)
	}
}

func logControlEvent(event string, nodeID, version int64, stage string, err error, elapsed time.Duration) {
	category := "none"
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		category = "timeout"
	case errors.Is(err, context.Canceled):
		category = "canceled"
	case controlPlaneRejected(err):
		category = "rejected"
	case err != nil:
		category = "storage_error"
	}
	// Never log raw database errors: they can contain SQL, configuration or keys.
	// Skip the database-backed log sink so an I/O incident cannot amplify itself.
	fields := []zap.Field{zap.String("component", "wireguard"), zap.Int64("node_id", nodeID), zap.Int64("config_version", version), zap.String("stage", stage), zap.String("error_category", category), zap.Duration("elapsed", elapsed), zap.Bool(logger.OpsSystemLogSkipField, true)}
	if err == nil && event == "wireguard.control_plane_recovered" {
		logger.L().Info(event, fields...)
	} else {
		logger.L().Warn(event, fields...)
	}
}
