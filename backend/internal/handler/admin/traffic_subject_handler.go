package admin

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func trafficParameterError(c *gin.Context, reason, message string) {
	response.ErrorWithDetails(c, http.StatusBadRequest, message, reason, nil)
}
func (h *OpsHandler) trafficSubjectAccess(c *gin.Context) bool {
	if adminaccess.FromContext(c.Request.Context()) != nil {
		response.Error(c, http.StatusForbidden, "Traffic attribution requires a full administrator")
		return false
	}
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Traffic attribution unavailable")
		return false
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return false
	}
	if h.serviceTraffic == nil || h.serviceTraffic.Subjects == nil || h.serviceTraffic.Collector == nil || h.serviceTraffic.Collector.Meter == nil {
		response.Error(c, http.StatusServiceUnavailable, "Traffic attribution unavailable")
		return false
	}
	return true
}
func trafficAllowedParameters(c *gin.Context, allowed ...string) bool {
	names := map[string]bool{"timezone": true}
	for _, k := range allowed {
		names[k] = true
	}
	for key, values := range c.Request.URL.Query() {
		if !names[key] || len(values) != 1 {
			trafficParameterError(c, "TRAFFIC_FILTER_INVALID", "Unsupported or repeated traffic parameter")
			return false
		}
	}
	return true
}
func trafficSubjectType(c *gin.Context, value string) bool {
	if value != "user" && value != "api_key" {
		trafficParameterError(c, "TRAFFIC_SUBJECT_INVALID", "subject must be user or api_key")
		return false
	}
	return true
}
func trafficInt(c *gin.Context, key string, fallback, max int, reason string) (int, bool) {
	raw, exists := c.GetQuery(key)
	if !exists {
		if _, present := c.Request.URL.Query()[key]; !present {
			return fallback, true
		}
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || max > 0 && value > max {
		trafficParameterError(c, reason, "Invalid "+key)
		return 0, false
	}
	return value, true
}
func trafficSubjectWindow(c *gin.Context) (wgtunnel.TrafficWindow, bool) {
	interval := c.DefaultQuery("interval", "hour")
	if interval != "hour" {
		trafficParameterError(c, "TRAFFIC_INTERVAL_INVALID", "Subject traffic only supports hour buckets")
		return wgtunnel.TrafficWindow{}, false
	}
	w, err := wgtunnel.ResolveTrafficWindow(wgtunnel.TrafficQuery{Range: c.Query("range"), Interval: interval, Start: c.Query("start"), End: c.Query("end")}, time.Now())
	if err != nil {
		var invalid *wgtunnel.TrafficQueryError
		if errors.As(err, &invalid) {
			trafficParameterError(c, invalid.Reason, invalid.Message)
		} else {
			trafficParameterError(c, "TRAFFIC_RANGE_INVALID", "Invalid traffic window")
		}
		return w, false
	}
	return w, true
}
func trafficCompare(c *gin.Context) (bool, bool) {
	value, exists := c.Request.URL.Query()["compare"]
	if exists && (len(value) != 1 || value[0] != "previous") {
		trafficParameterError(c, "TRAFFIC_FILTER_INVALID", "compare must be previous")
		return false, false
	}
	return exists, true
}
func trafficSubjectResult(c *gin.Context, result any, err error) {
	if errors.Is(err, service.ErrTrafficSubjectNotFound) {
		response.ErrorWithDetails(c, http.StatusNotFound, "Traffic subject not found", "TRAFFIC_SUBJECT_NOT_FOUND", nil)
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Unable to read traffic attribution")
		return
	}
	response.Success(c, result)
}
func (h *OpsHandler) GetTrafficRanking(c *gin.Context) {
	if !h.trafficSubjectAccess(c) || !trafficAllowedParameters(c, "subject", "range", "start", "end", "interval", "sort", "order", "page", "page_size", "q", "user_id", "compare") {
		return
	}
	q := service.TrafficSubjectQuery{Subject: c.Query("subject"), Sort: c.DefaultQuery("sort", "total_bytes"), Order: c.DefaultQuery("order", "desc"), Q: c.Query("q")}
	if !trafficSubjectType(c, q.Subject) {
		return
	}
	validSort := map[string]bool{"total_bytes": true, "in_bytes": true, "out_bytes": true, "requests": true, "bytes_per_request": true, "peak_total": true, "peak_in": true, "peak_out": true, "active_avg_rate": true, "growth": true}
	if !validSort[q.Sort] || q.Order != "asc" && q.Order != "desc" {
		trafficParameterError(c, "TRAFFIC_SORT_INVALID", "Invalid sort or order")
		return
	}
	var ok bool
	q.Compare, ok = trafficCompare(c)
	if !ok {
		return
	}
	if q.Sort == "growth" && !q.Compare {
		trafficParameterError(c, "TRAFFIC_COMPARE_REQUIRED", "growth requires compare=previous")
		return
	}
	q.Page, ok = trafficInt(c, "page", 1, 0, "TRAFFIC_PAGE_INVALID")
	if !ok {
		return
	}
	q.PageSize, ok = trafficInt(c, "page_size", 20, 200, "TRAFFIC_PAGE_INVALID")
	if !ok {
		return
	}
	if raw, exists := c.Request.URL.Query()["user_id"]; exists {
		var err error
		q.UserID, err = strconv.ParseInt(raw[0], 10, 64)
		if q.Subject != "api_key" || err != nil || q.UserID < 1 {
			trafficParameterError(c, "TRAFFIC_FILTER_INVALID", "user_id requires subject=api_key and a positive ID")
			return
		}
	}
	q.Window, ok = trafficSubjectWindow(c)
	if !ok {
		return
	}
	result, err := h.serviceTraffic.Ranking(c.Request.Context(), q)
	trafficSubjectResult(c, result, err)
}
func (h *OpsHandler) GetTrafficSubject(c *gin.Context) {
	if !h.trafficSubjectAccess(c) || !trafficAllowedParameters(c, "range", "start", "end", "interval", "compare") {
		return
	}
	q := service.TrafficSubjectQuery{Subject: c.Param("type")}
	if !trafficSubjectType(c, q.Subject) {
		return
	}
	var err error
	q.ID, err = strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || q.ID < 1 {
		trafficParameterError(c, "TRAFFIC_SUBJECT_INVALID", "Subject ID must be positive")
		return
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
	result, err := h.serviceTraffic.SubjectDetail(c.Request.Context(), q)
	trafficSubjectResult(c, result, err)
}
func (h *OpsHandler) GetTrafficLive(c *gin.Context) {
	if !h.trafficSubjectAccess(c) || !trafficAllowedParameters(c, "subject", "limit", "sort") {
		return
	}
	subject, order := c.Query("subject"), c.DefaultQuery("sort", "avg_60s")
	if !trafficSubjectType(c, subject) {
		return
	}
	if order != "avg_60s" && order != "rate" {
		trafficParameterError(c, "TRAFFIC_SORT_INVALID", "sort must be avg_60s or rate")
		return
	}
	limit, ok := trafficInt(c, "limit", 10, 50, "TRAFFIC_PAGE_INVALID")
	if !ok {
		return
	}
	result, err := h.serviceTraffic.LiveSubjects(c.Request.Context(), subject, order, limit)
	trafficSubjectResult(c, result, err)
}
