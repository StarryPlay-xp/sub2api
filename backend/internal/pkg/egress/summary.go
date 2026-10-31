package egress

import (
	"net/netip"
	"slices"
)

type AttemptQuery struct {
	RequestID string `json:"request_id"`
	AccountID int64  `json:"account_id"`
}

// ExitSummary uses immutable connection snapshots, never a node's current IP.
type ExitSummary struct {
	AttemptQuery
	IPs        []string `json:"ips"`
	Recorded   bool     `json:"recorded"`
	Unobserved bool     `json:"unobserved"`
	Expired    bool     `json:"expired"`
}

func (s *ExitSummary) Add(state, ip string) {
	s.Recorded = true
	if state == "expired" {
		s.Expired = true
		return
	}
	address, err := netip.ParseAddr(ip)
	if state != "observed" || err != nil {
		s.Unobserved = true
		return
	}
	ip = address.String()
	if !slices.Contains(s.IPs, ip) {
		s.IPs = append(s.IPs, ip)
		slices.Sort(s.IPs)
	}
}
