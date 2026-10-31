//go:build integration

package repository

import "testing"

func TestWireGuardLifecycleMigrationReapply(t *testing.T) {
	db := wireguardTestDB(t)
	// Migration files promise idempotency, including their trigger definitions.
	applyWireGuardLifecycleMigration(t, db)
}
