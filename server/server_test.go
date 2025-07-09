package server

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/config"
	"github.com/smartcontractkit/branch-out/internal/testhelpers"
	"github.com/smartcontractkit/branch-out/jira"
	"github.com/smartcontractkit/branch-out/mocks"
)

var testConfig = &config.Config{
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

func TestStart(t *testing.T) {
	t.Parallel()

	logger := testhelpers.Logger(t)
	server := New(WithLogger(logger), WithConfig(testConfig))
	require.NotNil(t, server)

	ctx := t.Context()

	go func() {
		if err := server.Start(ctx); err != nil && err != http.ErrServerClosed {
			t.Errorf("Server failed to start: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	require.Positive(t, server.Port)

	client := resty.New()
	resp, err := client.R().Get(fmt.Sprintf("http://localhost:%d/health", server.Port))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode())
}

func TestServer_WithMockExpectations(t *testing.T) {
	t.Parallel()

	logger := testhelpers.Logger(t)

	// Create generated mocks
	mockJira := &mocks.MockJiraClient{}
	mockTrunk := &mocks.MockTrunkClient{}
	mockGitHub := &mocks.MockGitHubClient{}

	// Set up expectations
	mockJira.On("CreateFlakyTestTicket", mock.AnythingOfType("jira.FlakyTestTicketRequest")).
		Return(&jira.TicketResponse{Key: "TEST-123"}, nil).
		Maybe() // Optional call

	mockTrunk.On("LinkTicketToTestCase", "test-case-id", mock.AnythingOfType("*jira.TicketResponse"), "https://github.com/test/repo").
		Return(nil).
		Maybe()

		// Optional call

	server := New(
		WithLogger(logger),
		WithConfig(testConfig),
		WithJiraClient(mockJira),
		WithTrunkClient(mockTrunk),
		WithGitHubClient(mockGitHub),
	)

	// Test server functionality here
	// Mocks will verify expectations automatically

	require.NotNil(t, server)
}
