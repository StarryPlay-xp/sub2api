package admin

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *SystemHandler) SetImageUpdater(s *service.ImageUpdateService) { h.imageUpdater = s }

func (h *SystemHandler) imageUpdateAccess(c *gin.Context) bool {
	if adminaccess.FromContext(c.Request.Context()) != nil {
		response.Error(c, http.StatusForbidden, "Image updates require a full administrator")
		return false
	}
	return true
}

func (h *SystemHandler) ImageUpdateStatus(c *gin.Context) {
	if !h.imageUpdateAccess(c) {
		return
	}
	status, err := h.imageUpdater.Status(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, err.Error())
		return
	}
	response.Success(c, status)
}

func (h *SystemHandler) StartImageUpdate(c *gin.Context) {
	if !h.imageUpdateAccess(c) {
		return
	}
	if service.SystemUpdateMode() != "image" {
		response.Error(c, http.StatusConflict, "Image updater requires Docker deployment")
		return
	}
	info, err := h.updateSvc.CheckUpdate(c.Request.Context(), true)
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "Cannot check release")
		return
	}
	if info.Warning != "" || !info.HasUpdate {
		response.Error(c, http.StatusConflict, "No verified newer release is available")
		return
	}
	// The host owns the durable single-job lock; it survives application replacement.
	status, err := h.imageUpdater.Start(c.Request.Context(), info.LatestVersion)
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, err.Error())
		return
	}
	response.Success(c, status)
}
