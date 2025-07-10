package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/config"
	github_mock "github.com/smartcontractkit/branch-out/internal/testhelpers/mocks/github"
	jira_mock "github.com/smartcontractkit/branch-out/internal/testhelpers/mocks/jira"
	trunk_mock "github.com/smartcontractkit/branch-out/internal/testhelpers/mocks/trunk"
	"github.com/smartcontractkit/branch-out/jira"
	"github.com/smartcontractkit/branch-out/trunk"
)

func TestServer_Integration_TestStatusChanged(t *testing.T) {
	t.Parallel()

	testConfig := config.Config{
		Port: 0,
	}

	jiraClient := jira_mock.NewIClient(t)
	githubClient := github_mock.NewIClient(t)
	trunkClient := trunk_mock.NewIClient(t)

	server := New(
		WithConfig(testConfig),
		WithJiraClient(jiraClient),
		WithGitHubClient(githubClient),
		WithTrunkClient(trunkClient),
	)

	ctx := t.Context()
	t.Cleanup(func() {
		require.NoError(t, server.Error(), "server should not have a startup error")
	})
	go func() {
		_ = server.Start(ctx) //nolint:errcheck // we're checking this in the cleanup
	}()

	healthyCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	t.Cleanup(cancel)
	err := server.WaitHealthy(healthyCtx)
	require.NoError(t, err, "server should be healthy")

	expectedJiraTicketRequest := jira.FlakyTestTicketRequest{
		RepoName:        "test/repo",
		TestPackageName: "test/package",
		FilePath:        "test/file.go",
		TrunkID:         "test_trunk_id",
		Details:         "test_details",
	}

	jiraClient.EXPECT().CreateFlakyTestTicket(expectedJiraTicketRequest).Return(&jira.TicketResponse{
		Key: "BRANCH-1",
	}, nil).Times(1)

	payloadJSON, err := os.ReadFile("testdata/test_quarantined.json")
	require.NoError(t, err, "failed to read test data")

	var trunkWebhookPayload trunk.TestCaseStatusChangedPayload
	err = json.Unmarshal(payloadJSON, &trunkWebhookPayload)
	require.NoError(t, err, "cannot parse the Trunk payload")

	webhookRequest := &http.Request{
		Method: "POST",
		URL:    &url.URL{Path: "/webhooks/trunk"},
		Body:   io.NopCloser(bytes.NewBuffer(payloadJSON)),
	}

	webhookResponse, err := server.ReceiveWebhook(webhookRequest)
	require.NoError(t, err, "failed to receive webhook")
	require.True(t, webhookResponse.Success, "webhook should be successful")
}
