package admin

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type egressHandlerStore struct {
	serviceTrafficHandlerStore
	fail bool
}

func (s *egressHandlerStore) EgressData(_ context.Context, q service.TrafficEgressQuery) (service.TrafficEgressData, error) {
	if s.fail {
		return service.TrafficEgressData{}, errors.New("synthetic failure")
	}
	return service.TrafficEgressData{Window: service.TrafficEgressWindow{TrafficWindow: q.Window, MetricBasis: traffic.MetricBasis}, Routes: map[traffic.EgressIdentity]service.TrafficRouteIdentity{{Type: "direct"}: {RouteType: "direct"}}, History: []service.TrafficEgressHistory{}}, nil
}
func TestTrafficEgressHandlerContracts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		path, reason                        string
		status                              int
		scoped, disabled, unavailable, fail bool
	}{
		{path: "/egress?timezone=Asia%2FTaipei", status: 200},
		{path: "/egress/direct/0?compare=previous", status: 200},
		{path: "/egress", status: 403, scoped: true},
		{path: "/egress", status: 404, disabled: true},
		{path: "/egress", status: 503, unavailable: true},
		{path: "/egress", status: 500, fail: true},
		{path: "/egress/proxy/999", status: 404, reason: "TRAFFIC_ROUTE_NOT_FOUND"},
		{path: "/egress/direct/1", status: 400, reason: "TRAFFIC_ROUTE_INVALID"},
		{path: "/egress/proxy/0", status: 400, reason: "TRAFFIC_ROUTE_INVALID"},
		{path: "/egress/wireguard/-1", status: 400, reason: "TRAFFIC_ROUTE_INVALID"},
		{path: "/egress/proxy/9223372036854775808", status: 400, reason: "TRAFFIC_ROUTE_INVALID"},
		{path: "/egress/unknown/1", status: 400, reason: "TRAFFIC_ROUTE_INVALID"},
		{path: "/egress/proxy/no", status: 400, reason: "TRAFFIC_ROUTE_INVALID"},
		{path: "/egress?interval=minute", status: 400, reason: "TRAFFIC_INTERVAL_INVALID"},
		{path: "/egress?compare=bad", status: 400, reason: "TRAFFIC_FILTER_INVALID"},
		{path: "/egress?range=24h&range=7d", status: 400, reason: "TRAFFIC_FILTER_INVALID"},
		{path: "/egress?timezone=a&timezone=b", status: 400, reason: "TRAFFIC_FILTER_INVALID"},
		{path: "/egress?q=search", status: 400, reason: "TRAFFIC_FILTER_INVALID"},
		{path: "/egress?range=custom&start=" + time.Now().UTC().Add(-time.Hour).Truncate(time.Hour).Add(time.Second).Format(time.RFC3339), status: 400, reason: "TRAFFIC_BOUNDARY_UNALIGNED"},
		{path: "/egress?range=invalid", status: 400},
		{path: "/egress?range=custom&start=invalid", status: 400},
	} {
		t.Run(tc.path, func(t *testing.T) {
			store := &egressHandlerStore{fail: tc.fail}
			collector := traffic.NewCollector(store)
			defer collector.Close()
			ops := service.NewOpsService(nil, nil, &config.Config{Ops: config.OpsConfig{Enabled: !tc.disabled}}, nil, nil, nil, nil, nil, nil, nil, nil)
			monitor := &service.ServiceTrafficService{Collector: collector, Store: store, Egress: store}
			if tc.unavailable {
				monitor = nil
			}
			h := NewTrafficOpsHandler(ops, monitor)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				if tc.scoped {
					c.Request = c.Request.WithContext(adminaccess.WithPrincipal(c.Request.Context(), &adminaccess.Principal{}))
				}
				c.Next()
			})
			router.GET("/egress", h.GetTrafficEgress)
			router.GET("/egress/:route_type/:route_id", h.GetTrafficEgressDetail)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
			require.Equal(t, tc.status, w.Code, w.Body.String())
			if tc.reason != "" {
				require.Contains(t, w.Body.String(), tc.reason)
			}
			if tc.status == 200 {
				require.NotContains(t, w.Body.String(), `"routes":null`)
				require.NotContains(t, w.Body.String(), `"accounts":null`)
				require.NotContains(t, w.Body.String(), `"history":null`)
			}
		})
	}
}
