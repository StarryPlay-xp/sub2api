package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestServiceTrafficExcludesPanelAndPreservesFlush(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now()
	m := traffic.NewMeter("ingress", now)
	r := gin.New()
	r.POST("/v1/responses", ServiceTraffic(m), func(c *gin.Context) {
		_, _ = io.Copy(io.Discard, c.Request.Body)
		_, _ = c.Writer.WriteString("data: ")
		c.Writer.Flush()
		_, _ = c.Writer.Write([]byte("hello\n\n"))
	})
	r.GET("/api/v1/admin/accounts", func(c *gin.Context) { c.String(200, "not counted") })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/v1/responses", strings.NewReader("request")))
	require.Equal(t, 200, w.Code)
	require.True(t, w.Flushed)
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts", nil))
	m.Sample(now.Add(time.Second))
	require.Equal(t, 7.0, m.Current().Rates["client_in"])
	require.Equal(t, 13.0, m.Current().Rates["client_out"])
}
