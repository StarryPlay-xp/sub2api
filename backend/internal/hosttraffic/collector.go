package hosttraffic

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Reading struct {
	Name     string
	Index    int
	Boot     string
	BootAt   time.Time
	At       time.Time
	RX       string
	TX       string
	Physical *bool
	Route    *Routes
}
type Sample struct {
	At         time.Time
	Readings   []Reading
	Collection Collection
}
type Reader struct {
	Root       string
	OS         string
	Deployment string
}

func excluded(n string) bool {
	if n == "lo" {
		return true
	}
	for _, prefix := range []string{"docker", "veth", "br-", "wg", "tun", "tap", "virbr", "dummy", "ifb"} {
		if strings.HasPrefix(n, prefix) {
			return true
		}
	}
	return false
}
func (r Reader) path(p string) string { return filepath.Join(r.Root, p) }
func (r Reader) Read(at time.Time) Sample {
	at = at.UTC().Truncate(time.Minute)
	out := Sample{At: at, Collection: Collection{State: "starting", Deployment: "binary"}}
	osName := r.OS
	if osName == "" {
		osName = runtime.GOOS
	}
	if osName != "linux" {
		out.Collection.State = "unsupported"
		out.Collection.Deployment = "unknown"
		return out
	}
	_, dockerErr := os.Stat(r.path("/.dockerenv"))
	if r.Deployment == "docker" || dockerErr == nil {
		out.Collection.Deployment = "docker"
	}
	root := "/sys/class/net"
	source := "native"
	if _, e := os.Stat(r.path("/host/sys/class/net")); e == nil {
		root = "/host/sys/class/net"
		source = "host_mount"
	} else if out.Collection.Deployment == "docker" {
		out.Collection.State = "needs_host_mount"
		return out
	}
	out.Collection.Source = &source
	fail := func(reason string) Sample {
		out.Collection.State = "error"
		out.Collection.ErrorReason = &reason
		out.Readings = nil
		return out
	}
	boot, e := os.ReadFile(r.path("/proc/sys/kernel/random/boot_id"))
	if e != nil {
		return fail("boot_identity_unavailable")
	}
	bootID := strings.TrimSpace(string(boot))
	if id, e := uuid.Parse(bootID); e != nil || id == uuid.Nil {
		return fail("boot_identity_unavailable")
	}
	uptime, e := os.ReadFile(r.path("/proc/uptime"))
	if e != nil {
		return fail("uptime_unavailable")
	}
	fields := strings.Fields(string(uptime))
	if len(fields) == 0 {
		return fail("uptime_unavailable")
	}
	seconds, e := strconv.ParseFloat(fields[0], 64)
	if e != nil || seconds < 0 || seconds > 1e11 {
		return fail("uptime_unavailable")
	}
	// uptime is read at actual time, not at the truncated sample label.
	bootAt := time.Now().UTC().Add(-time.Duration(seconds * float64(time.Second))).Truncate(time.Second)
	out.Collection.HostBootAt = &bootAt
	entries, e := os.ReadDir(r.path(root))
	if e != nil {
		return fail("interfaces_unavailable")
	}
	routes := map[string]*Routes{}
	if source == "native" {
		routes = r.routes()
	}
	for _, entry := range entries {
		name := entry.Name()
		if excluded(name) {
			continue
		}
		base := r.path(filepath.Join(root, name))
		// sysfs can also expose class control files such as bonding_masters.
		// Network interfaces are directories (usually reached through symlinks).
		info, statErr := os.Stat(base)
		if statErr != nil {
			return fail("counter_read_failed")
		}
		if !info.IsDir() {
			continue
		}
		readNum := func(p string) (string, error) {
			b, e := os.ReadFile(filepath.Join(base, p))
			if e != nil {
				return "", e
			}
			s := strings.TrimSpace(string(b))
			_, e = strconv.ParseUint(s, 10, 64)
			return s, e
		}
		idx, e := readNum("ifindex")
		if e != nil {
			return fail("counter_read_failed")
		}
		index, _ := strconv.Atoi(idx)
		rx, e := readNum("statistics/rx_bytes")
		if e != nil {
			return fail("counter_read_failed")
		}
		tx, e := readNum("statistics/tx_bytes")
		if e != nil {
			return fail("counter_read_failed")
		}
		var physical *bool
		if _, e = os.Stat(filepath.Join(base, "device")); e == nil {
			physical = ptr(true)
		} else if errors.Is(e, os.ErrNotExist) {
			physical = ptr(false)
		}
		route := routes[name]
		if source == "native" && route == nil {
			route = &Routes{}
		}
		out.Readings = append(out.Readings, Reading{Name: name, Index: index, Boot: bootID, BootAt: bootAt, At: at, RX: rx, TX: tx, Physical: physical, Route: route})
	}
	if len(out.Readings) > 16 {
		return fail("too_many_interfaces")
	}
	if len(out.Readings) == 0 {
		return fail("no_candidate_interfaces")
	}
	out.Collection.State = "ok"
	out.Collection.LastSampleAt = &at
	return out
}
func (r Reader) routes() map[string]*Routes {
	result := map[string]*Routes{}
	get := func(n string) *Routes {
		if result[n] == nil {
			result[n] = &Routes{}
		}
		return result[n]
	}
	if b, e := os.ReadFile(r.path("/proc/net/route")); e == nil {
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			if len(f) >= 8 && f[1] == "00000000" && f[7] == "00000000" {
				flags, _ := strconv.ParseUint(f[3], 16, 32)
				if flags&1 != 0 {
					get(f[0]).IPv4 = true
				}
			}
		}
	}
	if b, e := os.ReadFile(r.path("/proc/net/ipv6_route")); e == nil {
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			if len(f) >= 10 && f[0] == strings.Repeat("0", 32) && f[1] == "00" {
				flags, _ := strconv.ParseUint(f[8], 16, 32)
				if flags&1 != 0 && flags&0x200 == 0 {
					get(f[9]).IPv6 = true
				}
			}
		}
	}
	return result
}
func selectInterfaces(readings []Reading, p *Plan) ([]string, Selection) {
	selection := Selection{Mode: "auto"}
	names := []string{}
	if p != nil && len(p.Interfaces) > 0 {
		return append(names, p.Interfaces...), Selection{Mode: "manual"}
	}
	v4, v6 := []string{}, []string{}
	physical := []string{}
	for _, r := range readings {
		if r.Route != nil {
			if r.Route.IPv4 {
				v4 = append(v4, r.Name)
			}
			if r.Route.IPv6 {
				v6 = append(v6, r.Name)
			}
		}
		if r.Physical != nil && *r.Physical {
			physical = append(physical, r.Name)
		}
	}
	if len(v4) == 1 && len(v6) == 1 && v4[0] == v6[0] {
		names = append(names, v4[0])
	} else if len(v4) == 1 && len(v6) == 0 {
		names = append(names, v4[0])
	} else if len(v6) == 1 && len(v4) == 0 {
		names = append(names, v6[0])
	} else if len(v4) == 0 && len(v6) == 0 && len(readings) == 1 && len(physical) == 1 {
		names = append(names, physical[0])
	} else {
		selection.Ambiguous = true
	}
	sort.Strings(names)
	return names, selection
}

// Collector uses bounded FIFO retries. If the queue overflows, the next
// absolute counter still recovers same-stage bytes; crossed periods stay unknown.
type Collector struct {
	store  *Store
	reader Reader
	cancel context.CancelFunc
	done   chan struct{}
	mu     sync.RWMutex
	health Collection
}

func Start(store *Store) *Collector {
	ctx, cancel := context.WithCancel(context.Background())
	c := &Collector{store: store, reader: Reader{Deployment: os.Getenv("SUB2API_DEPLOYMENT")}, cancel: cancel, done: make(chan struct{}), health: Collection{State: "starting", Deployment: "unknown"}}
	// Discover support immediately; only persist readings on minute boundaries.
	sample := c.reader.Read(time.Now())
	c.health = sample.Collection
	c.health.LastSampleAt = nil
	if c.health.State == "ok" {
		c.health.State = "starting"
	}
	go c.run(ctx)
	return c
}
func (c *Collector) Health() Collection { c.mu.RLock(); defer c.mu.RUnlock(); return c.health }
func (c *Collector) Close() {
	if c != nil {
		c.cancel()
		<-c.done
	}
}
func (c *Collector) run(ctx context.Context) {
	defer close(c.done)
	queue := []Sample{}
	for {
		timer := time.NewTimer(time.Until(time.Now().Truncate(time.Minute).Add(time.Minute)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		sample := c.reader.Read(time.Now())
		c.mu.Lock()
		c.health = sample.Collection
		c.mu.Unlock()
		if sample.Collection.State != "ok" {
			continue
		}
		queue = append(queue, sample)
		if len(queue) > 180 {
			queue = queue[len(queue)-180:]
		}
		for len(queue) > 0 {
			writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := c.store.Save(writeCtx, queue[0])
			cancel()
			if err != nil {
				c.mu.Lock()
				c.health.State = "error"
				c.health.ErrorReason = ptr("persistence_unavailable")
				c.mu.Unlock()
				break
			}
			queue = queue[1:]
		}
	}
}
