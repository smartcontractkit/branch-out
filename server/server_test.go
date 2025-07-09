package server

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/config"
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

func TestServer_Handlers(t *testing.T) {
	t.Parallel()

	config := &config.Config{
		Port: 0,
	}
	logger := testhelpers.Logger(t)
	server := New(WithLogger(logger), WithConfig(config))
	require.NotNil(t, server)

	ctx, killServer := context.WithCancel(context.Background())
	errChan := make(chan error, 1)
	go func() {
		err := server.Start(ctx)
		errChan <- err
	}()

	t.Cleanup(func() {
		killServer()
		require.NoError(t, <-errChan, "error while running server")
	})

	err := server.WaitHealthy(context.Background())
	require.NoError(t, err, "server did not become healthy")

	baseURL := fmt.Sprintf("http://%s", server.Addr)
	t.Log("baseURL", baseURL)

	client := resty.New().SetBaseURL(baseURL)
	require.NotNil(t, client)

	tests := []struct {
		endpoint             string
		method               string
		expectedCode         int
		expectedBodyContains string
	}{
		{endpoint: "/", method: http.MethodGet, expectedCode: http.StatusOK, expectedBodyContains: "branch-out"},
		{endpoint: "/health", method: http.MethodGet, expectedCode: http.StatusOK, expectedBodyContains: "healthy"},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s %s", test.method, test.endpoint), func(t *testing.T) {
			t.Parallel()

			resp, err := client.R().
				SetResult(&map[string]any{}).
				Execute(test.method, test.endpoint)
			require.NoError(t, err, "error calling server %s %s", test.method, resp.Request.URL)
			require.Equal(t, test.expectedCode, resp.StatusCode())
			require.Contains(t, resp.String(), test.expectedBodyContains)
		})
	}
}
