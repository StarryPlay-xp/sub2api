package admin

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *OpsHandler) trafficEgressAccess(c *gin.Context) bool {
	if adminaccess.FromContext(c.Request.Context()) != nil {
		response.Error(c, http.StatusForbidden, "Egress traffic requires a full administrator")
		return false
	}
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Egress traffic unavailable")
		return false
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return false
	}
	if h.serviceTraffic == nil || h.serviceTraffic.Egress == nil || h.serviceTraffic.Collector == nil || h.serviceTraffic.Collector.Meter == nil {
		response.Error(c, http.StatusServiceUnavailable, "Egress traffic unavailable")
		return false
	}
	return true
}
func (h *OpsHandler) GetTrafficEgress(c *gin.Context)       { h.getTrafficEgress(c, false) }
func (h *OpsHandler) GetTrafficEgressDetail(c *gin.Context) { h.getTrafficEgress(c, true) }
func (h *OpsHandler) getTrafficEgress(c *gin.Context, detail bool) {
	if !h.trafficEgressAccess(c) || !trafficAllowedParameters(c, "range", "start", "end", "interval", "compare") {
		return
	}
	q := service.TrafficEgressQuery{Detail: detail}
	if detail {
		q.RouteType = c.Param("route_type")
		id, err := strconv.ParseInt(c.Param("route_id"), 10, 64)
		q.RouteID = id
		valid := err == nil && ((q.RouteType == "direct" && id == 0) || ((q.RouteType == "proxy" || q.RouteType == "wireguard") && id > 0))
		if !valid {
			trafficParameterError(c, "TRAFFIC_ROUTE_INVALID", "Use direct/0 or proxy|wireguard with a positive ID")
			return
		}
	}
	var ok bool
	q.Compare, ok = trafficCompare(c)
	if !ok {
		return
	}
	q.Window, ok = trafficSubjectWindow(c)
	if !ok {
		return
	}
	var result any
	var err error
	if detail {
		result, err = h.serviceTraffic.EgressDetail(c.Request.Context(), q)
	} else {
		result, err = h.serviceTraffic.EgressReport(c.Request.Context(), q)
	}
	if errors.Is(err, service.ErrTrafficRouteNotFound) {
		response.ErrorWithDetails(c, http.StatusNotFound, "Traffic route not found", "TRAFFIC_ROUTE_NOT_FOUND", nil)
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Unable to read egress traffic")
		return
	}
	response.Success(c, result)
}
