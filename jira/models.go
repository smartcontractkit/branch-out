package jira

import (
	"slices"
	"strings"
)

// FlakyTestTicketRequest represents the data needed to create a Jira ticket for a flaky test
type FlakyTestTicketRequest struct {
	RepoName        string `json:"repo_name"`
	TestPackageName string `json:"test_package_name"`
	FilePath        string `json:"file_path"`
	TrunkID         string `json:"trunk_id"` // UUID
	Details         string `json:"details"`  // JSON string with additional details (trunk Payload for example)
}

// TicketResponse represents the response from Jira when creating a ticket
type TicketResponse struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Self string `json:"self"`
}

// TicketStatus represents the status information of a Jira ticket
type TicketStatus struct {
	Key    string `json:"key"`
	Status struct {
		Name       string `json:"name"`
		StatusCode string `json:"statusCategory"`
	} `json:"status"`
	Fields struct {
		Status struct {
			Name           string `json:"name"`
			StatusCategory struct {
				Key  string `json:"key"`
				Name string `json:"name"`
			} `json:"statusCategory"`
		} `json:"status"`
	} `json:"fields"`
}

// IsResolved returns true if the ticket is in a resolved/closed state
func (jts *TicketStatus) IsResolved() bool {
	// Common resolved status categories in Jira
	resolvedCategories := []string{"done", "complete", "resolved", "closed"}
	statusCategory := strings.ToLower(jts.Fields.Status.StatusCategory.Key)

	return slices.Contains(resolvedCategories, statusCategory)
}

// CreateIssueRequest represents the request body for creating a Jira issue.
type CreateIssueRequest struct {
	Fields IssueFields `json:"fields"`
}

// IssueFields represents the fields for a Jira issue
type IssueFields struct {
	Project     Project   `json:"project"`
	Summary     string    `json:"summary"`
	Description string    `json:"description"`
	IssueType   IssueType `json:"issuetype"`
	Labels      []string  `json:"labels"`
}

// Project represents a Jira project
type Project struct {
	Key string `json:"key"`
}

// IssueType represents a Jira issue type
type IssueType struct {
	Name string `json:"name"`
}
