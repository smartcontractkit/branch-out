package trunk

import (
	"encoding/json"
	"testing"
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
	if err != nil {
		t.Fatalf("Should parse the real Trunk test_case.status_changed payload: %v", err)
	}

	// Verify the parsed structure
	if payload.StatusChange.CurrentStatus.Value != "flaky" {
		t.Errorf("Expected current status 'flaky', got '%s'", payload.StatusChange.CurrentStatus.Value)
	}
	if payload.StatusChange.PreviousStatus != "healthy" {
		t.Errorf("Expected previous status 'healthy', got '%s'", payload.StatusChange.PreviousStatus)
	}
	if payload.TestCase.ID != "2bfedccc-7fda-442c-bcf9-5e01c6d046d3" {
		t.Errorf("Expected test case ID '2bfedccc-7fda-442c-bcf9-5e01c6d046d3', got '%s'", payload.TestCase.ID)
	}
	if payload.TestCase.Name != "DistributedLock #tryLock default throws on double unlock" {
		t.Errorf(
			"Expected test name 'DistributedLock #tryLock default throws on double unlock', got '%s'",
			payload.TestCase.Name,
		)
	}
	if payload.TestCase.FilePath != "trunk/services/__tests__/distributed_lock.test.js" {
		t.Errorf(
			"Expected file path 'trunk/services/__tests__/distributed_lock.test.js', got '%s'",
			payload.TestCase.FilePath,
		)
	}
	if len(payload.TestCase.Codeowners) == 0 || payload.TestCase.Codeowners[0] != "@backend" {
		t.Errorf("Expected first codeowner '@backend', got %v", payload.TestCase.Codeowners)
	}
	if payload.TestCase.FailureRateLast7D != 0.1 {
		t.Errorf("Expected failure rate 0.1, got %f", payload.TestCase.FailureRateLast7D)
	}
	if payload.TestCase.PullRequestsImpactedLast7D != 42 {
		t.Errorf("Expected 42 impacted PRs, got %d", payload.TestCase.PullRequestsImpactedLast7D)
	}
	if !payload.TestCase.Quarantine {
		t.Error("Expected quarantine to be true")
	}
	if payload.TestCase.Repository.HTMLURL != "https://github.com/trunk-io/analytics-cli" {
		t.Errorf(
			"Expected repository URL 'https://github.com/trunk-io/analytics-cli', got '%s'",
			payload.TestCase.Repository.HTMLURL,
		)
	}
	if payload.TestCase.Ticket.HTMLURL != "https://trunk-io.atlassian.net/browse/KAN-130" {
		t.Errorf(
			"Expected ticket URL 'https://trunk-io.atlassian.net/browse/KAN-130', got '%s'",
			payload.TestCase.Ticket.HTMLURL,
		)
	}
	if payload.TestCase.TestSuite != "DistributedLock" {
		t.Errorf("Expected test suite 'DistributedLock', got '%s'", payload.TestCase.TestSuite)
	}
	if payload.TestCase.Variant != "linux-x86_64" {
		t.Errorf("Expected variant 'linux-x86_64', got '%s'", payload.TestCase.Variant)
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
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
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
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
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
			if owner != tt.expectedOwner {
				t.Errorf("Owner mismatch: expected '%s', got '%s'", tt.expectedOwner, owner)
			}
			if repo != tt.expectedRepo {
				t.Errorf("Repo name mismatch: expected '%s', got '%s'", tt.expectedRepo, repo)
			}
		})
	}
}
