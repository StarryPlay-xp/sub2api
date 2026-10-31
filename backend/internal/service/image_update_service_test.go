package service

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
	"github.com/stretchr/testify/require"
)

func TestImageUpdateModeBlocksAllBinaryMutations(t *testing.T) {
	t.Setenv("SUB2API_DEPLOYMENT", "docker")
	s := &UpdateService{}
	require.ErrorIs(t, s.PerformUpdate(context.Background()), ErrImageUpdateRequired)
	require.ErrorIs(t, s.Rollback(), ErrImageUpdateRequired)
	require.ErrorIs(t, s.RollbackToVersion(context.Background(), "1.0.0"), ErrImageUpdateRequired)
}

func TestImageUpdaterUsesOnlyConfiguredUnixSocket(t *testing.T) {
	directory, err := os.MkdirTemp("/tmp", "siu-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	path := filepath.Join(directory, "u.sock")
	listener, err := net.Listen("unix", path)
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		_ = json.NewEncoder(w).Encode(ImageUpdateStatus{Enabled: true, Phase: "idle"})
	})
	mux.HandleFunc("/update", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, map[string]string{"version": "1.2.4"}, body)
		_ = json.NewEncoder(w).Encode(ImageUpdateStatus{Enabled: true, Phase: "queued", Version: body["version"]})
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	t.Setenv("IMAGE_UPDATER_SOCKET", path)
	s := NewImageUpdateService(nil, &WireGuardService{}, "1.2.3")
	t.Cleanup(s.Close)
	status, err := s.Status(context.Background())
	require.NoError(t, err)
	require.True(t, status.Enabled)
	status, err = s.Start(context.Background(), "1.2.4")
	require.NoError(t, err)
	require.Equal(t, "queued", status.Phase)
}

func TestImageUpdaterUnconfiguredDoesNotClaimSuccess(t *testing.T) {
	var s *ImageUpdateService
	status, err := s.Status(context.Background())
	require.NoError(t, err)
	require.False(t, status.Enabled)
	_, err = s.Start(context.Background(), "1.2.4")
	require.Error(t, err)
}

func TestImageUpdateVerificationRejectsMissingKeyAndMissingNode(t *testing.T) {
	for _, test := range []struct {
		name     string
		row      bool
		expected []int64
	}{
		{"missing_key", true, nil}, {"missing_node", false, []int64{7}}, {"no_nodes", false, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			rows := sqlmock.NewRows([]string{"proxy_id", "config_ciphertext"})
			if test.row {
				rows.AddRow(7, "encrypted-secret")
			}
			mock.ExpectQuery("SELECT proxy_id,config_ciphertext FROM wireguard_nodes").WillReturnRows(rows)
			s := &ImageUpdateService{db: db, wg: &WireGuardService{}}
			err = s.verifyWireGuard(context.Background(), test.expected)
			if test.name == "no_nodes" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "encrypted-secret")
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestImageUpdateVerificationRejectsWrongKey(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	oldKey, err := wgtunnel.NewSecretBox(make([]byte, 32))
	require.NoError(t, err)
	cipher, err := oldKey.Seal(7, "private configuration")
	require.NoError(t, err)
	key := make([]byte, 32)
	key[0] = 1
	newKey, err := wgtunnel.NewSecretBox(key)
	require.NoError(t, err)
	mock.ExpectQuery("SELECT proxy_id,config_ciphertext FROM wireguard_nodes").WillReturnRows(sqlmock.NewRows([]string{"proxy_id", "config_ciphertext"}).AddRow(7, cipher))
	s := &ImageUpdateService{db: db, wg: &WireGuardService{Box: newKey}}
	err = s.verifyWireGuard(context.Background(), nil)
	require.Error(t, err)
	require.NotContains(t, err.Error(), cipher)
	require.NoError(t, mock.ExpectationsWereMet())
}
