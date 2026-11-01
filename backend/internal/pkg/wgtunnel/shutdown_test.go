package wgtunnel

import (
	"context"
	"io"
	"net/netip"
	"sync"
	"testing"
	"time"
)

func TestConcurrentCloseWaitsForFinalCounters(t *testing.T) {
	client, server := pairedRuntimes(t)
	listener, err := server.network.ListenTCPAddrPort(netip.MustParseAddrPort("10.77.0.1:8099"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	received := make(chan struct{})
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		c, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		b := make([]byte, 1024)
		for {
			n, err := c.Read(b)
			if n > 0 {
				select {
				case <-received:
				default:
					close(received)
				}
				if _, err := c.Write(b[:n]); err != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	c, err := client.DialContext(context.Background(), "tcp", "10.77.0.1:8099")
	if err != nil {
		t.Fatal(err)
	}
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		b := make([]byte, 1024)
		for {
			if _, err := c.Write(b); err != nil {
				return
			}
			if _, err := io.ReadFull(c, b); err != nil {
				return
			}
		}
	}()
	select {
	case <-received:
	case <-time.After(5 * time.Second):
		t.Fatal("traffic did not start")
	}
	// Hold one admitted operation to model a canceled dial still unwinding.
	if !client.beginOperation() {
		t.Fatal("runtime already closed")
	}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(client.operations.Done) }
	defer release()
	first := make(chan struct{})
	go func() { client.Close(); close(first) }()
	<-client.ctx.Done()
	second := make(chan UDPCounts, 1)
	go func() { client.Close(); second <- client.Counts.Snapshot() }()
	select {
	case <-second:
		t.Fatal("concurrent close returned before in-flight work joined")
	case <-time.After(20 * time.Millisecond):
	}
	release()
	var final UDPCounts
	select {
	case final = <-second:
	case <-time.After(5 * time.Second):
		t.Fatal("close deadlocked")
	}
	<-first
	select {
	case <-writerDone:
	case <-time.After(time.Second):
		t.Fatal("connection survived close")
	}
	if final.TX == 0 || final.RX == 0 {
		t.Fatal("missing final traffic")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); client.Close() }()
	}
	wg.Wait()
	if client.Counts.Snapshot() != final {
		t.Fatal("counters changed after close")
	}
	if _, err := client.DialContext(context.Background(), "tcp", "10.77.0.1:8099"); err == nil {
		t.Fatal("dial survived revocation")
	}
	server.Close()
	select {
	case <-serverDone:
	case <-time.After(time.Second):
		t.Fatal("server connection survived close")
	}
}

func TestRetirementWaitsForWatchdogAndSavesTailOnce(t *testing.T) {
	m, store := managedTestNode(t)
	r, _, id, err := m.Acquire(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if !r.beginOperation() {
		t.Fatal("runtime already closed")
	}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(r.operations.Done) }
	defer release()
	watchdog := make(chan struct{})
	go func() { r.Close(); close(watchdog) }()
	<-r.ctx.Done()
	retired := make(chan struct{})
	go func() { m.Revoke(1); close(retired) }()
	select {
	case <-retired:
		t.Fatal("retirement sampled counters before close completed")
	case <-time.After(20 * time.Millisecond):
	}
	// Simulate successful socket I/O accounting just before the worker exits.
	r.Counts.TX.Add(123)
	r.Counts.RX.Add(456)
	release()
	select {
	case <-retired:
	case <-time.After(5 * time.Second):
		t.Fatal("retirement deadlocked")
	}
	<-watchdog
	m.flush()
	m.Revoke(1)
	m.flush()
	store.mu.Lock()
	defer store.mu.Unlock()
	c := store.checkpoints[id]
	final := r.Counts.Snapshot()
	if !c.Closed || c.TX != final.TX || c.RX != final.RX || c.TX < 123 || c.RX < 456 || len(store.checkpoints) != 1 {
		t.Fatalf("wrong final checkpoint: %+v", c)
	}
}
