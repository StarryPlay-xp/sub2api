package admin

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/hosttraffic"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestHostTrafficAccessAndValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name             string
		disabled, scoped bool
		code             int
	}{
		{"monitoring disabled", true, false, 404}, {"scoped admin rejected", false, true, 403}, {"invalid plan", false, false, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ops := service.NewOpsService(nil, nil, &config.Config{Ops: config.OpsConfig{Enabled: !tc.disabled}}, nil, nil, nil, nil, nil, nil, nil, nil)
			h := NewTrafficOpsHandler(ops, &service.ServiceTrafficService{Host: &hosttraffic.Store{}, HostCollector: &hosttraffic.Collector{}})
			r := gin.New()
			r.PUT("/plan", func(c *gin.Context) {
				if tc.scoped {
					c.Request = c.Request.WithContext(adminaccess.WithPrincipal(c.Request.Context(), &adminaccess.Principal{}))
				}
				h.UpdateHostTrafficPlan(c)
			})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("PUT", "/plan", strings.NewReader(`{"quota_value":"invalid"}`)))
			require.Equal(t, tc.code, w.Code, w.Body.String())
			if tc.disabled {
				require.Contains(t, w.Body.String(), "OPS_DISABLED")
			}
			if tc.code == 400 {
				require.Contains(t, w.Body.String(), "HOST_TRAFFIC_PLAN_INVALID")
				require.Contains(t, w.Body.String(), "field")
			}
		})
	}
}
func TestHostTrafficStrictRequestDecoding(t *testing.T) {
	for _, body := range []string{`{"mode":"auto","unknown":true}`, `{"mode":"auto"} {}`, `{"mode":1}`, strings.Repeat(" ", 8193) + `{}`} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/calibrations", strings.NewReader(body))
		var input hosttraffic.CalibrationInput
		require.False(t, decodeHostTraffic(c, &input))
	}
}
