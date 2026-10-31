package service

import (
	"context"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/egress"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
	"time"
)

type WireGuardRepository interface {
	CreateNode(context.Context, wgtunnel.NodeInput, *wgtunnel.SecretBox) (int64, error)
	ReplaceNode(context.Context, int64, int64, wgtunnel.NodeInput, *wgtunnel.SecretBox) error
	ReadNode(context.Context, int64) (wgtunnel.NodeView, error)
	SetNodePaused(context.Context, int64, int64, bool) error
	TrafficHistory(context.Context, int64, string) ([]wgtunnel.Bucket, error)
	NodeObservations(context.Context, int64) ([]wgtunnel.Evidence, error)
	ReadAttempts(context.Context, string) ([]egress.Attempt, error)
	ReadExitSummaries(context.Context, []egress.AttemptQuery) ([]egress.ExitSummary, error)
	PruneTrafficHistory(context.Context, time.Time) error
}
type WireGuardService struct {
	Store    WireGuardRepository
	Box      *wgtunnel.SecretBox
	Manager  *wgtunnel.Manager
	Router   *egress.Router
	Observer *egress.Observer
	Recorder *egress.Recorder
	cancel   context.CancelFunc
	done     chan struct{}
}

func (s *WireGuardService) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				work, cancel := context.WithTimeout(ctx, 30*time.Second)
				_ = s.Store.PruneTrafficHistory(work, time.Now())
				cancel()
			}
		}
	}()
}
func (s *WireGuardService) Close() {
	if s == nil {
		return
	}
	if s.cancel != nil {
		s.cancel()
		<-s.done
	}
	s.Observer.Close()
	s.Manager.Close()
	s.Recorder.Close()
}
func (s *WireGuardService) Create(ctx context.Context, input wgtunnel.NodeInput) (wgtunnel.NodeView, error) {
	if s.Box == nil {
		return wgtunnel.NodeView{}, errors.New("Configure WIREGUARD_KEY_FILE before creating WireGuard nodes")
	}
	id, err := s.Store.CreateNode(ctx, input, s.Box)
	if err != nil {
		return wgtunnel.NodeView{}, err
	}
	return s.Store.ReadNode(ctx, id)
}
func (s *WireGuardService) Replace(ctx context.Context, id, version int64, input wgtunnel.NodeInput) (wgtunnel.NodeView, error) {
	if s.Box == nil {
		return wgtunnel.NodeView{}, errors.New("WireGuard encryption key is unavailable")
	}
	if err := s.Store.ReplaceNode(ctx, id, version, input, s.Box); err != nil {
		return wgtunnel.NodeView{}, err
	}
	s.Manager.Revoke(id)
	return s.Store.ReadNode(ctx, id)
}
func (s *WireGuardService) Pause(ctx context.Context, id, version int64, paused bool) error {
	if err := s.Store.SetNodePaused(ctx, id, version, paused); err != nil {
		return err
	}
	s.Manager.Revoke(id)
	return nil
}

type WireGuardDetails struct {
	Node            wgtunnel.NodeView   `json:"node"`
	Rate            wgtunnel.Rate       `json:"rate"`
	History         []wgtunnel.Bucket   `json:"history"`
	Observations    []wgtunnel.Evidence `json:"observations"`
	DroppedAttempts uint64              `json:"dropped_attempt_records"`
}

func (s *WireGuardService) Details(ctx context.Context, id int64, interval string) (WireGuardDetails, error) {
	var result WireGuardDetails
	var err error
	result.Node, err = s.Store.ReadNode(ctx, id)
	if err != nil {
		return result, err
	}
	result.Rate = s.Manager.Rate(id)
	result.History, err = s.Store.TrafficHistory(ctx, id, interval)
	if err != nil {
		return result, err
	}
	result.Observations, err = s.Store.NodeObservations(ctx, id)
	runtimeID, version := s.Manager.Identity(id)
	for i := range result.Observations {
		e := &result.Observations[i]
		if e.RuntimeID != runtimeID || e.Version != version || !e.ExpiresAt.After(time.Now()) {
			e.State = "expired"
		}
	}
	result.DroppedAttempts = s.Recorder.Dropped()
	return result, err
}
func (s *WireGuardService) Probe(ctx context.Context, id int64, family string) (wgtunnel.Evidence, error) {
	route, err := s.Router.Resolve(ctx, egress.Reference{NodeID: id})
	if err != nil {
		return wgtunnel.Evidence{}, err
	}
	return s.Observer.ProbeNow(ctx, route, family)
}
