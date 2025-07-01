package trunk

import (
	"encoding/json"
	"testing"

	"github.com/smartcontractkit/branch-out/jira"
	// TODO: can we get rid of these?
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestTestCaseStatusChangedPayload_ParseRealPayload(t *testing.T) {
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
	require.NoError(t, err, "Should parse the real Trunk test_case.status_changed payload")

	// Verify the parsed structure
	assert.Equal(t, "flaky", payload.StatusChange.CurrentStatus.Value)
	assert.Equal(t, "healthy", payload.StatusChange.PreviousStatus)
	assert.Equal(t, "2bfedccc-7fda-442c-bcf9-5e01c6d046d3", payload.TestCase.ID)
	assert.Equal(t, "DistributedLock #tryLock default throws on double unlock", payload.TestCase.Name)
	assert.Equal(t, "trunk/services/__tests__/distributed_lock.test.js", payload.TestCase.FilePath)
	assert.Equal(t, "@backend", payload.TestCase.Codeowners[0])
	assert.Equal(t, 0.1, payload.TestCase.FailureRateLast7D)
	assert.Equal(t, 42, payload.TestCase.PullRequestsImpactedLast7D)
	assert.True(t, payload.TestCase.Quarantine)
	assert.Equal(t, "https://github.com/trunk-io/analytics-cli", payload.TestCase.Repository.HTMLURL)
	assert.Equal(t, "https://trunk-io.atlassian.net/browse/KAN-130", payload.TestCase.Ticket.HTMLURL)
	assert.Equal(t, "DistributedLock", payload.TestCase.TestSuite)
	assert.Equal(t, "linux-x86_64", payload.TestCase.Variant)
}

func TestExtractRepoNameFromURL(t *testing.T) {
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
			result := extractRepoNameFromURL(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractDomainFromJiraURL(t *testing.T) {
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
			result := extractDomainFromJiraURL(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractRepoInfoFromURL(t *testing.T) {
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
			owner, repo := extractRepoInfoFromURL(tt.url)
			assert.Equal(t, tt.expectedOwner, owner, "Owner mismatch")
			assert.Equal(t, tt.expectedRepo, repo, "Repo name mismatch")
		})
	}
}

// MockJiraClient is a mock implementation of the JiraClient interface
type MockJiraClient struct {
	mock.Mock
}

func (m *MockJiraClient) CreateFlakyTestTicket(req jira.FlakyTestTicketRequest) (*jira.JiraTicketResponse, error) {
	args := m.Called(req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*jira.JiraTicketResponse), args.Error(1)
}

func (m *MockJiraClient) GetTicketStatus(ticketKey string) (*jira.JiraTicketStatus, error) {
	args := m.Called(ticketKey)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*jira.JiraTicketStatus), args.Error(1)
}

func (m *MockJiraClient) AddCommentToTicket(ticketKey string, comment string) error {
	args := m.Called(ticketKey, comment)
	return args.Error(0)
}

// MockTrunkClient is a mock implementation of the TrunkClient interface
type MockTrunkClient struct {
	mock.Mock
}

func (m *MockTrunkClient) LinkTicketToTestCase(testCaseID string, ticket *jira.JiraTicketResponse, repoURL string) error {
	args := m.Called(testCaseID, ticket, repoURL)
	return args.Error(0)
}
