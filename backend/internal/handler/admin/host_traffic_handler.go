package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/hosttraffic"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
	"github.com/gin-gonic/gin"
)

func (h *OpsHandler) hostTrafficAccess(c *gin.Context) bool {
	if adminaccess.FromContext(c.Request.Context()) != nil {
		response.Error(c, http.StatusForbidden, "Host traffic requires a full administrator")
		return false
	}
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Host traffic unavailable")
		return false
	}
	if e := h.opsService.RequireMonitoringEnabled(c.Request.Context()); e != nil {
		response.ErrorFrom(c, e)
		return false
	}
	if h.serviceTraffic == nil || h.serviceTraffic.Host == nil || h.serviceTraffic.HostCollector == nil {
		response.Error(c, http.StatusServiceUnavailable, "Host traffic unavailable")
		return false
	}
	return true
}
func decodeHostTraffic(c *gin.Context, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 8192))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return false
	}
	var extra any
	return d.Decode(&extra) == io.EOF
}
func (h *OpsHandler) hostTrafficStatus(c *gin.Context) {
	st, e := h.serviceTraffic.Host.Status(c.Request.Context(), time.Now().UTC(), h.serviceTraffic.HostCollector.Health())
	if e != nil {
		response.InternalError(c, "Unable to read host traffic")
		return
	}
	cfg, e := h.opsService.GetEmailNotificationConfig(c.Request.Context())
	if e != nil {
		response.InternalError(c, "Unable to read alert settings")
		return
	}
	st.Alerts.EmailEnabled = cfg.Alert.Enabled
	response.Success(c, st)
}
func (h *OpsHandler) GetHostTraffic(c *gin.Context) {
	if !h.hostTrafficAccess(c) {
		return
	}
	h.hostTrafficStatus(c)
}
func (h *OpsHandler) UpdateHostTrafficPlan(c *gin.Context) {
	if !h.hostTrafficAccess(c) {
		return
	}
	var in hosttraffic.PlanInput
	if !decodeHostTraffic(c, &in) {
		response.ErrorWithDetails(c, 400, "Invalid traffic plan", "HOST_TRAFFIC_PLAN_INVALID", map[string]string{"field": "quota"})
		return
	}
	if e := h.serviceTraffic.Host.UpdatePlan(c.Request.Context(), in, time.Now().UTC()); e != nil {
		response.ErrorFrom(c, e)
		return
	}
	h.hostTrafficStatus(c)
}
func (h *OpsHandler) CalibrateHostTraffic(c *gin.Context) {
	if !h.hostTrafficAccess(c) {
		return
	}
	var in hosttraffic.CalibrationInput
	if !decodeHostTraffic(c, &in) {
		response.ErrorFrom(c, hosttraffic.ErrCalibration)
		return
	}
	kind, e := h.serviceTraffic.Host.Calibrate(c.Request.Context(), in, time.Now().UTC())
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	st, e := h.serviceTraffic.Host.Status(c.Request.Context(), time.Now().UTC(), h.serviceTraffic.HostCollector.Health())
	if e != nil {
		response.InternalError(c, "Unable to read host traffic")
		return
	}
	cfg, e := h.opsService.GetEmailNotificationConfig(c.Request.Context())
	if e != nil {
		response.InternalError(c, "Unable to read alert settings")
		return
	}
	st.Alerts.EmailEnabled = cfg.Alert.Enabled
	response.Success(c, gin.H{"kind": kind, "status": st})
}
func (h *OpsHandler) GetHostTrafficUsage(c *gin.Context) {
	if !h.hostTrafficAccess(c) {
		return
	}
	for key := range c.Request.URL.Query() {
		if key != "range" && key != "timezone" {
			response.BadRequest(c, "Unsupported host traffic filter")
			return
		}
	}
	usage, e := h.serviceTraffic.Host.Usage(c.Request.Context(), c.Query("range"), time.Now().UTC(), h.serviceTraffic.HostCollector.Health())
	if e != nil {
		if q, ok := e.(*wgtunnel.TrafficQueryError); ok {
			response.ErrorWithDetails(c, 400, q.Message, q.Reason, nil)
		} else {
			response.ErrorFrom(c, e)
		}
		return
	}
	response.Success(c, usage)
}
func (h *OpsHandler) GetHostTrafficDetail(c *gin.Context) {
	if !h.hostTrafficAccess(c) {
		return
	}
	detail, e := h.serviceTraffic.Host.Detail(c.Request.Context(), time.Now().UTC())
	if e != nil {
		response.InternalError(c, "Unable to read host traffic detail")
		return
	}
	response.Success(c, detail)
}
