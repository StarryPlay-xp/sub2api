package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/egress"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"time"
)

func NewRoutedProxyHandler(adminService service.AdminService, wg *service.WireGuardService) *ProxyHandler {
	h := NewProxyHandler(adminService)
	h.wireguard = wg
	return h
}
func (h *ProxyHandler) wireGuardAccess(c *gin.Context) bool {
	if adminaccess.FromContext(c.Request.Context()) != nil {
		response.Error(c, 403, "WireGuard management requires a full administrator")
		return false
	}
	if h.wireguard == nil {
		response.Error(c, 503, "WireGuard service unavailable")
		return false
	}
	return true
}

type wireGuardWriteRequest struct {
	Name      string     `json:"name"`
	Config    string     `json:"config"`
	Version   int64      `json:"config_version"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// Never include this body in logging, exports, audit or idempotency records.
func (wireGuardWriteRequest) String() string   { return "[WireGuard configuration redacted]" }
func (wireGuardWriteRequest) GoString() string { return "[WireGuard configuration redacted]" }
func (h *ProxyHandler) WireGuardCapabilities(c *gin.Context) {
	if !h.wireGuardAccess(c) {
		return
	}
	response.Success(c, gin.H{"key_configured": h.wireguard.Box != nil, "metric_basis": wgtunnel.MetricBasis})
}
func (h *ProxyHandler) WriteWireGuard(c *gin.Context) {
	if !h.wireGuardAccess(c) {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 40<<10)
	var input wireGuardWriteRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid WireGuard input")
		return
	}
	defer func() { input.Config = "" }()
	value := wgtunnel.NodeInput{Name: input.Name, Config: input.Config, ExpiresAt: input.ExpiresAt}
	var result wgtunnel.NodeView
	var err error
	if c.Request.Method == http.MethodPost {
		result, err = h.wireguard.Create(c.Request.Context(), value)
	} else {
		id, parseErr := strconv.ParseInt(c.Param("id"), 10, 64)
		if parseErr != nil || id < 1 || input.Version < 1 {
			response.BadRequest(c, "Invalid node or configuration version")
			return
		}
		result, err = h.wireguard.Replace(c.Request.Context(), id, input.Version, value)
	}
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, result)
}
func (h *ProxyHandler) GetWireGuard(c *gin.Context) {
	if !h.wireGuardAccess(c) {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		response.BadRequest(c, "Invalid node")
		return
	}
	result, err := h.wireguard.Details(c.Request.Context(), id, c.Query("interval"))
	if err != nil {
		response.BadRequest(c, "Unable to read WireGuard node")
		return
	}
	response.Success(c, result)
}
func (h *ProxyHandler) SetWireGuardState(c *gin.Context) {
	if !h.wireGuardAccess(c) {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		response.BadRequest(c, "Invalid node")
		return
	}
	var input struct {
		Version int64 `json:"config_version"`
		Paused  *bool `json:"paused"`
	}
	if c.ShouldBindJSON(&input) != nil || input.Version < 1 || input.Paused == nil {
		response.BadRequest(c, "Version and explicit paused state are required")
		return
	}
	if err = h.wireguard.Pause(c.Request.Context(), id, input.Version, *input.Paused); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{"success": true})
}
func (h *ProxyHandler) ProbeWireGuard(c *gin.Context) {
	if !h.wireGuardAccess(c) {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		response.BadRequest(c, "Invalid node")
		return
	}
	family := c.DefaultQuery("family", "ipv4")
	if family != "ipv4" && family != "ipv6" {
		response.BadRequest(c, "Invalid address family")
		return
	}
	evidence, err := h.wireguard.Probe(c.Request.Context(), id, family)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, evidence)
}
func (h *ProxyHandler) EgressAttempts(c *gin.Context) {
	if !h.wireGuardAccess(c) {
		return
	}
	id := c.Query("request_id")
	if id == "" || len(id) > 128 {
		response.BadRequest(c, "request_id is required")
		return
	}
	attempts, err := h.wireguard.Store.ReadAttempts(c.Request.Context(), id)
	if err != nil {
		response.Error(c, 500, "Unable to read exit evidence")
		return
	}
	response.Success(c, attempts)
}

func (h *ProxyHandler) EgressSummaries(c *gin.Context) {
	if !h.wireGuardAccess(c) {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var input struct {
		Requests []egress.AttemptQuery `json:"requests"`
	}
	if c.ShouldBindJSON(&input) != nil || len(input.Requests) == 0 || len(input.Requests) > 200 {
		response.BadRequest(c, "Between 1 and 200 requests are required")
		return
	}
	for _, q := range input.Requests {
		if q.RequestID == "" || len(q.RequestID) > 128 || q.AccountID <= 0 {
			response.BadRequest(c, "Valid request and account IDs are required")
			return
		}
	}
	summaries, err := h.wireguard.Store.ReadExitSummaries(c.Request.Context(), input.Requests)
	if err != nil {
		response.Error(c, 500, "Unable to read exit IPs")
		return
	}
	response.Success(c, summaries)
}
