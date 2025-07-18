package trunk

import (
	"encoding/json"
	"fmt"
	"time"
)

// WebhookType represents the type of webhook event
type WebhookType string

const (
	// WebhookTypeQuarantiningSettingChanged represents a quarantining setting change event
	WebhookTypeQuarantiningSettingChanged WebhookType = "test_case.quarantining_setting_changed"
	// WebhookTypeStatusChanged represents a status change event
	WebhookTypeStatusChanged WebhookType = "test_case.status_changed"

	// TestCaseStatusHealthy is the status of a test that is healthy.
	TestCaseStatusHealthy = "healthy"
	// TestCaseStatusFlaky is the status of a test that is flaky.
	TestCaseStatusFlaky = "flaky"
	// TestCaseStatusBroken is the status of a test that is broken.
	TestCaseStatusBroken = "broken"
)

// WebhookEnvelope is the common structure for all webhook events
type WebhookEnvelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"-"` // The entire JSON payload for specific parsing
}

// WebhookEvent is an interface that all webhook events must implement
type WebhookEvent interface {
	GetType() WebhookType
	GetTestCase() TestCase // Common test case information
}

// TestCase is the common structure for all test case events.
type TestCase struct {
	Codeowners                 []string            `json:"codeowners"`
	FailureRateLast7D          float64             `json:"failure_rate_last_7d"`
	FilePath                   string              `json:"file_path"`
	HTMLURL                    string              `json:"html_url"`
	ID                         string              `json:"id"`
	MostCommonFailures         []MostCommonFailure `json:"most_common_failures"`
	Name                       string              `json:"name"`
	PullRequestsImpactedLast7D int                 `json:"pull_requests_impacted_last_7d"`
	Quarantine                 bool                `json:"quarantine"`
	Repository                 Repository          `json:"repository"`
	Status                     Status              `json:"status"`
	TestSuite                  string              `json:"test_suite"`
	Ticket                     Ticket              `json:"ticket"`
	Variant                    string              `json:"variant"`
}

// Repository represents the repository URL associated with a test case.
type Repository struct {
	HTMLURL string `json:"html_url"`
}

// MostCommonFailure represents the most common failure for a test case.
type MostCommonFailure struct {
	LastOccurrence  string `json:"last_occurrence"`
	OccurrenceCount int    `json:"occurrence_count"`
	Summary         string `json:"summary"`
}

// Ticket represents the Jira ticket associated with a test case.
type Ticket struct {
	HTMLURL string `json:"html_url"`
}

// QuarantiningSettingChanged is the event type for when a test case's quarantining setting is changed.
// https://www.svix.com/event-types/us/org_2eQPL41Ew5XSHxiXZIamIUIXg8H/#test_case.quarantining_setting_changed
type QuarantiningSettingChanged struct {
	QuarantineSettingChanged struct {
		Actor struct {
			Email    string `json:"email"`
			FullName string `json:"full_name"`
		} `json:"actor"`
		PreviousQuarantiningSetting string    `json:"previous_quarantining_setting"`
		Reason                      string    `json:"reason"`
		Timestamp                   time.Time `json:"timestamp"`
		UpdatedQuarantiningSetting  string    `json:"updated_quarantining_setting"`
	} `json:"quarantine_setting_changed"`
	TestCase TestCase `json:"test_case"`
	Type     string   `json:"type"`
}

// GetType implements the WebhookEvent interface
func (q QuarantiningSettingChanged) GetType() WebhookType {
	return WebhookTypeQuarantiningSettingChanged
}

// GetTestCase implements the WebhookEvent interface
func (q QuarantiningSettingChanged) GetTestCase() TestCase {
	return q.TestCase
}

// ParseWebhookEvent parses raw JSON into the appropriate webhook event type
func ParseWebhookEvent(data []byte) (WebhookEvent, error) {
	// First, parse just the type field
	var envelope WebhookEnvelope
	envelope.Data = data

	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse webhook envelope: %w", err)
	}

	// Parse into specific type based on the type field
	switch WebhookType(envelope.Type) {
	case WebhookTypeQuarantiningSettingChanged:
		var event QuarantiningSettingChanged
		if err := json.Unmarshal(data, &event); err != nil {
			return nil, fmt.Errorf("failed to parse quarantining setting changed event: %w", err)
		}
		return event, nil

	case WebhookTypeStatusChanged:
		var event TestCaseStatusChange
		if err := json.Unmarshal(data, &event); err != nil {
			return nil, fmt.Errorf("failed to parse status changed event: %w", err)
		}
		return event, nil

	default:
		return nil, fmt.Errorf("unknown webhook type: %s", envelope.Type)
	}
}

// GetWebhookType quickly extracts just the type from raw JSON without full parsing
func GetWebhookType(data []byte) (WebhookType, error) {
	var envelope struct {
		Type string `json:"type"`
	}

	if err := json.Unmarshal(data, &envelope); err != nil {
		return "", fmt.Errorf("failed to parse webhook type: %w", err)
	}

	return WebhookType(envelope.Type), nil
}

// TestCaseStatusChange represents the payload for test_case.status_changed events from Trunk.io
// See: https://www.svix.com/event-types/us/org_2eQPL41Ew5XSHxiXZIamIUIXg8H/#test_case.status_changed
type TestCaseStatusChange struct {
	StatusChange StatusChange `json:"status_change"`
	TestCase     TestCase     `json:"test_case"`
}

// GetType implements the WebhookEvent interface
func (s TestCaseStatusChange) GetType() WebhookType {
	return WebhookTypeStatusChanged
}

// GetTestCase implements the WebhookEvent interface
func (s TestCaseStatusChange) GetTestCase() TestCase {
	return s.TestCase
}

// StatusChange represents the status change event from Trunk.io
// See: https://www.svix.com/event-types/us/org_2eQPL41Ew5XSHxiXZIamIUIXg8H/#test_case.status_changed
type StatusChange struct {
	CurrentStatus  Status `json:"current_status"`
	PreviousStatus string `json:"previous_status"`
}

// Status represents the current status of a test case
// See: https://www.svix.com/event-types/us/org_2eQPL41Ew5XSHxiXZIamIUIXg8H/#test_case.status_changed
type Status struct {
	Reason    string `json:"reason"`
	Timestamp string `json:"timestamp"`
	Value     string `json:"value"`
}

// LinkTicketRequest represents the request to link a Jira ticket to a test case in Trunk.io
// See: https://docs.trunk.io/references/apis/flaky-tests#post-flaky-tests-link-ticket-to-test-case
type LinkTicketRequest struct {
	TestCaseID       string        `json:"test_case_id"`
	ExternalTicketID string        `json:"external_ticket_id"`
	Repo             RepoReference `json:"repo"`
}

// RepoReference represents the repository information for Trunk.io API
type RepoReference struct {
	Host  string `json:"host"`
	Owner string `json:"owner"`
	Name  string `json:"name"`
}

// QuarantinedTestsRequest represents the request to list quarantined tests in Trunk.io
// See: https://docs.trunk.io/references/apis/flaky-tests#post-flaky-tests-list-quarantined-tests
type QuarantinedTestsRequest struct {
	Repo       RepoReference `json:"repo"`
	OrgURLSlug string        `json:"org_url_slug"`
	PageQuery  PageQuery     `json:"page_query"`
}

// QuarantinedTestsResponse represents the response from the Trunk.io API for quarantined tests
// See: https://docs.trunk.io/references/apis/flaky-tests#post-flaky-tests-list-quarantined-tests
type QuarantinedTestsResponse struct {
	QuarantinedTests []TestCase   `json:"quarantined_tests"`
	PageResponse     PageResponse `json:"page_response"`
}

// PageQuery represents the pagination query parameters for Trunk.io API
// See: https://docs.trunk.io/references/apis/flaky-tests
type PageQuery struct {
	PageToken string `json:"page_token"`
	PageSize  int    `json:"page_size"`
}

// PageResponse represents the pagination response from the Trunk.io API
// See: https://docs.trunk.io/references/apis/flaky-tests
type PageResponse struct {
	TotalRows         int    `json:"total_rows"`
	TotalPages        int    `json:"total_pages"`
	NextPageToken     string `json:"next_page_token"`
	PreviousPageToken string `json:"previous_page_token"`
	LastPageToken     string `json:"last_page_token"`
	PageIndex         int    `json:"page_index"`
}
