package egress

import "context"

type routeContextKey struct{}

func WithRoute(ctx context.Context, route *Route) context.Context {
	return context.WithValue(ctx, routeContextKey{}, route)
}
func FromContext(ctx context.Context) *Route {
	if ctx == nil {
		return nil
	}
	r, _ := ctx.Value(routeContextKey{}).(*Route)
	return r
}

// LegacyProxyURL is only for adapting an existing HTTP/SOCKS transport. WG has
// no proxy URL; consumers must handle its typed Kind before calling this method.
func (r *Route) LegacyProxyURL() string {
	if r.Kind() == Proxy {
		return r.record.ProxyURL
	}
	return ""
}
