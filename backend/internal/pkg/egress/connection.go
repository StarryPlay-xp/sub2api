package egress

import (
	"context"
	"net"
	"sync"
	"time"
)

// Active streams cannot outlive a revoked account binding or node generation.
// One checker is shared by all sockets of a route, including idle HTTP pools.
type connectionGroup struct {
	route   *Route
	conns   map[*guardedConnection]struct{}
	done    chan struct{}
	revoked bool
}
type guardedConnection struct {
	net.Conn
	once  sync.Once
	group *connectionGroup
}

func (c *guardedConnection) Close() error {
	var err error
	c.once.Do(func() {
		err = c.Conn.Close()
		router := c.group.route.router
		router.connMu.Lock()
		delete(c.group.conns, c)
		if len(c.group.conns) == 0 {
			delete(router.connections, c.group.route.PoolKey())
			close(c.group.done)
		}
		router.connMu.Unlock()
	})
	return err
}
func (r *Route) guardConnection(conn net.Conn) net.Conn {
	if r.AccountID() == 0 && r.Kind() != WireGuard {
		return conn
	}
	router := r.router
	router.connMu.Lock()
	defer router.connMu.Unlock()
	if router.connections == nil {
		router.connections = map[string]*connectionGroup{}
	}
	group := router.connections[r.PoolKey()]
	if group == nil {
		group = &connectionGroup{route: r, conns: map[*guardedConnection]struct{}{}, done: make(chan struct{})}
		router.connections[r.PoolKey()] = group
		go group.watch()
	}
	if group.revoked {
		_ = conn.Close()
		return conn
	}
	c := &guardedConnection{Conn: conn, group: group}
	group.conns[c] = struct{}{}
	return c
}
func (g *connectionGroup) watch() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-g.done:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			err := g.route.Check(ctx)
			cancel()
			if err != nil {
				g.route.router.connMu.Lock()
				g.revoked = true
				connections := make([]*guardedConnection, 0, len(g.conns))
				for c := range g.conns {
					connections = append(connections, c)
				}
				g.route.router.connMu.Unlock()
				for _, c := range connections {
					_ = c.Close()
				}
				return
			}
		}
	}
}
