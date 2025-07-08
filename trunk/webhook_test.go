package trunk

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTestCaseStatusChangedPayload_ParseRealPayload(t *testing.T) {
	t.Parallel()
	payloadJSON := `{
		"status_change": {
			"current_status": {
				"reason": " Inconsistent results on main",
				"timestamp": "2024-11-22T20:04:33.127Z",
				"value": "flaky"
			},
			"previous_status": "healthy"
		},
		"test_case": {
			"codeowners": [
				"@backend"
			],
			"failure_rate_last_7d": 0.1,
			"file_path": "trunk/services/__tests__/distributed_lock.test.js",
			"html_url": "https://app.trunk.io/trunk/flaky-tests/test/77681877-1608-5871-bf95-e02f8baa5a9a?repo=trunk-io%2Fanalytics-cli",
			"id": "2bfedccc-7fda-442c-bcf9-5e01c6d046d3",
			"most_common_failures": [
				{
					"last_occurrence": "2024-11-22T20:04:33.127Z",
					"occurrence_count": 42,
					"summary": "timeout"
				}
			],
			"name": "DistributedLock #tryLock default throws on double unlock",
			"pull_requests_impacted_last_7d": 42,
			"quarantine": true,
			"repository": {
				"html_url": "https://github.com/trunk-io/analytics-cli"
			},
			"status": {
				"reason": " Inconsistent results on main",
				"timestamp": "2024-11-22T20:04:33.127Z",
				"value": "flaky"
			},
			"test_suite": "DistributedLock",
			"ticket": {
				"html_url": "https://trunk-io.atlassian.net/browse/KAN-130"
			},
			"variant": "linux-x86_64"
		}
	}`

	var payload TestCaseStatusChangedPayload
	err := json.Unmarshal([]byte(payloadJSON), &payload)
	require.NoError(t, err, "cannot parse the Trunk payload")

	// Verify the parsed structure
	require.Equal(t, "flaky", payload.StatusChange.CurrentStatus.Value, "expected current status 'flaky'")
	require.Equal(t, "healthy", payload.StatusChange.PreviousStatus, "expected previous status 'healthy'")
	require.Equal(
		t,
		"2bfedccc-7fda-442c-bcf9-5e01c6d046d3",
		payload.TestCase.ID,
		"expected test case ID '2bfedccc-7fda-442c-bcf9-5e01c6d046d3'",
	)
	require.Equal(
		t,
		"DistributedLock #tryLock default throws on double unlock",
		payload.TestCase.Name,
		"expected test name 'DistributedLock #tryLock default throws on double unlock'",
	)
	require.Equal(
		t,
		"trunk/services/__tests__/distributed_lock.test.js",
		payload.TestCase.FilePath,
		"expected file path 'trunk/services/__tests__/distributed_lock.test.js'",
	)
	require.Equal(t, "@backend", payload.TestCase.Codeowners[0], "expected first codeowner '@backend'")
	require.InEpsilon(t, 0.1, payload.TestCase.FailureRateLast7D, 0.0000000000000001, "expected failure rate 0.1")
	require.Equal(t, 42, payload.TestCase.PullRequestsImpactedLast7D, "expected 42 impacted PRs")
	require.True(t, payload.TestCase.Quarantine, "expected quarantine to be true")
	require.Equal(
		t,
		"https://github.com/trunk-io/analytics-cli",
		payload.TestCase.Repository.HTMLURL,
		"expected repository URL 'https://github.com/trunk-io/analytics-cli'",
	)
	require.Equal(
		t,
		"https://trunk-io.atlassian.net/browse/KAN-130",
		payload.TestCase.Ticket.HTMLURL,
		"expected ticket URL 'https://trunk-io.atlassian.net/browse/KAN-130'",
	)
	require.Equal(t, "DistributedLock", payload.TestCase.TestSuite, "expected test suite 'DistributedLock'")
	require.Equal(t, "linux-x86_64", payload.TestCase.Variant, "expected variant 'linux-x86_64'")
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
