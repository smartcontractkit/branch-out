// Package trunk provides models for the Trunk.io API.
package trunk

import "time"

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
	TestCase struct {
		Codeowners         []string `json:"codeowners"`
		FailureRateLast7D  int      `json:"failure_rate_last_7d"`
		FilePath           string   `json:"file_path"`
		HTMLURL            string   `json:"html_url"`
		ID                 string   `json:"id"`
		MostCommonFailures []struct {
			LastOccurrence  time.Time `json:"last_occurrence"`
			OccurrenceCount int       `json:"occurrence_count"`
			Summary         string    `json:"summary"`
		} `json:"most_common_failures"`
		Name                       string `json:"name"`
		PullRequestsImpactedLast7D int    `json:"pull_requests_impacted_last_7d"`
		Quarantined                bool   `json:"quarantined"`
		Repository                 struct {
			HTMLURL string `json:"html_url"`
		} `json:"repository"`
		Status struct {
			Reason    string    `json:"reason"`
			Timestamp time.Time `json:"timestamp"`
			Value     string    `json:"value"`
		} `json:"status"`
		TestSuite string `json:"test_suite"`
		Ticket    struct {
			HTMLURL string `json:"html_url"`
		} `json:"ticket"`
		Variant string `json:"variant"`
	} `json:"test_case"`
	Type string `json:"type"`
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
	TestCase struct {
		Codeowners         []string `json:"codeowners"`
		FailureRateLast7D  float64  `json:"failure_rate_last_7d"`
		FilePath           string   `json:"file_path"`
		HTMLURL            string   `json:"html_url"`
		ID                 string   `json:"id"`
		MostCommonFailures []struct {
			LastOccurrence  time.Time `json:"last_occurrence"`
			OccurrenceCount int       `json:"occurrence_count"`
			Summary         string    `json:"summary"`
		} `json:"most_common_failures"`
		Name                       string `json:"name"`
		PullRequestsImpactedLast7D int    `json:"pull_requests_impacted_last_7d"`
		Quarantine                 bool   `json:"quarantine"`
		Repository                 struct {
			HTMLURL string `json:"html_url"`
		} `json:"repository"`
		Status struct {
			Reason    string    `json:"reason"`
			Timestamp time.Time `json:"timestamp"`
			Value     string    `json:"value"`
		} `json:"status"`
		TestSuite string `json:"test_suite"`
		Ticket    struct {
			HTMLURL string `json:"html_url"`
		} `json:"ticket"`
		Variant string `json:"variant"`
	} `json:"test_case"`
}
