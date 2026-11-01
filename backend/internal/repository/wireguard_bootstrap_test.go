package repository

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
	"github.com/stretchr/testify/require"
)

func bootstrapPlanFile(t *testing.T) string {
	t.Helper()
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 32)))
	config := "[Interface]\nPrivateKey=" + key + "\nAddress=10.0.0.2/32\nDNS=1.1.1.1\n[Peer]\nPublicKey=" + key + "\nEndpoint=192.0.2.1:51820\nAllowedIPs=0.0.0.0/0\n"
	data, err := json.Marshal(wireGuardBootstrapPlan{Name: "bootstrap-node", AccountIDs: []int64{1, 2, 3}, Config: config})
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(path, data, 0600))
	return path
}

func TestWireGuardBootstrapNeverReplaysCompletedPlan(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT value FROM settings").WithArgs(wireGuardBootstrapKey).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`{"node_id":7,"account_ids":[1,2,3]}`))
	mock.ExpectRollback()
	s := &WireGuardStore{DB: db}
	// A removed private file and later account edits must not trigger a rebind.
	box, err := wgtunnel.NewSecretBox(make([]byte, 32))
	require.NoError(t, err)
	require.NoError(t, s.BootstrapWireGuard(context.Background(), "/missing/private-plan", box))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWireGuardBootstrapTransactionAndFailureRollback(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "success"
		if fail {
			name = "binding_rejected"
		}
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			box, err := wgtunnel.NewSecretBox(make([]byte, 32))
			require.NoError(t, err)
			mock.ExpectBegin()
			mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("SELECT value FROM settings").WillReturnError(sql.ErrNoRows)
			mock.ExpectExec("LOCK TABLE accounts").WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectQuery("SELECT id,proxy_id,parent_account_id").WillReturnRows(sqlmock.NewRows([]string{"id", "proxy_id", "parent_account_id"}).AddRow(1, nil, nil).AddRow(2, nil, nil).AddRow(3, nil, nil))
			mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
			mock.ExpectQuery("INSERT INTO proxies").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
			mock.ExpectExec("INSERT INTO wireguard_nodes").WithArgs(int64(7), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
			for i := 1; i <= 3; i++ {
				mock.ExpectExec("SELECT set_config").WillReturnResult(sqlmock.NewResult(0, 1))
				update := mock.ExpectExec("UPDATE accounts SET proxy_id").WithArgs(int64(7), int64(i))
				if fail && i == 2 {
					update.WillReturnError(sql.ErrConnDone)
					break
				}
				update.WillReturnResult(sqlmock.NewResult(0, 1))
			}
			if fail {
				mock.ExpectRollback()
			} else {
				mock.ExpectExec("INSERT INTO settings").WithArgs(wireGuardBootstrapKey, `{"node_id":7,"account_ids":[1,2,3]}`).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			s := &WireGuardStore{DB: db}
			err = s.BootstrapWireGuard(context.Background(), bootstrapPlanFile(t), box)
			if fail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestWireGuardBootstrapRejectsUnexpectedAccounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	box, err := wgtunnel.NewSecretBox(make([]byte, 32))
	require.NoError(t, err)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT value FROM settings").WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("LOCK TABLE accounts").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT id,proxy_id,parent_account_id").WillReturnRows(sqlmock.NewRows([]string{"id", "proxy_id", "parent_account_id"}).AddRow(1, nil, nil).AddRow(2, nil, nil).AddRow(4, nil, nil))
	mock.ExpectRollback()
	err = (&WireGuardStore{DB: db}).BootstrapWireGuard(context.Background(), bootstrapPlanFile(t), box)
	require.ErrorContains(t, err, "account set changed")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWireGuardBootstrapRejectsWorldReadableFile(t *testing.T) {
	path := bootstrapPlanFile(t)
	require.NoError(t, os.Chmod(path, 0644))
	_, err := readWireGuardBootstrapPlan(path)
	require.ErrorContains(t, err, "owner-only")
}
