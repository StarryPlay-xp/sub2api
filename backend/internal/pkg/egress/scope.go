package egress

import (
	"context"
	"sync"
)

type scopeContextKey struct{}

// Scope belongs to one incoming request, including its account-selection retry
// loop and credential refresh calls. Once WG is selected, later attempts cannot
// switch accounts, nodes, generations or direct routes.
type Scope struct {
	mu        sync.Mutex
	pinned    bool
	accountID int64
	nodeID    int64
	version   int64
	runtimeID string
	revision  int64
	blocked   bool
}

func WithScope(ctx context.Context) context.Context {
	if ScopeFromContext(ctx) != nil {
		return ctx
	}
	return context.WithValue(ctx, scopeContextKey{}, &Scope{})
}
func ScopeFromContext(ctx context.Context) *Scope {
	if ctx == nil {
		return nil
	}
	s, _ := ctx.Value(scopeContextKey{}).(*Scope)
	return s
}
func (s *Scope) Pin(route *Route) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked {
		return ErrBlocked
	}
	if s.pinned {
		if route.Kind() != WireGuard || route.AccountID() != s.accountID || route.NodeID() != s.nodeID || (s.runtimeID != "" && (route.RuntimeID() != s.runtimeID || route.Version() != s.version)) || route.record.Revision != s.revision {
			return ErrBlocked
		}
		s.version = route.Version()
		s.runtimeID = route.RuntimeID()
		return nil
	}
	if route.Kind() == WireGuard {
		s.pinned = true
		s.accountID = route.AccountID()
		s.nodeID = route.NodeID()
		s.version = route.Version()
		s.runtimeID = route.RuntimeID()
		s.revision = route.record.Revision
	}
	return nil
}
func (s *Scope) BlocksFailover() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pinned || s.blocked
}

// Select runs before runtime startup, so even an unavailable WG node pins the
// request. A resolution failure poisons the scope instead of guessing a route.
func (s *Scope) Select(record Record) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked {
		return ErrBlocked
	}
	if s.pinned {
		if record.Kind != WireGuard || record.AccountID != s.accountID || record.NodeID != s.nodeID || record.Revision != s.revision {
			return ErrBlocked
		}
		return nil
	}
	if record.Kind == WireGuard {
		s.pinned = true
		s.accountID = record.AccountID
		s.nodeID = record.NodeID
		s.revision = record.Revision
	}
	return nil
}
func (s *Scope) Block() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.blocked = true
	s.mu.Unlock()
}
