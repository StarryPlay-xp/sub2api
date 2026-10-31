package wgtunnel

import "time"

// NodeInput is not an export DTO: configuration plaintext must never be
// serialized into response, idempotency, audit or scheduler payloads.
type NodeInput struct {
	Name      string
	Config    string `json:"-"`
	ExpiresAt *time.Time
}

func (NodeInput) String() string   { return "[WireGuard node input redacted]" }
func (NodeInput) GoString() string { return "[WireGuard node input redacted]" }

type NodeView struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Version     int64      `json:"config_version"`
	Summary     Summary    `json:"summary"`
	Active      bool       `json:"active"`
	Paused      bool       `json:"paused"`
	PauseReason string     `json:"pause_reason,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	TX          string     `json:"tx_bytes"`
	RX          string     `json:"rx_bytes"`
	SavedAt     *time.Time `json:"saved_at,omitempty"`
	Incomplete  bool       `json:"incomplete"`
	MetricBasis string     `json:"metric_basis"`
}
