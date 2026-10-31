//go:build unit

package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestImageUpdateHandlerUnconfiguredStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewSystemHandler(nil, nil)
	r := gin.New()
	r.GET("/status", h.ImageUpdateStatus)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/status", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"enabled":false`)
	require.Contains(t, w.Body.String(), `"phase":"unconfigured"`)
}

func TestImageUpdateHandlerRejectsCachedFallbackAndUnconfiguredHost(t *testing.T) {
	t.Setenv("SUB2API_DEPLOYMENT", "docker")
	for _, warning := range []string{"Using cached data", ""} {
		svc := &systemHandlerUpdateServiceStub{updateInfo: &service.UpdateInfo{HasUpdate: true, LatestVersion: "1.2.4", Warning: warning}}
		h := NewSystemHandler(svc, nil)
		r := gin.New()
		r.POST("/update", h.StartImageUpdate)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/update", nil))
		if warning != "" {
			require.Equal(t, http.StatusConflict, w.Code)
		} else {
			require.Equal(t, http.StatusServiceUnavailable, w.Code)
		}
		require.Equal(t, []bool{true}, svc.checkForces)
		require.Zero(t, svc.performCall)
	}
}

func TestImageUpdateHandlerBlocksIndependentRestart(t *testing.T) {
	t.Setenv("SUB2API_DEPLOYMENT", "docker")
	h := NewSystemHandler(nil, nil)
	r := gin.New()
	r.POST("/restart", h.RestartService)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/restart", nil))
	require.Equal(t, http.StatusConflict, w.Code)
}

func TestImageUpdateHandlerRejectsScopedAdmin(t *testing.T) {
	h := NewSystemHandler(nil, nil)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(adminaccess.WithPrincipal(c.Request.Context(), &adminaccess.Principal{}))
		c.Next()
	})
	r.GET("/status", h.ImageUpdateStatus)
	r.POST("/update", h.StartImageUpdate)
	for _, test := range []struct{ method, path string }{{http.MethodGet, "/status"}, {http.MethodPost, "/update"}} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(test.method, test.path, nil))
		require.Equal(t, http.StatusForbidden, w.Code)
	}
}
