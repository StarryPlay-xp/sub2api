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

type subjectHandlerStore struct {
	serviceTrafficHandlerStore
	fail bool
}

func (s *subjectHandlerStore) SubjectData(_ context.Context, q service.TrafficSubjectQuery) (service.TrafficSubjectData, error) {
	if s.fail {
		return service.TrafficSubjectData{}, errors.New("synthetic database failure")
	}
	return service.TrafficSubjectData{Window: service.TrafficRankingWindow{TrafficWindow: q.Window, MetricBasis: traffic.MetricBasis}, History: []service.TrafficSubjectHistory{}}, nil
}
func (s *subjectHandlerStore) SubjectIdentities(context.Context, string, []int64) (map[int64]service.TrafficIdentity, error) {
	return map[int64]service.TrafficIdentity{}, nil
}
func TestTrafficSubjectHandlerContracts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		path, reason                        string
		status                              int
		scoped, disabled, unavailable, fail bool
	}{
		{path: "/ranking?subject=user&timezone=Asia%2FTaipei", status: 200},
		{path: "/live?subject=api_key", status: 200},
		{path: "/ranking?subject=user", status: 403, scoped: true},
		{path: "/ranking?subject=user", status: 404, disabled: true},
		{path: "/ranking?subject=user", status: 503, unavailable: true},
		{path: "/ranking?subject=user", status: 500, fail: true},
		{path: "/ranking", status: 400, reason: "TRAFFIC_SUBJECT_INVALID"},
		{path: "/ranking?subject=account", status: 400, reason: "TRAFFIC_SUBJECT_INVALID"},
		{path: "/ranking?subject=user&interval=minute", status: 400, reason: "TRAFFIC_INTERVAL_INVALID"},
		{path: "/ranking?subject=user&sort=invalid", status: 400, reason: "TRAFFIC_SORT_INVALID"},
		{path: "/ranking?subject=user&sort=growth", status: 400, reason: "TRAFFIC_COMPARE_REQUIRED"},
		{path: "/ranking?subject=user&sort=growth&compare=previous", status: 200},
		{path: "/ranking?subject=user&page=0", status: 400, reason: "TRAFFIC_PAGE_INVALID"},
		{path: "/ranking?subject=user&page=", status: 400, reason: "TRAFFIC_PAGE_INVALID"},
		{path: "/ranking?subject=user&page_size=201", status: 400, reason: "TRAFFIC_PAGE_INVALID"},
		{path: "/ranking?subject=user&group_id=1", status: 400, reason: "TRAFFIC_FILTER_INVALID"},
		{path: "/ranking?subject=user&user_id=1", status: 400, reason: "TRAFFIC_FILTER_INVALID"},
		{path: "/ranking?subject=api_key&user_id=0", status: 400, reason: "TRAFFIC_FILTER_INVALID"},
		{path: "/ranking?subject=user&compare=bad", status: 400, reason: "TRAFFIC_FILTER_INVALID"},
		{path: "/ranking?subject=user&subject=api_key", status: 400, reason: "TRAFFIC_FILTER_INVALID"},
		{path: "/ranking?subject=user&range=custom&start=" + time.Now().UTC().Add(-time.Hour).Truncate(time.Hour).Add(time.Second).Format(time.RFC3339), status: 400, reason: "TRAFFIC_BOUNDARY_UNALIGNED"},
		{path: "/live?subject=user&limit=51", status: 400, reason: "TRAFFIC_PAGE_INVALID"},
		{path: "/live?subject=user&sort=total_bytes", status: 400, reason: "TRAFFIC_SORT_INVALID"},
		{path: "/subjects/user/999", status: 404, reason: "TRAFFIC_SUBJECT_NOT_FOUND"},
		{path: "/subjects/user/-1", status: 400, reason: "TRAFFIC_SUBJECT_INVALID"},
	} {
		t.Run(tc.path+tc.reason, func(t *testing.T) {
			store := &subjectHandlerStore{fail: tc.fail}
			collector := traffic.NewCollector(store)
			defer collector.Close()
			ops := service.NewOpsService(nil, nil, &config.Config{Ops: config.OpsConfig{Enabled: !tc.disabled}}, nil, nil, nil, nil, nil, nil, nil, nil)
			monitor := &service.ServiceTrafficService{Collector: collector, Store: store, Subjects: store}
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
			router.GET("/ranking", h.GetTrafficRanking)
			router.GET("/live", h.GetTrafficLive)
			router.GET("/subjects/:type/:id", h.GetTrafficSubject)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest("GET", tc.path, nil))
			require.Equal(t, tc.status, recorder.Code, recorder.Body.String())
			if tc.reason != "" {
				require.Contains(t, recorder.Body.String(), tc.reason)
			}
			if tc.status == 200 {
				require.NotContains(t, recorder.Body.String(), "\"items\":null")
				require.NotContains(t, recorder.Body.String(), "api_key_hash")
			}
		})
	}
}
