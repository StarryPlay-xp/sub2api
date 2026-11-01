package wgtunnel

import (
	"testing"
	"time"
)

func TestMeterCountsBothDirectionsOnce(t *testing.T) {
	var counters Counters
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := NewMeter(&counters, now)
	counters.TX.Add(100)
	counters.RX.Add(200)
	r, e := m.Sample(now.Add(time.Second))
	if e != nil || r.TX != 100 || r.RX != 200 {
		t.Fatal("wrong directional rate", r, e)
	}
	c, gap := m.Checkpoint(1, "run", 1, false)
	if gap || c.TX+c.RX != 300 || len(c.Buckets) != 1 {
		t.Fatal("wrong checkpoint")
	}
	// Reading twice must not add the same counters to the minute bucket twice.
	c2, _ := m.Checkpoint(1, "run", 1, false)
	if c2.Seq != c.Seq || c2.Buckets[0].TX != 100 {
		t.Fatal("checkpoint changed counters")
	}
	m.Ack(c)
	counters.TX.Add(50)
	if _, err := m.Sample(now.Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	c3, _ := m.Checkpoint(1, "run", 1, false)
	if c3.Buckets[0].TX != 150 {
		t.Fatal("open bucket lost cumulative identity")
	}
}

func TestMeterMarksResetAndMissingHistory(t *testing.T) {
	var c Counters
	now := time.Now()
	m := NewMeter(&c, now)
	c.TX.Add(10)
	if _, err := m.Sample(now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	c.TX.Store(0)
	if _, e := m.Sample(now.Add(2 * time.Second)); e == nil {
		t.Fatal("counter reset silently accepted")
	}
	_, gap := m.Checkpoint(1, "run", 1, false)
	if !gap {
		t.Fatal("reset not marked")
	}
}

func TestMeterAckPreservesIdleSamplesAddedDuringSave(t *testing.T) {
	var counts Counters
	start := time.Date(2026, 1, 1, 0, 0, 57, 0, time.UTC)
	m := NewMeter(&counts, start)
	if _, err := m.Sample(start.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	saved, _ := m.Checkpoint(1, "run", 1, false)
	if _, err := m.Sample(start.Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Sample(start.Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	m.Ack(saved)
	next, _ := m.Checkpoint(1, "run", 1, false)
	if len(next.Buckets) != 2 {
		t.Fatal("unsaved idle sample was discarded")
	}
	m.Ack(next)
	final, _ := m.Checkpoint(1, "run", 1, false)
	if len(final.Buckets) != 1 {
		t.Fatal("persisted completed bucket was not retired")
	}
}
