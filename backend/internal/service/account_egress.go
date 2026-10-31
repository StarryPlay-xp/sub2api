package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/egress"
)

// EgressRouterProvider is supplied by the production account/proxy repositories.
// Narrow test repositories can omit it; known WG accounts still fail closed.
type EgressRouterProvider interface{ EgressRouter() *egress.Router }

func contextForAccountEgress(ctx context.Context, repository any, account *Account) (context.Context, error) {
	if account == nil {
		return ctx, egress.ErrBlocked
	}
	if route := egress.FromContext(ctx); route != nil {
		if route.AccountID() != account.ID && route.CredentialParentID() != account.ID {
			return ctx, egress.ErrBlocked
		}
		return ctx, route.Check(ctx)
	}
	provider, ok := repository.(EgressRouterProvider)
	if !ok || provider.EgressRouter() == nil {
		if account.Proxy != nil && account.Proxy.Protocol == ProxyProtocolWireGuard {
			return ctx, egress.ErrBlocked
		}
		return ctx, nil
	}
	ctx = egress.WithScope(ctx)
	route, err := provider.EgressRouter().Resolve(ctx, egress.Reference{AccountID: account.ID})
	if err != nil {
		return ctx, err
	}
	return egress.WithRoute(ctx, route), nil
}

func contextForNodeEgress(ctx context.Context, repository any, nodeID *int64) (context.Context, error) {
	if route := egress.FromContext(ctx); route != nil {
		return ctx, route.Check(ctx)
	}
	provider, ok := repository.(EgressRouterProvider)
	if !ok || provider.EgressRouter() == nil {
		return ctx, nil
	}
	ref := egress.Reference{ExplicitDirect: true}
	if nodeID != nil && *nodeID > 0 {
		ref = egress.Reference{NodeID: *nodeID}
	}
	ctx = egress.WithScope(ctx)
	route, err := provider.EgressRouter().Resolve(ctx, ref)
	if err != nil {
		return ctx, err
	}
	return egress.WithRoute(ctx, route), nil
}

func contextForAccountIDEgress(ctx context.Context, repository any, id int64) (context.Context, error) {
	provider, ok := repository.(EgressRouterProvider)
	if !ok || provider.EgressRouter() == nil {
		return ctx, nil
	}
	if route := egress.FromContext(ctx); route != nil {
		if route.AccountID() != id && route.CredentialParentID() != id {
			return ctx, egress.ErrBlocked
		}
		return ctx, route.Check(ctx)
	}
	ctx = egress.WithScope(ctx)
	route, err := provider.EgressRouter().Resolve(ctx, egress.Reference{AccountID: id})
	if err != nil {
		return ctx, err
	}
	return egress.WithRoute(ctx, route), nil
}

// ShouldRetryNextAccountContext preserves a selected WG route across the entire incoming request.
func (e *UpstreamFailoverError) ShouldRetryNextAccountContext(ctx context.Context) bool {
	return e.ShouldRetryNextAccount() && !egress.ScopeFromContext(ctx).BlocksFailover()
}

func contextForOAuthEgress(ctx context.Context, repository any, saved *egress.Route, override *int64) (context.Context, error) {
	if saved != nil {
		if override != nil && *override != saved.NodeID() {
			return ctx, egress.ErrBlocked
		}
		if err := saved.Check(ctx); err != nil {
			return ctx, err
		}
		return egress.WithRoute(egress.WithScope(ctx), saved), nil
	}
	return contextForNodeEgress(ctx, repository, override)
}

// ContextForSelectedNode is used by administrator token-import endpoints before
// converting the selected node to any legacy HTTP/SOCKS adapter arguments.
func ContextForSelectedNode(ctx context.Context, repository any, id *int64) (context.Context, error) {
	return contextForNodeEgress(ctx, repository, id)
}
func (s *adminServiceImpl) EgressRouter() *egress.Router {
	if p, ok := s.proxyRepo.(EgressRouterProvider); ok {
		return p.EgressRouter()
	}
	return nil
}
