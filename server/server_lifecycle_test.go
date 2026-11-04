package server_test

import (
	"context"
	"testing"
	"time"

	simulate "github.com/disaster37/gobot-simulate"
	"github.com/disaster37/gobot-simulate/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerDirectLifecycle(t *testing.T) {
	sim := simulate.New()
	srv, err := server.New(sim, server.Config{Addr: "127.0.0.1", Port: 0, MaxClients: 0})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, srv.Start(ctx))
	require.NotZero(t, srv.Port())
	require.NotEmpty(t, srv.URL())
	require.NotNil(t, srv.Hertz())

	// Starting again is a no-op.
	require.NoError(t, srv.Start(ctx))

	require.NoError(t, srv.Stop(context.Background()))
	// Stopping again is a no-op.
	require.NoError(t, srv.Stop(context.Background()))
}

func TestServerServeBlocksUntilCancel(t *testing.T) {
	sim := simulate.New()
	srv, err := server.New(sim, server.Config{Addr: "127.0.0.1", Port: 0})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx) }()
	require.Eventually(t, func() bool { return srv.Port() != 0 }, time.Second, 10*time.Millisecond)

	cancel()
	require.NoError(t, <-done)
}

func TestServerNewRequiresBackend(t *testing.T) {
	srv, err := server.New(nil, server.Config{})
	assert.Nil(t, srv)
	assert.Error(t, err)
}

func TestServerURLBeforeStart(t *testing.T) {
	sim := simulate.New()
	srv, err := server.New(sim, server.Config{Addr: "0.0.0.0", Port: 0})
	require.NoError(t, err)
	assert.Empty(t, srv.URL())
	assert.Zero(t, srv.Port())
}

func TestServerURLWildcardHost(t *testing.T) {
	sim := simulate.New()
	srv, err := server.New(sim, server.Config{Addr: "0.0.0.0", Port: 0})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, srv.Start(ctx))

	// The wildcard bind address is normalized to 127.0.0.1 in the reported URL.
	assert.Contains(t, srv.URL(), "127.0.0.1")
}
