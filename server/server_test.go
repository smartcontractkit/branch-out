package server

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/internal/testhelpers"
)

func TestNew(t *testing.T) {
	t.Parallel()

	logger := testhelpers.Logger(t)
	server := New(WithLogger(logger))
	require.NotNil(t, server)
}

func TestStart(t *testing.T) {
	t.Parallel()

	logger := testhelpers.Logger(t)
	server := New(WithLogger(logger))
	require.NotNil(t, server)

	ctx, killServer := context.WithCancel(context.Background())
	errChan := make(chan error, 1)

	go func() {
		err := server.Start(ctx)
		errChan <- err
	}()

	healthyCtx, healthyCtxCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	t.Cleanup(healthyCtxCancel)

	err := server.WaitHealthy(healthyCtx)
	require.NoError(t, err, "server did not become healthy")

	killServer()

	err = <-errChan
	require.NoError(t, err, "server start returned error")
}

//nolint:revive // context-as-argument is not a good idea here
func startServer(tb testing.TB, ctx context.Context) *Server {
	tb.Helper()

	logger := testhelpers.Logger(tb)
	server := New(WithLogger(logger))
	require.NotNil(tb, server)
	if ctx == nil {
		ctx = context.Background()
	}

	go func() {
		if err := server.Start(ctx); err != nil {
			tb.Logf("Error starting server: %s", err)
			tb.Fail()
		}
	}()

	waitCtx, waitCtxCancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer waitCtxCancel()
	err := server.WaitHealthy(waitCtx)
	require.NoError(tb, err, "server did not become healthy")

	return server
}
