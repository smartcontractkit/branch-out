package trunk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	svix "github.com/svix/svix-webhooks/go"

	"github.com/smartcontractkit/branch-out/internal/testhelpers"
	"github.com/smartcontractkit/branch-out/internal/testhelpers/mock"
	"github.com/smartcontractkit/branch-out/jira"
)

var (
	repoName        = "test/repo"
	testPackageName = "github.com/test/repo/package"
	testName        = "TestFlaky"
	filePath        = "test/file_test.go"
	trunkID         = "test_trunk_id"
	details         = "test_details"
	codeowners      = []string{"@test"}
	repoURL         = "https://github.com/test/repo"

	flakyTestCase = TestCase{
		ID:         trunkID,
		Codeowners: codeowners,
		FilePath:   filePath,
		HTMLURL:    repoURL,
		Name:       testName,
		Quarantine: true,
	}

	quarantinedPayload = TestCaseStatusChange{
		StatusChange: StatusChange{
			CurrentStatus: CurrentStatus{
				Value: "flaky",
			},
			PreviousStatus: "healthy",
		},
		TestCase: flakyTestCase,
	}

	healthyTestCase = TestCase{
		ID:         trunkID,
		Codeowners: codeowners,
		FilePath:   filePath,
		HTMLURL:    repoURL,
		Name:       testName,
		Quarantine: false,
	}

	unQuarantinedPayload = TestCaseStatusChange{
		StatusChange: StatusChange{
			CurrentStatus: CurrentStatus{
				Value: "healthy",
			},
			PreviousStatus: "flaky",
		},
		TestCase: healthyTestCase,
	}
	// webhookSecret is the secret used to sign Trunk webhooks. This is an example secret from the Trunk docs.
	// We use it to sign our own payloads and make them valid for testing.
	webhookSecret = "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"
)

func TestReceiveWebhook_FlakyTest(t *testing.T) {
	t.Parallel()

	l := testhelpers.Logger(t)

	jiraClient := mock.NewJiraIClient(t)
	trunkClient := mock.NewTrunkIClient(t)

	expectedJiraTicketRequest := jira.FlakyTestTicketRequest{
		RepoName:        "test/repo",
		TestPackageName: "test/package",
		FilePath:        "test/file.go",
		TrunkID:         "test_trunk_id",
		Details:         "test_details",
	}

	quarantinedPayloadJSON, err := json.Marshal(quarantinedPayload)
	require.NoError(t, err, "failed to marshal payload")

	// Generate valid svix signature
	webhookRequest := createValidWebhookRequest(t, "/webhooks/trunk", quarantinedPayloadJSON)

	jiraClient.EXPECT().CreateFlakyTestTicket(expectedJiraTicketRequest).Return(&jira.TicketResponse{
		Key: "BRANCH-1",
	}, nil).Times(1)

	jiraClient.EXPECT().GetTicketStatus("BRANCH-1").Return(&jira.TicketStatus{
		Key: "BRANCH-1",
	}, nil).Times(1)

	err = ReceiveWebhook(l, webhookRequest, webhookSecret, jiraClient, trunkClient)
	require.NoError(t, err, "failed to receive webhook")
}

// createValidWebhookRequest creates an HTTP request with a valid svix signature for unit testing
func createValidWebhookRequest(t *testing.T, path string, payload []byte) *http.Request {
	t.Helper()

	// Create svix webhook for signing
	wh, err := svix.NewWebhook(webhookSecret)
	require.NoError(t, err, "failed to create svix webhook for signing")

	// Generate headers (svix will add the signature)
	headers := http.Header{}
	headers.Set("webhook-id", "msg_p5jXN8AQM9LWM0D4loKWxJek")
	headers.Set("webhook-timestamp", fmt.Sprintf("%d", time.Now().Unix()))

	// Sign the payload
	signature, err := wh.Sign("msg_p5jXN8AQM9LWM0D4loKWxJek", time.Now(), payload)
	require.NoError(t, err, "failed to sign webhook payload")

	// Set the signature header
	headers.Set("webhook-signature", signature)

	return &http.Request{
		Method: "POST",
		URL:    &url.URL{Path: path},
		Body:   io.NopCloser(bytes.NewBuffer(payload)),
		Header: headers,
	}
}

func TestExtractRepoNameFromURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		url      string
		expected string
	}{
		{"https://github.com/trunk-io/analytics-cli", "analytics-cli"},
		{"https://github.com/owner/repo", "repo"},
		{"invalid-url", "invalid-url"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			t.Parallel()
			result := extractRepoNameFromURL(tt.url)
			require.Equal(t, tt.expected, result, "expected '%s', got '%s'", tt.expected, result)
		})
	}
}

func TestExtractDomainFromJiraURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		url      string
		expected string
	}{
		{"https://company.atlassian.net/rest/api/2/issue/123", "company.atlassian.net"},
		{"https://trunk-io.atlassian.net/rest/api/2/issue/456", "trunk-io.atlassian.net"},
		{"invalid-url", "unknown-domain.atlassian.net"},
		{"", "unknown-domain.atlassian.net"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			t.Parallel()
			result := extractDomainFromJiraURL(tt.url)
			require.Equal(t, tt.expected, result, "expected '%s', got '%s'", tt.expected, result)
		})
	}
}

func TestExtractRepoInfoFromURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		url           string
		expectedOwner string
		expectedRepo  string
	}{
		{"https://github.com/trunk-io/analytics-cli", "trunk-io", "analytics-cli"},
		{"https://github.com/owner/repo", "owner", "repo"},
		{"https://github.com/smartcontractkit/branch-out", "smartcontractkit", "branch-out"},
		{"invalid-url", "unknown", "unknown"},
		{"https://gitlab.com/owner/repo", "unknown", "unknown"}, // Non-GitHub URL
		{"", "unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			t.Parallel()
			owner, repo := extractRepoInfoFromURL(tt.url)
			require.Equal(
				t,
				tt.expectedOwner,
				owner,
				"owner mismatch: expected '%s', got '%s'",
				tt.expectedOwner,
				owner,
			)
			require.Equal(
				t,
				tt.expectedRepo,
				repo,
				"repo name mismatch: expected '%s', got '%s'",
				tt.expectedRepo,
				repo,
			)
		})
	}
}
