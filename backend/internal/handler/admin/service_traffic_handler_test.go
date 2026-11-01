package admin

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type serviceTrafficHandlerStore struct{ window wgtunnel.TrafficWindow }

func (s *serviceTrafficHandlerStore) Report(_ context.Context, w wgtunnel.TrafficWindow) (service.ServiceTrafficReport, error) {
	s.window = w
	return service.ServiceTrafficReport{TrafficWindow: w, Scope: "single_instance_history", MetricBasis: traffic.MetricBasis}, nil
}
func (s *serviceTrafficHandlerStore) Save(context.Context, traffic.Snapshot) error { return nil }
func (s *serviceTrafficHandlerStore) Prune(context.Context, time.Time) error       { return nil }
func TestServiceTrafficHandlerAccessAndFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, query      string
		scoped, disabled bool
		status           int
	}{{"allowed", "?range=24h", false, false, 200}, {"panel timezone accepted", "?range=24h&timezone=Asia%2FTaipei", false, false, 200}, {"platform filter rejected", "?platform=openai", false, false, 400}, {"scoped", "", true, false, 403}, {"disabled", "", false, true, 404}, {"group filter rejected", "?group_id=1", false, false, 400}, {"range rejected", "?range=365d", false, false, 400}, {"unaligned rejected", "?range=custom&start=2026-10-31T00:00:01Z&end=2026-10-31T01:00:00Z", false, false, 400}} {
		t.Run(tc.name, func(t *testing.T) {
			store := &serviceTrafficHandlerStore{}
			collector := traffic.NewCollector(store)
			defer collector.Close()
			ops := service.NewOpsService(nil, nil, &config.Config{Ops: config.OpsConfig{Enabled: !tc.disabled}}, nil, nil, nil, nil, nil, nil, nil, nil)
			h := NewTrafficOpsHandler(ops, &service.ServiceTrafficService{Collector: collector, Store: store})
			r := gin.New()
			r.GET("/traffic", func(c *gin.Context) {
				if tc.scoped {
					c.Request = c.Request.WithContext(adminaccess.WithPrincipal(c.Request.Context(), &adminaccess.Principal{}))
				}
				h.GetServiceTraffic(c)
			})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", "/traffic"+tc.query, nil))
			require.Equal(t, tc.status, w.Code, w.Body.String())
			if tc.status == 200 {
				require.Equal(t, "minute", store.window.Interval)
				require.Contains(t, w.Body.String(), "single_instance_history")
			}
		})
	}
}
