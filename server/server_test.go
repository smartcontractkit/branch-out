package server

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/base"
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
		WebhookSecret: "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw", // example secret from svix docs
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

	server, _, _, _ := runServer(t, logger)

	require.Positive(t, server.Port, "server port should be greater than 0")
}

func TestServer_UnknownRoute(t *testing.T) {
	t.Parallel()

	l := testhelpers.Logger(t)
	server, _, _, _ := runServer(t, l)

	unknownRouteURL := url.URL{Scheme: "http", Host: server.Addr, Path: "/unknown"}
	req, err := http.NewRequest(http.MethodGet, unknownRouteURL.String(), nil)
	require.NoError(t, err, "failed to create request")

	c := base.NewClient("test", base.WithLogger(l))
	resp, err := c.Do(req)
	require.NoError(t, err, "failed to send request")
	require.Equal(t, http.StatusNotFound, resp.StatusCode, "expected a not found status for an unknown route")
}

// runServer runs a server with mocked clients
func runServer(t *testing.T, l zerolog.Logger) (*Server, *mock.JiraIClient, *mock.TrunkIClient, *mock.GithubIClient) {
	jiraClient := mock.NewJiraIClient(t)
	trunkClient := mock.NewTrunkIClient(t)
	githubClient := mock.NewGithubIClient(t)

	// Create server with mocked clients
	server, err := New(
		WithLogger(l),
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

	return server, jiraClient, trunkClient, githubClient
}
