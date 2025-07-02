// Package trunk provides models for the Trunk.io API.
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
	Codeowners         []string `json:"codeowners"`
	FailureRateLast7D  float64  `json:"failure_rate_last_7d"`
	FilePath           string   `json:"file_path"`
	HTMLURL            string   `json:"html_url"`
	ID                 string   `json:"id"`
	MostCommonFailures []struct {
		LastOccurrence  string `json:"last_occurrence"`
		OccurrenceCount int    `json:"occurrence_count"`
		Summary         string `json:"summary"`
	} `json:"most_common_failures"`
	Name                       string `json:"name"`
	PullRequestsImpactedLast7D int    `json:"pull_requests_impacted_last_7d"`
	Quarantine                 bool   `json:"quarantine"`
	Repository                 struct {
		HTMLURL string `json:"html_url"`
	} `json:"repository"`
	Status struct {
		Reason    string `json:"reason"`
		Timestamp string `json:"timestamp"`
		Value     string `json:"value"`
	} `json:"status"`
	TestSuite string `json:"test_suite"`
	Ticket    struct {
		HTMLURL string `json:"html_url"`
	} `json:"ticket"`
	Variant string `json:"variant"`
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

// StatusChanged is the event type for when a test case's status is changed.
// https://www.svix.com/event-types/us/org_2eQPL41Ew5XSHxiXZIamIUIXg8H/#test_case.status_changed
type StatusChanged struct {
	StatusChange struct {
		CurrentStatus struct {
			Reason    string    `json:"reason"`
			Timestamp time.Time `json:"timestamp"`
			Value     string    `json:"value"`
		} `json:"current_status"`
		PreviousStatus string `json:"previous_status"`
	} `json:"status_change"`
	TestCase TestCase `json:"test_case"`
	Type     string   `json:"type"` // Added missing Type field
}

// GetType implements the WebhookEvent interface
func (s StatusChanged) GetType() WebhookType {
	return WebhookTypeStatusChanged
}

// GetTestCase implements the WebhookEvent interface
func (s StatusChanged) GetTestCase() TestCase {
	return s.TestCase
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
		var event StatusChanged
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

// TestCaseStatusChangedPayload represents the payload for test_case.status_changed events from Trunk.io
// See: https://www.svix.com/event-types/us/org_2eQPL41Ew5XSHxiXZIamIUIXg8H/#test_case.status_changed
type TestCaseStatusChangedPayload struct {
	StatusChange struct {
		CurrentStatus struct {
			Reason    string `json:"reason"`
			Timestamp string `json:"timestamp"`
			Value     string `json:"value"`
		} `json:"current_status"`
		PreviousStatus string `json:"previous_status"`
	} `json:"status_change"`
	TestCase TestCase `json:"test_case"` // Reuse the existing TestCase struct
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
