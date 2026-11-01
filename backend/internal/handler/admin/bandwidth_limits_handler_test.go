package admin

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type bandwidthHandlerStore struct {
	err    error
	calls  int
	nodeID int64
}

func (s *bandwidthHandlerStore) ReadBandwidthLimits(context.Context) (service.BandwidthLimits, error) {
	s.calls++
	return service.BandwidthLimits{Nodes: map[string]service.BandwidthLimit{}}, s.err
}
func (s *bandwidthHandlerStore) UpdateBandwidthLimit(_ context.Context, nodeID int64, _ string, mbps *float64) (service.BandwidthLimit, error) {
	s.calls++
	s.nodeID = nodeID
	return service.BandwidthLimit{Mbps: mbps, Version: "saved"}, s.err
}
func TestBandwidthLimitsHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, method, path, body string
		scoped                   bool
		err                      error
		status, calls            int
	}{
		{"read", "GET", "/limits", "", false, nil, 200, 1},
		{"save host", "PUT", "/limits/host", `{"mbps":100.5,"version":""}`, false, nil, 200, 1},
		{"clear node", "PUT", "/limits/nodes/1", `{"mbps":null,"version":""}`, false, nil, 200, 1},
		{"read scoped", "GET", "/limits", "", true, nil, 403, 0},
		{"write scoped", "PUT", "/limits/host", `{"mbps":100,"version":""}`, true, nil, 403, 0},
		{"missing value", "PUT", "/limits/host", `{"version":""}`, false, nil, 400, 0},
		{"missing version", "PUT", "/limits/host", `{"mbps":100}`, false, nil, 400, 0},
		{"null version", "PUT", "/limits/host", `{"mbps":100,"version":null}`, false, nil, 400, 0},
		{"invalid version", "PUT", "/limits/host", `{"mbps":100,"version":"old"}`, false, nil, 400, 0},
		{"zero", "PUT", "/limits/host", `{"mbps":0,"version":""}`, false, nil, 400, 0},
		{"negative", "PUT", "/limits/host", `{"mbps":-1,"version":""}`, false, nil, 400, 0},
		{"too large", "PUT", "/limits/host", `{"mbps":1000001,"version":""}`, false, nil, 400, 0},
		{"string", "PUT", "/limits/host", `{"mbps":"100","version":""}`, false, nil, 400, 0},
		{"trailing", "PUT", "/limits/host", `{"mbps":100,"version":""}{}`, false, nil, 400, 0},
		{"unknown field", "PUT", "/limits/host", `{"mbps":100,"version":"","extra":true}`, false, nil, 400, 0},
		{"invalid node", "PUT", "/limits/nodes/0", `{"mbps":100,"version":""}`, false, nil, 400, 0},
		{"conflict", "PUT", "/limits/host", `{"mbps":100,"version":""}`, false, service.ErrBandwidthLimitConflict, 409, 1},
		{"missing node", "PUT", "/limits/nodes/2", `{"mbps":100,"version":""}`, false, service.ErrBandwidthNodeNotFound, 404, 1},
		{"read failure", "GET", "/limits", "", false, errors.New("private database details"), 500, 1},
		{"write failure", "PUT", "/limits/host", `{"mbps":100,"version":""}`, false, errors.New("private database details"), 500, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &bandwidthHandlerStore{err: tc.err}
			h := NewTrafficOpsHandler(nil, &service.ServiceTrafficService{Limits: s})
			r := gin.New()
			r.Use(func(c *gin.Context) {
				if tc.scoped {
					c.Request = c.Request.WithContext(adminaccess.WithPrincipal(c.Request.Context(), &adminaccess.Principal{}))
				}
				c.Next()
			})
			r.GET("/limits", h.GetBandwidthLimits)
			r.PUT("/limits/host", h.UpdateBandwidthLimit)
			r.PUT("/limits/nodes/:id", h.UpdateBandwidthLimit)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
			require.Equal(t, tc.status, w.Code, w.Body.String())
			require.Equal(t, tc.calls, s.calls)
			require.NotContains(t, w.Body.String(), "private database details")
			if tc.name == "clear node" {
				require.Equal(t, int64(1), s.nodeID)
				require.Contains(t, w.Body.String(), `"mbps":null`)
			}
		})
	}
}
