//go:build unit

package middleware

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestServiceTrafficActualAuthBranches(t *testing.T) {
	for _, mode := range []string{config.RunModeSimple, config.RunModeStandard} {
		for _, google := range []bool{false, true} {
			name := mode + "/normal"
			if google {
				name = mode + "/google"
			}
			t.Run(name, func(t *testing.T) {
				cfg := &config.Config{RunMode: mode}
				key := &service.APIKey{ID: 11, UserID: 1, Key: "synthetic", Status: service.StatusActive, User: &service.User{ID: 1, Role: service.RoleUser, Status: service.StatusActive, Balance: 10, Concurrency: 3}}
				repo := &stubApiKeyRepo{getByKey: func(_ context.Context, value string) (*service.APIKey, error) {
					if value != key.Key {
						return nil, service.ErrAPIKeyNotFound
					}
					copy := *key
					return &copy, nil
				}}
				auth := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)
				now := time.Now().UTC().Truncate(time.Hour)
				m := traffic.NewMeter("auth-branches", now)
				router := gin.New()
				router.Use(ServiceTraffic(m))
				header := "x-api-key"
				if google {
					router.Use(APIKeyAuthWithSubscriptionGoogle(auth, nil, cfg))
					header = "x-goog-api-key"
				} else {
					router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(auth, nil, cfg)))
				}
				router.GET("/t", func(c *gin.Context) { c.String(200, "accepted") })
				recorder := httptest.NewRecorder()
				req := httptest.NewRequest("GET", "/t", nil)
				req.Header.Set(header, key.Key)
				router.ServeHTTP(recorder, req)
				require.Equal(t, 200, recorder.Code, recorder.Body.String())
				denied := httptest.NewRecorder()
				router.ServeHTTP(denied, httptest.NewRequest("GET", "/t", nil))
				require.Equal(t, 401, denied.Code)
				m.Sample(now.Add(time.Second))
				var observed bool
				for _, b := range m.Snapshot(false).Subjects {
					if b.Type == "api_key" {
						observed = true
						require.Equal(t, int64(11), b.ID)
						require.Equal(t, uint64(8), b.Bytes.ClientOut)
						require.Equal(t, uint64(1), b.Requests)
					}
					if b.Type == "unattributed" {
						require.Positive(t, b.Bytes.ClientOut)
						require.Zero(t, b.Requests)
					}
				}
				require.True(t, observed)
			})
		}
	}
}
