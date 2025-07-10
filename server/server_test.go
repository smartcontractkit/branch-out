package server

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/config"
	"github.com/smartcontractkit/branch-out/internal/testhelpers"
	"github.com/smartcontractkit/branch-out/internal/testhelpers/mock"
)

var testConfig = config.Config{
	Port: 0, // Set to 0 to allow the server to start on a random port
	GitHub: config.GitHub{
		Token: "test-token",
	},
	Jira: config.Jira{
		BaseDomain: "test-domain",
		ProjectKey: "test-project",
		Username:   "test-username",
		Token:      "test-token",
	},
	Trunk: config.Trunk{
		Token:         "test-token",
		WebhookSecret: "test-secret",
	},
}

func TestServer_New(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() {
		s, err := New(WithLogger(testhelpers.Logger(t)), WithConfig(testConfig))
		require.NotNil(t, s)
		require.NoError(t, err)
	})
}

func TestServer_Start(t *testing.T) {
	t.Parallel()

	logger := testhelpers.Logger(t)

	jiraClient := mock.NewJiraIClient(t)
	trunkClient := mock.NewTrunkIClient(t)
	githubClient := mock.NewGithubIClient(t)

	// Create server with mocked clients
	server, err := New(
		WithLogger(logger),
		WithConfig(testConfig),
		WithJiraClient(jiraClient),
		WithGitHubClient(githubClient),
		WithTrunkClient(trunkClient),
	)
	require.NoError(t, err)
	require.NotNil(t, server)

	ctx := t.Context()
	t.Cleanup(func() {
		require.NoError(t, server.Error(), "server experienced error during startup")
	})

	go func() {
		_ = server.Start(ctx) // already checking this in the t.Cleanup
	}()

	healthyCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	t.Cleanup(cancel)
	require.NoError(t, server.WaitHealthy(healthyCtx), "server did not become healthy")

	require.Positive(t, server.Port, "server port should be greater than 0")
}
