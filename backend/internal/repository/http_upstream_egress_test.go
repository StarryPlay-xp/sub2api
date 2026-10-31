//go:build unit

package repository

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/egress"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"time"
)

type upstreamRouteStore struct {
	mu   sync.Mutex
	node wgtunnel.Node
}

func (s *upstreamRouteStore) LoadNode(context.Context, int64) (wgtunnel.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.node, nil
}
func (s *upstreamRouteStore) SaveCheckpoint(context.Context, wgtunnel.Checkpoint, bool) error {
	return nil
}
func (s *upstreamRouteStore) ResolveReference(_ context.Context, ref egress.Reference) (egress.Record, error) {
	return egress.Record{Kind: egress.WireGuard, NodeID: 1, AccountID: ref.AccountID, Revision: 1}, nil
}

// Opt-in end-to-end coverage of the actual HTTPUpstream implementation, including
// the case where the caller has lost its hydrated proxy and passes an empty URL.
func TestHTTPUpstreamAuthorizedWireGuardExit(t *testing.T) {
	path := os.Getenv("SUB2API_WG_TEST_CONFIG")
	if path == "" {
		t.Skip("authorized remote config required")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm()&0077 != 0 {
		t.Fatal("test config must be owner-only")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("cannot read test configuration")
	}
	expected, err := os.ReadFile(os.Getenv("SUB2API_WG_TEST_EXPECTED_EXIT"))
	if err != nil {
		t.Fatal("expected exit is required")
	}
	box, err := wgtunnel.NewSecretBox([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := box.Seal(1, string(raw))
	if err != nil {
		t.Fatal(err)
	}
	store := &upstreamRouteStore{node: wgtunnel.Node{ID: 1, Version: 1, Active: true, Ciphertext: encrypted}}
	manager := wgtunnel.NewManager(store, box)
	defer manager.Close()
	router, err := egress.NewRouter(store, manager)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	upstream := NewRoutedHTTPUpstream(cfg, router)
	for _, fingerprint := range []bool{false, true} {
		ctx := service.WithHTTPUpstreamProfile(context.Background(), service.HTTPUpstreamProfileLongStream)
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://icanhazip.com", nil)
		if err != nil {
			t.Fatal(err)
		}
		var response *http.Response
		if fingerprint {
			response, err = upstream.DoWithTLS(request, "", 7, 2, &tlsfingerprint.Profile{Name: "WG-test"})
		} else {
			response, err = upstream.Do(request, "", 7, 2)
		}
		if err != nil {
			t.Fatal("actual upstream transport did not reach authorized exit", err)
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, 128))
		response.Body.Close()
		if err != nil || strings.TrimSpace(string(data)) != strings.TrimSpace(string(expected)) {
			t.Fatal("actual upstream used a different exit")
		}
	}
	route, err := router.Resolve(context.Background(), egress.Reference{AccountID: 7})
	if err != nil {
		t.Fatal(err)
	}
	shared, err := httpclient.GetClient(httpclient.Options{Route: route, Timeout: 10 * time.Second, ValidateResolvedIP: true})
	if err != nil {
		t.Fatal(err)
	}
	response, err := shared.Get("https://icanhazip.com")
	if err != nil {
		t.Fatal("shared HTTP client did not use WG", err)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 128))
	response.Body.Close()
	if err != nil || strings.TrimSpace(string(data)) != strings.TrimSpace(string(expected)) {
		t.Fatal("shared HTTP exit mismatch")
	}
	browser, err := getSharedReqClient(reqClientOptions{Route: route, Timeout: 10 * time.Second, Impersonate: true, ForceHTTP2: true})
	if err != nil {
		t.Fatal(err)
	}
	browserResponse, err := browser.R().Get("https://icanhazip.com")
	if err != nil || strings.TrimSpace(browserResponse.String()) != strings.TrimSpace(string(expected)) {
		t.Fatal("shared req exit mismatch")
	}
	store.mu.Lock()
	store.node.Paused = true
	store.mu.Unlock()
	if _, err = shared.Get("https://icanhazip.com"); !errors.Is(err, egress.ErrBlocked) {
		t.Fatal("shared HTTP pooled connection survived pause")
	}
	if _, err = browser.R().Get("https://icanhazip.com"); !errors.Is(err, egress.ErrBlocked) {
		t.Fatal("shared req pooled connection survived pause")
	}
	request, _ := http.NewRequest(http.MethodGet, "https://icanhazip.com", nil)
	if _, err = upstream.Do(request, "", 7, 2); !errors.Is(err, egress.ErrBlocked) {
		t.Fatal("paused route was not blocked")
	}
}
