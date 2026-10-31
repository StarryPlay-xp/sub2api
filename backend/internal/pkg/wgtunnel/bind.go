package wgtunnel

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"

	"golang.zx2c4.com/wireguard/conn"
)

type Counters struct {
	TX atomic.Uint64
	RX atomic.Uint64
}
type UDPCounts struct {
	TX uint64
	RX uint64
}

func (c *Counters) Snapshot() UDPCounts { return UDPCounts{c.TX.Load(), c.RX.Load()} }

// MeteredBind counts UDP payload at successful socket IO, not Peer.SendBuffers.
// BatchSize=1 deliberately avoids GSO/partial-batch accounting ambiguity.
// Each instance belongs to exactly one node/runtime, regardless of account count.
type MeteredBind struct {
	mu     sync.Mutex
	socket *net.UDPConn
	family string
	counts *Counters
}

func NewMeteredBind(family string, counts *Counters) *MeteredBind {
	return &MeteredBind{family: family, counts: counts}
}
func (b *MeteredBind) BatchSize() int { return 1 }
func (b *MeteredBind) SetMark(mark uint32) error {
	if mark != 0 {
		return errors.New("WireGuard socket marks are unsupported")
	}
	return nil
}
func (b *MeteredBind) ParseEndpoint(s string) (conn.Endpoint, error) {
	a, e := netip.ParseAddrPort(s)
	if e != nil {
		return nil, errors.New("invalid WireGuard endpoint")
	}
	return endpoint{a}, nil
}
func (b *MeteredBind) Open(port uint16) ([]conn.ReceiveFunc, uint16, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.socket != nil {
		return nil, 0, conn.ErrBindAlreadyOpen
	}
	s, err := net.ListenUDP(b.family, &net.UDPAddr{Port: int(port)})
	if err != nil {
		return nil, 0, err
	}
	b.socket = s
	recv := func(packets [][]byte, sizes []int, eps []conn.Endpoint) (int, error) {
		n, addr, err := s.ReadFromUDPAddrPort(packets[0])
		if n > 0 {
			b.counts.RX.Add(uint64(n))
		}
		if err != nil {
			return 0, err
		}
		sizes[0] = n
		eps[0] = endpoint{addr}
		return 1, nil
	}
	return []conn.ReceiveFunc{recv}, uint16(s.LocalAddr().(*net.UDPAddr).Port), nil
}
func (b *MeteredBind) Send(bufs [][]byte, ep conn.Endpoint) error {
	b.mu.Lock()
	s := b.socket
	b.mu.Unlock()
	if s == nil {
		return net.ErrClosed
	}
	e, ok := ep.(endpoint)
	if !ok {
		return conn.ErrWrongEndpointType
	}
	for _, p := range bufs {
		n, err := s.WriteToUDPAddrPort(p, e.addr)
		if n > 0 {
			b.counts.TX.Add(uint64(n))
		}
		if err != nil {
			return err
		}
		if n != len(p) {
			return io.ErrShortWrite
		}
	}
	return nil
}
func (b *MeteredBind) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.socket == nil {
		return nil
	}
	err := b.socket.Close()
	b.socket = nil
	return err
}

type endpoint struct{ addr netip.AddrPort }

func (endpoint) ClearSrc()             {}
func (endpoint) SrcToString() string   { return "" }
func (endpoint) SrcIP() netip.Addr     { return netip.Addr{} }
func (e endpoint) DstToString() string { return e.addr.String() }
func (e endpoint) DstIP() netip.Addr   { return e.addr.Addr() }
func (e endpoint) DstToBytes() []byte {
	a := e.addr.Addr().AsSlice()
	out := make([]byte, len(a)+2)
	copy(out, a)
	binary.LittleEndian.PutUint16(out[len(a):], e.addr.Port())
	return out
}
