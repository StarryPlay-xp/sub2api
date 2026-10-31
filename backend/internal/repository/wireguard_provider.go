package repository

import (
	"database/sql"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/egress"
	"github.com/Wei-Shaw/sub2api/internal/pkg/wgtunnel"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func ProvideWireGuardService(db *sql.DB, cfg *config.Config) (*service.WireGuardService, error) {
	var box *wgtunnel.SecretBox
	var err error
	if cfg.WireGuard.KeyFile != "" {
		box, err = wgtunnel.LoadSecretBox(cfg.WireGuard.KeyFile)
		if err != nil {
			return nil, err
		}
	}
	store := &WireGuardStore{DB: db}
	manager := wgtunnel.NewManager(store, box)
	router, err := egress.NewRouter(store, manager)
	if err != nil {
		manager.Close()
		return nil, err
	}
	observer := egress.NewObserver(router, store)
	recorder := egress.NewRecorder(store)
	router.SetTelemetry(observer, recorder)
	s := &service.WireGuardService{Store: store, Box: box, Manager: manager, Router: router, Observer: observer, Recorder: recorder}
	s.Start()
	return s, nil
}
func ProvideEgressRouter(s *service.WireGuardService) *egress.Router { return s.Router }
