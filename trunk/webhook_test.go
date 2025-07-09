package trunk

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Parse_TestCaseStatusChangedPayload(t *testing.T) {
	t.Parallel()

	payloadJSON, err := os.ReadFile("testdata/status_changed_webhook.json")
	require.NoError(t, err, "cannot read the Trunk payload")

	var payload TestCaseStatusChangedPayload
	err = json.Unmarshal(payloadJSON, &payload)
	require.NoError(t, err, "cannot parse the Trunk payload")

	// Verify the parsed structure
	assert.Equal(t, "flaky", payload.StatusChange.CurrentStatus.Value, "expected current status 'flaky'")
	assert.Equal(t, "healthy", payload.StatusChange.PreviousStatus, "expected previous status 'healthy'")
	assert.Equal(
		t,
		"2bfedccc-7fda-442c-bcf9-5e01c6d046d3",
		payload.TestCase.ID,
		"expected test case ID '2bfedccc-7fda-442c-bcf9-5e01c6d046d3'",
	)
	assert.Equal(
		t,
		"DistributedLock #tryLock default throws on double unlock",
		payload.TestCase.Name,
		"expected test name 'DistributedLock #tryLock default throws on double unlock'",
	)
	assert.Equal(
		t,
		"trunk/services/__tests__/distributed_lock.test.js",
		payload.TestCase.FilePath,
		"expected file path 'trunk/services/__tests__/distributed_lock.test.js'",
	)
	assert.Equal(t, "@backend", payload.TestCase.Codeowners[0], "expected first codeowner '@backend'")
	assert.InEpsilon(t, 0.1, payload.TestCase.FailureRateLast7D, 0.0000000000000001, "expected failure rate 0.1")
	assert.Equal(t, 42, payload.TestCase.PullRequestsImpactedLast7D, "expected 42 impacted PRs")
	assert.True(t, payload.TestCase.Quarantine, "expected quarantine to be true")
	assert.Equal(
		t,
		"https://github.com/trunk-io/analytics-cli",
		payload.TestCase.Repository.HTMLURL,
		"expected repository URL 'https://github.com/trunk-io/analytics-cli'",
	)
	assert.Equal(
		t,
		"https://trunk-io.atlassian.net/browse/KAN-130",
		payload.TestCase.Ticket.HTMLURL,
		"expected ticket URL 'https://trunk-io.atlassian.net/browse/KAN-130'",
	)
	assert.Equal(t, "DistributedLock", payload.TestCase.TestSuite, "expected test suite 'DistributedLock'")
	assert.Equal(t, "linux-x86_64", payload.TestCase.Variant, "expected variant 'linux-x86_64'")
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
