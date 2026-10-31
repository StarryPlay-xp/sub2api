//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type egressAuthorizationRepo struct {
	accountRepoStubForBulkUpdate
	checked          bool
	allowed          bool
	unrelatedAllowed bool
}

func (r *egressAuthorizationRepo) BulkUpdate(ctx context.Context, ids []int64, _ AccountBulkUpdate) (int64, error) {
	r.checked = true
	r.allowed = len(ids) == 1 && IsExplicitEgressEdit(ctx, ids[0])
	r.unrelatedAllowed = IsExplicitEgressEdit(ctx, 999)
	return 0, errors.New("stop after inspecting write authorization")
}

func TestBulkEgressAuthorization(t *testing.T) {
	for _, tc := range []struct {
		name                                 string
		ctx                                  context.Context
		filtered, proxyEdit, shadow, allowed bool
	}{
		{"explicit IDs", WithExplicitEgressEdit(context.Background(), 77), false, true, false, true},
		{"filtered admin", WithExplicitFilteredEgressEdit(context.Background()), true, true, false, true},
		{"background filters", context.Background(), true, true, false, false},
		{"background IDs", context.Background(), false, true, false, false},
		{"empty ID authorization is not a wildcard", WithExplicitEgressEdit(context.Background()), true, true, false, false},
		{"different IDs cannot expand authorization", WithExplicitEgressEdit(context.Background(), 66), true, true, false, false},
		{"filter capability cannot authorize explicit IDs", WithExplicitFilteredEgressEdit(context.Background()), false, true, false, false},
		{"filter without proxy edit", WithExplicitFilteredEgressEdit(context.Background()), true, false, false, false},
		{"filtered shadow stays protected", WithExplicitFilteredEgressEdit(context.Background()), true, true, true, false},
		{"explicit shadow stays protected", WithExplicitEgressEdit(context.Background(), 77), false, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := Account{ID: 77, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
			if tc.shadow {
				parent := int64(10)
				account.ParentAccountID = &parent
			}
			repo := &egressAuthorizationRepo{accountRepoStubForBulkUpdate: accountRepoStubForBulkUpdate{
				listData: []Account{account}, listResult: &pagination.PaginationResult{Total: 1}, getByIDsAccounts: []*Account{&account},
			}}
			svc := &adminServiceImpl{accountRepo: repo}
			input := &BulkUpdateAccountsInput{Name: "edited"}
			if tc.filtered {
				input.Filters = &BulkUpdateAccountFilters{Platform: PlatformOpenAI}
			} else {
				input.AccountIDs = []int64{77}
			}
			if tc.proxyEdit {
				direct := int64(0)
				input.ProxyID = &direct
			}
			_, err := svc.BulkUpdateAccounts(tc.ctx, input)
			if tc.shadow {
				require.Error(t, err)
				require.False(t, repo.checked)
				return
			}
			require.True(t, repo.checked)
			require.Equal(t, tc.allowed, repo.allowed)
			require.False(t, repo.unrelatedAllowed)
			if tc.filtered {
				require.False(t, IsExplicitEgressEdit(tc.ctx, 77), "input context must not gain resolved permissions")
			}
		})
	}
}
