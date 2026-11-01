//go:build integration

package repository

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestBandwidthLimitsPersistAcrossInstancesAndClear(t *testing.T) {
	db := wireguardTestDB(t)
	_, err := db.Exec(`CREATE TABLE settings(key text PRIMARY KEY,value text NOT NULL,updated_at timestamptz NOT NULL)`)
	require.NoError(t, err)
	a, b := &BandwidthLimitStore{DB: db}, &BandwidthLimitStore{DB: db}
	ctx := context.Background()
	initial, err := a.ReadBandwidthLimits(ctx)
	require.NoError(t, err)
	require.Nil(t, initial.Host.Mbps)
	require.Empty(t, initial.Host.Version)
	require.Empty(t, initial.Nodes)
	value := 100.5
	host, err := a.UpdateBandwidthLimit(ctx, 0, "", &value)
	require.NoError(t, err)
	node, err := b.UpdateBandwidthLimit(ctx, 1, "", &value)
	require.NoError(t, err)
	read, err := b.ReadBandwidthLimits(ctx)
	require.NoError(t, err)
	require.Equal(t, host, read.Host)
	require.Equal(t, node, read.Nodes["1"])
	_, err = b.UpdateBandwidthLimit(ctx, 0, "", &value)
	require.ErrorIs(t, err, service.ErrBandwidthLimitConflict)
	cleared, err := b.UpdateBandwidthLimit(ctx, 0, host.Version, nil)
	require.NoError(t, err)
	require.Nil(t, cleared.Mbps)
	require.NotEqual(t, host.Version, cleared.Version)
	_, err = a.UpdateBandwidthLimit(ctx, 0, host.Version, &value)
	require.ErrorIs(t, err, service.ErrBandwidthLimitConflict)
	_, err = a.UpdateBandwidthLimit(ctx, 0, "", &value)
	require.ErrorIs(t, err, service.ErrBandwidthLimitConflict)
	read, err = a.ReadBandwidthLimits(ctx)
	require.NoError(t, err)
	require.Equal(t, cleared, read.Host)
	require.Equal(t, node, read.Nodes["1"])
	_, err = db.Exec(`UPDATE proxies SET deleted_at=NOW() WHERE id=1`)
	require.NoError(t, err)
	read, err = b.ReadBandwidthLimits(ctx)
	require.NoError(t, err)
	require.Empty(t, read.Nodes)
	_, err = a.UpdateBandwidthLimit(ctx, 1, node.Version, &value)
	require.ErrorIs(t, err, service.ErrBandwidthNodeNotFound)
}

func TestBandwidthLimitsConcurrentWritesDoNotOverwrite(t *testing.T) {
	db := wireguardTestDB(t)
	_, err := db.Exec(`CREATE TABLE settings(key text PRIMARY KEY,value text NOT NULL,updated_at timestamptz NOT NULL)`)
	require.NoError(t, err)
	s := &BandwidthLimitStore{DB: db}
	ctx := context.Background()
	version := ""
	for round := 0; round < 2; round++ {
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(value float64) {
				defer wg.Done()
				_, e := s.UpdateBandwidthLimit(ctx, 0, version, &value)
				results <- e
			}(float64(i + 100))
		}
		wg.Wait()
		close(results)
		success, conflict := 0, 0
		for e := range results {
			if e == nil {
				success++
			} else if errors.Is(e, service.ErrBandwidthLimitConflict) {
				conflict++
			} else {
				t.Fatal(e)
			}
		}
		require.Equal(t, 1, success)
		require.Equal(t, 1, conflict)
		read, e := s.ReadBandwidthLimits(ctx)
		require.NoError(t, e)
		version = read.Host.Version
	}
	// Ordinary proxies are not valid WG limit targets.
	_, err = db.Exec(`INSERT INTO proxies(id,name,protocol) VALUES(2,'http','http')`)
	require.NoError(t, err)
	value := 10.0
	_, err = s.UpdateBandwidthLimit(ctx, 2, "", &value)
	require.ErrorIs(t, err, service.ErrBandwidthNodeNotFound)
	_, err = s.UpdateBandwidthLimit(ctx, 999, "", &value)
	require.ErrorIs(t, err, service.ErrBandwidthNodeNotFound)
}
