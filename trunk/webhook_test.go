package trunk

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/internal/testhelpers"
)

var (
	flakyTestCase = TestCase{
		ID:         "test_trunk_id",
		Codeowners: []string{"@test"},
		FilePath:   "test/file_test.go",
		HTMLURL:    "https://github.com/test/repo",
		Name:       "TestFlaky",
		Quarantine: true,
	}

	quarantinedPayload = TestCaseStatusChange{
		StatusChange: StatusChange{
			CurrentStatus: Status{
				Value: "flaky",
			},
			PreviousStatus: "healthy",
		},
		TestCase: flakyTestCase,
	}

	// Unused for now, but keeping for reference.
	// healthyTestCase = TestCase{
	// 	ID:         trunkID,
	// 	Codeowners: codeowners,
	// 	FilePath:   filePath,
	// 	HTMLURL:    repoURL,
	// 	Name:       testName,
	// 	Quarantine: false,
	// }

	// unQuarantinedPayload = TestCaseStatusChange{
	// 	StatusChange: StatusChange{
	// 		CurrentStatus: CurrentStatus{
	// 			Value: "healthy",
	// 		},
	// 		PreviousStatus: "flaky",
	// 	},
	// 	TestCase: healthyTestCase,
	// }
	// webhookSecret is the secret used to sign Trunk webhooks. This is an example secret from the Trunk docs.
	// We use it to sign our own payloads and make them valid for testing.
	webhookSecret = "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"
)

func TestSignWebhookRequest(t *testing.T) {
	t.Parallel()

	l := testhelpers.Logger(t)

	quarantinedPayloadJSON, err := json.Marshal(quarantinedPayload)
	require.NoError(t, err, "failed to marshal payload")

	webhookRequest, err := SelfSignWebhookRequest(l, &http.Request{
		Method: "POST",
		URL:    &url.URL{Path: "/webhooks/trunk"},
		Body:   io.NopCloser(bytes.NewBuffer(quarantinedPayloadJSON)),
	}, webhookSecret)
	require.NoError(t, err, "failed to sign webhook request")

	require.NotNil(t, webhookRequest, "webhook request should not be nil")

	err = VerifyWebhookRequest(l, webhookRequest, webhookSecret)
	require.NoError(t, err, "failed to verify webhook request")
}

func TestExtractRepoNameFromURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		url      string
		expected string
	}{
		{"https://github.com/trunk-io/analytics-cli", "analytics-cli"},
		{"https://github.com/owner/repo", "repo"},
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
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			t.Parallel()
			result := extractDomainFromJiraURL(tt.url)
			require.Equal(t, tt.expected, result, "expected '%s', got '%s'", tt.expected, result)
		})
	}
}
