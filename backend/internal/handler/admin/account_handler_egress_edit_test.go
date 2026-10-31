package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Exercise the actual handler -> service target resolution -> repository
// boundary. The repository records the resulting per-account authorization.
type bulkEgressHandlerRepo struct {
	service.AdminAccountRepository
	called  bool
	allowed bool
}

func (r *bulkEgressHandlerRepo) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, string, int64, string) ([]service.Account, *pagination.PaginationResult, error) {
	return []service.Account{{ID: 77, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey}}, &pagination.PaginationResult{Total: 1}, nil
}
func (r *bulkEgressHandlerRepo) GetByIDs(context.Context, []int64) ([]*service.Account, error) {
	return []*service.Account{{ID: 77, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey}}, nil
}
func (r *bulkEgressHandlerRepo) BulkUpdate(ctx context.Context, ids []int64, _ service.AccountBulkUpdate) (int64, error) {
	r.called = true
	r.allowed = len(ids) == 1 && service.IsExplicitEgressEdit(ctx, 77) && !service.IsExplicitEgressEdit(ctx, 999)
	return int64(len(ids)), nil
}
func TestBulkHandlerResolvesExplicitEgressAuthorization(t *testing.T) {
	for _, body := range []string{`{"account_ids":[77],"proxy_id":0}`, `{"filters":{"platform":"openai"},"proxy_id":0}`} {
		t.Run(body, func(t *testing.T) {
			repo := &bulkEgressHandlerRepo{}
			svc := service.NewAdminService(nil, nil, nil, repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
			h := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.POST("/accounts/bulk-update", h.BulkUpdate)
			req := httptest.NewRequest(http.MethodPost, "/accounts/bulk-update", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.True(t, repo.called)
			require.True(t, repo.allowed)
		})
	}
}

func (r *bulkEgressHandlerRepo) ListShadowsByParent(context.Context, int64) ([]*service.Account, error) {
	return nil, nil
}
