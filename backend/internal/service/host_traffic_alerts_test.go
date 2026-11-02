//go:build unit

package service

import (
	"context"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type hostTrafficAlertRepo struct {
	opsRepoMock
	rules []*OpsAlertRule
}

func (r *hostTrafficAlertRepo) ListAlertRules(context.Context) ([]*OpsAlertRule, error) {
	return r.rules, nil
}
func TestHostTrafficManagedAlertsAndEmailSeverity(t *testing.T) {
	repo := &hostTrafficAlertRepo{rules: []*OpsAlertRule{{ID: 1, Name: "host_traffic:usage_warning", ManagedBy: "host_traffic:usage_warning"}, {ID: 2, Name: "manual"}}}
	svc := &OpsService{opsRepo: repo}
	ctx := context.Background()
	_, err := svc.UpdateAlertRule(ctx, &OpsAlertRule{ID: 1, Name: "host_traffic:usage_warning"})
	require.Equal(t, "OPS_ALERT_RULE_MANAGED", infraerrors.Reason(err))
	require.Equal(t, "OPS_ALERT_RULE_MANAGED", infraerrors.Reason(svc.DeleteAlertRule(ctx, 1)))
	_, err = svc.UpdateAlertRule(ctx, &OpsAlertRule{ID: 2, Name: "manual"})
	require.NoError(t, err)
	require.NoError(t, svc.DeleteAlertRule(ctx, 2))
	_, err = svc.CreateAlertRule(ctx, &OpsAlertRule{Name: " host_traffic:projection"})
	require.Error(t, err)
	require.Equal(t, "warning", opsEmailSeverityForOps("P1"))
	require.Equal(t, "critical", opsEmailSeverityForOps("P0"))
	require.True(t, shouldSendOpsAlertEmailByMinSeverity("critical", "P0"))
	require.False(t, shouldSendOpsAlertEmailByMinSeverity("critical", "P1"))
}
