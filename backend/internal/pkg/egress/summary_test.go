package egress

import (
	"reflect"
	"testing"
)

func TestExitSummaryPreservesSnapshotMeaning(t *testing.T) {
	s := ExitSummary{IPs: []string{}}
	s.Add("observed", "203.0.113.9")
	s.Add("observed", "203.0.113.9")
	s.Add("observed", "2001:db8::1")
	s.Add("expired", "203.0.113.10")
	s.Add("unobserved", "203.0.113.11")
	s.Add("observed", "not-an-ip")
	if !reflect.DeepEqual(s.IPs, []string{"2001:db8::1", "203.0.113.9"}) || !s.Recorded || !s.Expired || !s.Unobserved {
		t.Fatalf("invalid summary: %+v", s)
	}
}
