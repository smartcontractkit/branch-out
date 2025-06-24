package server

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/internal/testhelpers"
	"github.com/smartcontractkit/branch-out/trunk"
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

	errChan := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		err := server.Start(ctx)
		errChan <- err
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	err := <-errChan
	require.NoError(t, err)
}

func TestReceiveWebhook_DetermineType(t *testing.T) {
	t.Parallel()

	server := startServer(t, nil)
	testCases := []struct {
		name            string
		payloadFile     string
		expectedMessage string
	}{
		{
			name:        "Quarantining setting changed",
			payloadFile: "testdata/quarantining_setting_changed_webhook.json",
			expectedMessage: fmt.Sprintf(
				webhookResponseMessageEventProcessed,
				trunk.WebhookTypeQuarantiningSettingChanged,
			),
		},
		{
			name:            "Status changed",
			payloadFile:     "testdata/status_changed_webhook.json",
			expectedMessage: fmt.Sprintf(webhookResponseMessageEventProcessed, trunk.WebhookTypeStatusChanged),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			payload, err := os.ReadFile(tc.payloadFile)
			require.NoError(t, err)

			response, err := server.ReceiveWebhook(&WebhookRequest{
				Payload: payload,
			})
			require.NoError(t, err)
			require.True(t, response.Success)
			require.Equal(t, tc.expectedMessage, response.Message, "response message mismatch")
		})
	}
}

func TestReceiveWebhook_StatusChanged(t *testing.T) {
	t.Parallel()

	server := startServer(t, nil)

	payload, err := os.ReadFile("testdata/status_changed_webhook.json")
	require.NoError(t, err)

	response, err := server.ReceiveWebhook(&WebhookRequest{
		Payload: payload,
	})
	require.NoError(t, err)
	require.True(t, response.Success)
	require.Equal(t, "Status change processed", response.Message)
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
