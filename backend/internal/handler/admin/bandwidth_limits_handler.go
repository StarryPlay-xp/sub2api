package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *OpsHandler) bandwidthLimitsAccess(c *gin.Context) bool {
	if adminaccess.FromContext(c.Request.Context()) != nil {
		response.Error(c, http.StatusForbidden, "Bandwidth settings require a full administrator")
		return false
	}
	if h.serviceTraffic == nil || h.serviceTraffic.Limits == nil {
		response.Error(c, http.StatusServiceUnavailable, "Bandwidth settings unavailable")
		return false
	}
	return true
}

func (h *OpsHandler) GetBandwidthLimits(c *gin.Context) {
	if !h.bandwidthLimitsAccess(c) {
		return
	}
	result, err := h.serviceTraffic.Limits.ReadBandwidthLimits(c.Request.Context())
	if err != nil {
		response.InternalError(c, "Unable to read bandwidth settings")
		return
	}
	response.Success(c, result)
}

func (h *OpsHandler) UpdateBandwidthLimit(c *gin.Context) {
	if !h.bandwidthLimitsAccess(c) {
		return
	}
	var nodeID int64
	if raw := c.Param("id"); raw != "" {
		var err error
		nodeID, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || nodeID <= 0 {
			response.ErrorFrom(c, service.ErrBandwidthLimitInvalid)
			return
		}
	}
	var input struct {
		Mbps    json.RawMessage `json:"mbps"`
		Version *string         `json:"version"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 4096))
	decoder.DisallowUnknownFields()
	var trailing any
	if err := decoder.Decode(&input); err != nil || len(input.Mbps) == 0 || input.Version == nil {
		response.ErrorFrom(c, service.ErrBandwidthLimitInvalid)
		return
	}
	if err := decoder.Decode(&trailing); err != io.EOF {
		response.ErrorFrom(c, service.ErrBandwidthLimitInvalid)
		return
	}
	var mbps *float64
	if err := json.Unmarshal(input.Mbps, &mbps); err != nil {
		response.ErrorFrom(c, service.ErrBandwidthLimitInvalid)
		return
	}
	if err := service.ValidateBandwidthLimit(nodeID, *input.Version, mbps); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	result, err := h.serviceTraffic.Limits.UpdateBandwidthLimit(c.Request.Context(), nodeID, *input.Version, mbps)
	if err != nil {
		if errors.Is(err, service.ErrBandwidthLimitConflict) || errors.Is(err, service.ErrBandwidthNodeNotFound) || errors.Is(err, service.ErrBandwidthLimitInvalid) {
			response.ErrorFrom(c, err)
		} else {
			response.InternalError(c, "Unable to save bandwidth settings")
		}
		return
	}
	response.Success(c, result)
}
