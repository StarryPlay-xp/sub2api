package netstack

import (
	"net/netip"
	"runtime"
	"sync"
	"testing"
	"time"

	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

func TestCloseUnblocksAndJoinsNotification(t *testing.T) {
	dev, _, err := CreateNetTUN([]netip.Addr{netip.MustParseAddr("10.0.0.1")}, nil, 1280)
	if err != nil {
		t.Fatal(err)
	}
	tun := dev.(*netTun)
	defer tun.Close()
	packet := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData([]byte{1, 2, 3})})
	defer packet.DecRef()
	var packets stack.PacketBufferList
	packets.PushBack(packet)
	written := make(chan struct{})
	go func() { tun.ep.WritePackets(packets); close(written) }()
	// There is deliberately no TUN reader. Wait until WriteNotify holds the
	// read lock and blocks sending, then prove Close releases that callback.
	deadline := time.Now().Add(3 * time.Second)
	for tun.notifyMu.TryLock() {
		tun.notifyMu.Unlock()
		if time.Now().After(deadline) {
			t.Fatal("notification did not start")
		}
		runtime.Gosched()
	}
	finished := make(chan struct{})
	go func() {
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); tun.Close() }()
		}
		wg.Wait()
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("close deadlocked")
	}
	select {
	case <-written:
	case <-time.After(time.Second):
		t.Fatal("notification survived close")
	}
	// A callback whose notification list was captured before RemoveNotify is safe.
	tun.WriteNotify()
	if _, err := tun.Read([][]byte{make([]byte, 1280)}, make([]int, 1), 0); err == nil {
		t.Fatal("closed reader succeeded")
	}
}
