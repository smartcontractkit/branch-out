package server_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/internal/testhelpers"
	"github.com/smartcontractkit/branch-out/server"
)

func TestServer_Integration(t *testing.T) {
	t.Parallel()

	logger := testhelpers.Logger(t)
	server := server.New(server.WithLogger(logger))
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
