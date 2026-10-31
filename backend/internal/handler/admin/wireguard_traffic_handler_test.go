package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type trafficHandlerStore struct {
	service.WireGuardRepository
	called bool
	saved  time.Time
}

func (s *trafficHandlerStore) ReadNode(_ context.Context, id int64) (wgtunnel.NodeView, error) {
	s.called = true
	return wgtunnel.NodeView{Incomplete: true, SavedAt: &s.saved}, nil
}

func (s *trafficHandlerStore) TrafficReport(_ context.Context, id int64, window wgtunnel.TrafficWindow) (wgtunnel.TrafficReport, error) {
	return wgtunnel.TrafficReport{TrafficWindow: window, TX: "9007199254740993", RX: "7", Total: "9007199254741000", HasRecords: true, BucketCount: 1, History: []wgtunnel.Bucket{{Start: window.Start, TX: 9007199254740993, RX: 7}}}, nil
}

func TestWireGuardTrafficHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, path, reason  string
		status              int
		scoped, unavailable bool
	}{
		{name: "period", path: "1?range=7d", status: 200},
		{name: "invalid-node", path: "0", status: 400},
		{name: "invalid-range", path: "1?range=365d", status: 400, reason: "TRAFFIC_RANGE_INVALID"},
		{name: "invalid-interval", path: "1?interval=day", status: 400, reason: "TRAFFIC_INTERVAL_INVALID"},
		{name: "retention", path: "1?range=custom&start=2000-01-01T00:00:00Z", status: 400, reason: "TRAFFIC_RETENTION_EXCEEDED"},
		{name: "scoped-admin", path: "1?range=7d", status: 403, scoped: true},
		{name: "unavailable", path: "1?range=7d", status: 503, unavailable: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &trafficHandlerStore{saved: time.Now().UTC().Truncate(time.Second)}
			h := &ProxyHandler{wireguard: &service.WireGuardService{Store: store}}
			if tc.unavailable {
				h.wireguard = nil
			}
			router := gin.New()
			router.GET("/traffic/:id", h.WireGuardTraffic)
			req := httptest.NewRequest(http.MethodGet, "/traffic/"+tc.path, nil)
			if tc.scoped {
				req = req.WithContext(adminaccess.WithPrincipal(req.Context(), &adminaccess.Principal{}))
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			var envelope struct {
				Code     int                    `json:"code"`
				Reason   string                 `json:"reason"`
				Metadata map[string]string      `json:"metadata"`
				Data     wgtunnel.TrafficReport `json:"data"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
			require.Equal(t, tc.reason, envelope.Reason)
			if tc.status == 200 {
				require.True(t, store.called)
				require.Equal(t, 0, envelope.Code)
				require.Equal(t, "7d", envelope.Data.Range)
				require.Equal(t, "hour", envelope.Data.Interval)
				require.Equal(t, "9007199254741000", envelope.Data.Total)
				require.True(t, envelope.Data.Incomplete)
				require.True(t, envelope.Data.SavedAt.Equal(store.saved))
				require.Contains(t, rec.Body.String(), `"tx_bytes":"9007199254740993"`)
			} else {
				require.False(t, store.called)
			}
			if tc.reason == "TRAFFIC_RETENTION_EXCEEDED" {
				require.NotEmpty(t, envelope.Metadata["retained_from"])
			}
			if tc.reason == "TRAFFIC_INTERVAL_INVALID" {
				require.Empty(t, envelope.Metadata)
			}
		})
	}
}
