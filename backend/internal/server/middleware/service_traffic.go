package middleware

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/gin-gonic/gin"
)

// ServiceTraffic is attached to registered gateway routes, before auth/limits.
// Readers count consumed bytes (not Content-Length); writers count accepted
// bytes. Embedding preserves Gin's Flush/Hijack/CloseNotify/Pusher interfaces.
func ServiceTraffic(m *traffic.Meter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if m == nil {
			c.Next()
			return
		}
		c.Request = c.Request.WithContext(traffic.WithMeter(c.Request.Context(), m))
		c.Request.Body = traffic.Body(c.Request.Body, m, traffic.ClientIn)
		original := c.Writer
		c.Writer = &trafficWriter{ResponseWriter: original, meter: m}
		defer func() { c.Writer = original }()
		c.Next()
	}
}

type trafficWriter struct {
	gin.ResponseWriter
	meter *traffic.Meter
}

func (w *trafficWriter) Write(p []byte) (int, error) {
	n, e := w.ResponseWriter.Write(p)
	w.meter.Add(traffic.ClientOut, n)
	return n, e
}
func (w *trafficWriter) WriteString(p string) (int, error) {
	n, e := w.ResponseWriter.WriteString(p)
	w.meter.Add(traffic.ClientOut, n)
	return n, e
}
func (w *trafficWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
