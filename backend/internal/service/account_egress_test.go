//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/egress"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
)

type previewRouteStore struct{ refs []egress.Reference }

func (s *previewRouteStore) ResolveReference(_ context.Context, r egress.Reference) (egress.Record, error) {
	s.refs = append(s.refs, r)
	if r.ExplicitDirect {
		return egress.Record{Kind: egress.Direct}, nil
	}
	return egress.Record{Kind: egress.WireGuard, NodeID: r.NodeID}, nil
}
func (*previewRouteStore) LoadNode(_ context.Context, id int64) (wgtunnel.Node, error) {
	return wgtunnel.Node{ID: id, Version: 1, Active: true, Paused: true}, nil
}
func (*previewRouteStore) SaveCheckpoint(context.Context, wgtunnel.Checkpoint, bool) error {
	return nil
}

type previewRoutedRepo struct {
	AccountRepository
	router *egress.Router
}

func (r *previewRoutedRepo) EgressRouter() *egress.Router { return r.router }

func TestUnsavedAccountModelPreviewHonorsSelectedNode(t *testing.T) {
	store := &previewRouteStore{}
	manager := wgtunnel.NewManager(store, nil)
	defer manager.Close()
	router, err := egress.NewRouter(store, manager)
	if err != nil {
		t.Fatal(err)
	}
	upstream := &httpUpstreamRecorder{responses: []*http.Response{{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"test-model"}]}`))}}}
	svc := &AccountTestService{cfg: &config.Config{}, accountRepo: &previewRoutedRepo{router: router}, httpUpstream: upstream}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "synthetic", "base_url": "https://example.invalid"}}
	if _, err = svc.fetchUpstreamModelListWithObservation(context.Background(), account); err != nil {
		t.Fatal("explicit direct preview failed", err)
	}
	if route := egress.FromContext(upstream.lastReq.Context()); route == nil || route.Kind() != egress.Direct {
		t.Fatal("preview did not carry an explicit route")
	}
	nodeID := int64(7)
	account.ProxyID = &nodeID
	upstream.lastReq = nil
	if _, err = svc.fetchUpstreamModelListWithObservation(context.Background(), account); !errors.Is(err, egress.ErrBlocked) {
		t.Fatal("paused preview node accepted", err)
	}
	if upstream.lastReq != nil {
		t.Fatal("paused preview escaped to upstream")
	}
	if store.refs[len(store.refs)-1].NodeID != nodeID {
		t.Fatal("selected preview node was ignored")
	}
}
