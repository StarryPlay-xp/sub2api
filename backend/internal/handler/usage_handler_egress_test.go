package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/egress"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type userExitReaderStub struct {
	queries []egress.AttemptQuery
	err     error
}

func (s *userExitReaderStub) ReadExitSummaries(_ context.Context, queries []egress.AttemptQuery) ([]egress.ExitSummary, error) {
	s.queries = queries
	if s.err != nil {
		return nil, s.err
	}
	result := make([]egress.ExitSummary, len(queries))
	for i, q := range queries {
		result[i] = egress.ExitSummary{AttemptQuery: q, IPs: []string{"203.0.113.9"}, Recorded: true}
	}
	return result, nil
}

func TestUserUsageExitIPsUseAuthorizedRows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &userUsageRepoCapture{listRows: []service.UsageLog{{ID: 1, UserID: 42, AccountID: 7, RequestID: "owned-request"}}}
	reader := &userExitReaderStub{}
	h := NewUsageHandler(service.NewUsageService(repo, nil, nil, nil), nil, nil, nil)
	h.exitReader = reader
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 42})
		c.Next()
	})
	router.GET("/usage", h.List)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/usage?user_id=99&request_id=other&account_id=99", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, int64(42), repo.listFilters.UserID)
	require.Equal(t, []egress.AttemptQuery{{RequestID: "owned-request", AccountID: 7}}, reader.queries)
	var payload struct {
		Data struct {
			Items []struct {
				ExitIP map[string]any `json:"exit_ip"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.Len(t, payload.Data.Items, 1)
	value := payload.Data.Items[0].ExitIP
	require.Equal(t, []any{"203.0.113.9"}, value["ips"])
	require.Len(t, value, 4, "user summary must not include account/node/runtime identifiers")
}

func TestUserUsageExitIPsSkipForeignRowsAndPreserveReadFailures(t *testing.T) {
	reader := &userExitReaderStub{err: errors.New("unavailable")}
	h := &UsageHandler{exitReader: reader}
	records := []service.UsageLog{{UserID: 42, AccountID: 7, RequestID: "owned"}, {UserID: 99, AccountID: 8, RequestID: "foreign"}, {UserID: 42}}
	out := make([]dto.UsageLog, len(records))
	h.fillExitIPs(context.Background(), 42, records, out)
	require.Equal(t, []egress.AttemptQuery{{RequestID: "owned", AccountID: 7}}, reader.queries)
	require.Nil(t, out[0].ExitIP, "failure must not be misrepresented as no historical observation")
	require.Nil(t, out[1].ExitIP)
	require.NotNil(t, out[2].ExitIP)
	require.False(t, out[2].ExitIP.Recorded)
}
