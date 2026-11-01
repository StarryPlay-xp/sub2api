package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
)

var ErrImageUpdateRequired = infraerrors.Conflict("IMAGE_UPDATE_REQUIRED", "Docker deployments require the host image updater; binary updates and rollbacks are disabled")

func SystemUpdateMode() string {
	if os.Getenv("SUB2API_DEPLOYMENT") == "docker" || os.Getenv("IMAGE_UPDATER_SOCKET") != "" {
		return "image"
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return "image"
	}
	return "binary"
}

// Only a fixed Unix socket is reachable. The application never receives a
// Docker socket, shell command, compose path, registry credential or backup.
type ImageUpdateService struct {
	client  *http.Client
	db      *sql.DB
	wg      *WireGuardService
	version string
	cancel  context.CancelFunc
	done    chan struct{}
}

type ImageUpdateStatus struct {
	Enabled bool    `json:"enabled"`
	ID      string  `json:"id,omitempty"`
	Phase   string  `json:"phase"`
	Version string  `json:"version,omitempty"`
	Error   string  `json:"error,omitempty"`
	Nodes   []int64 `json:"nodes,omitempty"`
}

func NewImageUpdateService(db *sql.DB, wg *WireGuardService, version string) *ImageUpdateService {
	s := &ImageUpdateService{db: db, wg: wg, version: strings.TrimPrefix(version, "v")}
	path := os.Getenv("IMAGE_UPDATER_SOCKET")
	if path == "" {
		return s
	}
	s.client = &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", path)
		},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel, s.done = cancel, make(chan struct{})
	wg.imageUpdater = s
	go s.verifyLoop(ctx)
	return s
}

func (s *ImageUpdateService) Close() {
	if s.cancel != nil {
		s.cancel()
		<-s.done
		s.client.CloseIdleConnections()
	}
}

func (s *ImageUpdateService) call(ctx context.Context, method, path string, body any) (*ImageUpdateStatus, error) {
	if s == nil || s.client == nil {
		return nil, errors.New("host image updater is not configured")
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://updater"+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, errors.New("host image updater is unavailable")
	}
	defer func() { _ = resp.Body.Close() }()
	var status ImageUpdateStatus
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&status) != nil {
		return nil, errors.New("invalid updater response")
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return nil, errors.New("host rejected update request; inspect updater status")
	}
	return &status, nil
}

func (s *ImageUpdateService) Status(ctx context.Context) (*ImageUpdateStatus, error) {
	if s == nil || s.client == nil {
		return &ImageUpdateStatus{Phase: "unconfigured"}, nil
	}
	return s.call(ctx, http.MethodGet, "/status", nil)
}

func (s *ImageUpdateService) Start(ctx context.Context, version string) (*ImageUpdateStatus, error) {
	return s.call(ctx, http.MethodPost, "/update", map[string]string{"version": version})
}

func (s *ImageUpdateService) verifyLoop(ctx context.Context) {
	defer close(s.done)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			status, err := s.Status(ctx)
			if err != nil || status.Phase != "verifying" || status.Version != s.version {
				continue
			}
			work, cancel := context.WithTimeout(ctx, 10*time.Minute)
			err = s.verifyWireGuard(work, status.Nodes)
			cancel()
			if ctx.Err() != nil {
				return
			}
			message := ""
			if err != nil {
				message = err.Error()
			}
			_, _ = s.call(ctx, http.MethodPost, "/verified", map[string]any{
				"id": status.ID, "version": s.version, "ok": err == nil, "error": message,
			})
		}
	}
}

// Decrypt and parse every retained configuration, including paused nodes.
// Configuration verification never starts tunnels or sends network probes;
// egress connectivity and business acceptance are explicitly manual.
func (s *ImageUpdateService) verifyWireGuard(ctx context.Context, expected []int64) error {
	rows, err := s.db.QueryContext(ctx, `SELECT proxy_id,config_ciphertext FROM wireguard_nodes`)
	if err != nil {
		return errors.New("WG configuration verification failed")
	}
	configs := map[int64]wgtunnel.Config{}
	for rows.Next() {
		var id int64
		var cipher string
		if err = rows.Scan(&id, &cipher); err != nil {
			break
		}
		var plain string
		plain, err = s.wg.Box.Open(id, cipher)
		if err != nil {
			break
		}
		configs[id], err = wgtunnel.Parse(plain)
		plain = ""
		if err != nil {
			break
		}
	}
	rowErr := rows.Err()
	_ = rows.Close()
	if err != nil || rowErr != nil {
		return errors.New("WG configuration cannot be read with the retained key")
	}
	for _, id := range expected {
		if _, ok := configs[id]; !ok {
			return fmt.Errorf("WG node %d is missing", id)
		}
	}
	return nil
}
