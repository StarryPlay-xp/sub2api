package admin

import (
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func NewTrafficOpsHandler(ops *service.OpsService, traffic *service.ServiceTrafficService) *OpsHandler {
	h := NewOpsHandler(ops)
	h.serviceTraffic = traffic
	return h
}
func (h *OpsHandler) GetServiceTraffic(c *gin.Context) {
	if adminaccess.FromContext(c.Request.Context()) != nil {
		response.Error(c, http.StatusForbidden, "Service traffic requires a full administrator")
		return
	}
	if h.serviceTraffic == nil || h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Service traffic unavailable")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	for k := range c.Request.URL.Query() {
		switch k {
		case "range", "interval", "start", "end", "timezone":
			// The shared panel client adds timezone to every GET. Traffic
			// buckets remain UTC-aligned; it is display metadata, not a filter.
		default:
			response.BadRequest(c, "Unsupported filter; service traffic is a whole-instance summary")
			return
		}
	}
	window, err := wgtunnel.ResolveTrafficWindow(wgtunnel.TrafficQuery{Range: c.Query("range"), Interval: c.Query("interval"), Start: c.Query("start"), End: c.Query("end")}, time.Now())
	if err != nil {
		if invalid, ok := err.(*wgtunnel.TrafficQueryError); ok {
			response.ErrorWithDetails(c, http.StatusBadRequest, invalid.Message, invalid.Reason, nil)
		} else {
			response.BadRequest(c, "Invalid traffic range")
		}
		return
	}
	report, err := h.serviceTraffic.Store.Report(c.Request.Context(), window)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Unable to read service traffic")
		return
	}
	saved, failed := h.serviceTraffic.Collector.Status()
	response.Success(c, gin.H{"report": report, "current": h.serviceTraffic.Collector.Meter.Current(), "last_persisted_at": saved, "persistence_error": failed})
}
