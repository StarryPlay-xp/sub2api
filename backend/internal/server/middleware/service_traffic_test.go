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

func TestServiceTrafficAttributionAndRejectedRequest(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	m := traffic.NewMeter("auth", now)
	r := gin.New()
	r.POST("/ok", ServiceTraffic(m), func(c *gin.Context) {
		traffic.BindContext(c.Request.Context(), 1, 11)
		_, _ = io.Copy(io.Discard, c.Request.Body)
		c.String(200, "accepted")
	})
	r.POST("/denied", ServiceTraffic(m), func(c *gin.Context) { c.AbortWithStatusJSON(401, gin.H{"error": "denied"}) })
	for _, path := range []string{"/ok", "/denied"} {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", path, strings.NewReader("input")))
	}
	m.Sample(now.Add(time.Second))
	var key, un uint64
	for _, b := range m.Snapshot(false).Subjects {
		if b.Type == "api_key" {
			key += b.Bytes.Vector()[6]
			require.Equal(t, uint64(1), b.Requests)
		}
		if b.Type == "unattributed" {
			un += b.Bytes.Vector()[6]
			require.Zero(t, b.Requests)
		}
	}
	require.Equal(t, uint64(13), key)
	require.Positive(t, un)
	require.Equal(t, int64(0), m.Live("user").Items[0].InFlight)
}
