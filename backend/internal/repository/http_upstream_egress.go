package repository

import (
	"net/http"
	"net/url"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/egress"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// NewRoutedHTTPUpstream is the production constructor once all account paths
// have migrated to typed routing. The legacy constructor remains for isolated
// transport tests and non-account callers.
func NewRoutedHTTPUpstream(cfg *config.Config, router *egress.Router) service.HTTPUpstream {
	s := NewHTTPUpstream(cfg).(*httpUpstreamService)
	s.egressRouter = router
	return s
}
func (s *httpUpstreamService) prepareEgress(request *http.Request, proxyURL string, accountID int64) (*http.Request, string, error) {
	if request == nil {
		return request, proxyURL, egress.ErrBlocked
	}
	route := egress.FromContext(request.Context())
	if route == nil && s.egressRouter != nil && accountID > 0 {
		var err error
		route, err = s.egressRouter.Resolve(request.Context(), egress.Reference{AccountID: accountID})
		if err != nil {
			return request, "", err
		}
	}
	if route == nil {
		return request, proxyURL, nil
	}
	if accountID > 0 && route.AccountID() != accountID {
		return request, "", egress.ErrBlocked
	}
	if err := egress.ScopeFromContext(request.Context()).Pin(route); err != nil {
		return request, "", err
	}
	if err := route.Check(request.Context()); err != nil {
		return request, "", err
	}
	if s.shouldValidateResolvedIP() || service.HTTPUpstreamPublicHostsOnly(request.Context()) {
		route = route.RequirePublicTargets()
	}
	request = request.WithContext(egress.WithRoute(request.Context(), route))
	return request, route.LegacyProxyURL(), nil
}
func normalizeEgressRoute(proxyURL string, routes []*egress.Route) (string, *url.URL, error) {
	if len(routes) == 0 || routes[0] == nil {
		return normalizeProxyURL(proxyURL)
	}
	route := routes[0]
	_, parsed, err := normalizeProxyURL(route.LegacyProxyURL())
	if err != nil {
		return "", nil, err
	}
	return route.PoolKey(), parsed, nil
}
