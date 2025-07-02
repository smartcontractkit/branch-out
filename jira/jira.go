// Package jira provides utilities for the Jira API.
package jira

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/oauth2"
)

// FlakyTestTicketRequest represents the data needed to create a Jira ticket for a flaky test
type FlakyTestTicketRequest struct {
	RepoName        string `json:"repo_name"`
	TestPackageName string `json:"test_package_name"`
	FilePath        string `json:"file_path"`
	TrunkID         string `json:"trunk_id"` // UUID
	Details         string `json:"details"`  // JSON string with additional details (trunk Payload for example)
}

// Config holds the configuration for Jira API client
type Config struct {
	BaseURL    string
	ProjectKey string

	// OAuth Configuration
	ClientID     string
	ClientSecret string
	AccessToken  string
	RefreshToken string

	// Legacy Basic Auth (optional fallback or for local testing)
	Username string
	APIToken string
}

// Interface for interacting with Jira API (for testability)
type Interface interface {
	CreateFlakyTestTicket(req FlakyTestTicketRequest) (*TicketResponse, error)
	GetTicketStatus(ticketKey string) (*TicketStatus, error)
	AddCommentToTicket(ticketKey string, comment string) error
}

// HTTPClient interface for HTTP requests (for testability)
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client implements Interface
type Client struct {
	config     Config
	httpClient HTTPClient
	logger     zerolog.Logger
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

	for _, resolved := range resolvedCategories {
		if statusCategory == resolved {
			return true
		}
	}
	return false
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

// NewClient creates a new Jira client with configuration from environment variables
func NewClient(logger zerolog.Logger, projectKey string) (*Client, error) {
	baseDomain := os.Getenv("JIRA_BASE_DOMAIN")
	if baseDomain == "" {
		return nil, fmt.Errorf("JIRA_BASE_DOMAIN environment variable is required (e.g., 'your-company.atlassian.net')")
	}

	baseURL := "https://" + baseDomain

	config := Config{
		BaseURL:    baseURL,
		ProjectKey: projectKey,
		// OAuth creds.
		ClientID:     os.Getenv("JIRA_OAUTH_CLIENT_ID"),
		ClientSecret: os.Getenv("JIRA_OAUTH_CLIENT_SECRET"),
		AccessToken:  os.Getenv("JIRA_OAUTH_ACCESS_TOKEN"),
		RefreshToken: os.Getenv("JIRA_OAUTH_REFRESH_TOKEN"),
		// Legacy Basic Auth.
		Username: os.Getenv("JIRA_USERNAME"),
		APIToken: os.Getenv("JIRA_API_TOKEN"),
	}

	if config.ProjectKey == "" {
		return nil, fmt.Errorf("project key is required")
	}

	// Check if OAuth credentials are provided
	hasOAuth := config.ClientID != "" && config.ClientSecret != "" && config.AccessToken != ""
	hasBasicAuth := config.Username != "" && config.APIToken != ""

	if !hasOAuth && !hasBasicAuth {
		return nil, fmt.Errorf(
			"either OAuth credentials (JIRA_OAUTH_CLIENT_ID, JIRA_OAUTH_CLIENT_SECRET, JIRA_OAUTH_ACCESS_TOKEN) or basic auth credentials (JIRA_USERNAME, JIRA_API_TOKEN) are required",
		)
	}

	// Ensure BaseURL doesn't have trailing slash
	config.BaseURL = strings.TrimSuffix(config.BaseURL, "/")

	// Create HTTP client with OAuth if available
	var httpClient HTTPClient
	if hasOAuth {
		oauthConfig := &oauth2.Config{
			ClientID:     config.ClientID,
			ClientSecret: config.ClientSecret,
			Endpoint: oauth2.Endpoint{
				TokenURL: fmt.Sprintf("%s/plugins/servlet/oauth/access-token", config.BaseURL),
			},
		}

		token := &oauth2.Token{
			AccessToken:  config.AccessToken,
			RefreshToken: config.RefreshToken,
			TokenType:    "Bearer",
		}

		httpClient = oauthConfig.Client(context.Background(), token)
	} else {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &Client{
		config:     config,
		httpClient: httpClient,
		logger:     logger.With().Str("component", "jira_client").Logger(),
	}, nil
}

// NewClientWithHTTPClient creates a new Jira client with a custom HTTP client (for testing)
func NewClientWithHTTPClient(config Config, httpClient HTTPClient, logger zerolog.Logger) *Client {
	return &Client{
		config:     config,
		httpClient: httpClient,
		logger:     logger.With().Str("component", "jira_client").Logger(),
	}
}

// CreateFlakyTestTicket creates a new Jira ticket for a flaky test
func (c *Client) CreateFlakyTestTicket(req FlakyTestTicketRequest) (*TicketResponse, error) {
	c.logger.Info().
		Str("repo_name", req.RepoName).
		Str("test_package", req.TestPackageName).
		Str("file_path", req.FilePath).
		Str("trunk_id", req.TrunkID).
		Msg("Creating Jira ticket for flaky test")

	// Construct the ticket summary and description
	summary := fmt.Sprintf("Flaky Test: %s in %s", req.TestPackageName, req.RepoName)

	description := fmt.Sprintf(`*Flaky Test Detected*

*Repository:* %s
*Test Package:* %s
*File Path:* %s
*Trunk ID:* %s

*Additional Details:*
{code:json}
%s
{code}

This ticket was automatically created by the branch-out system to track a flaky test that has been identified.`,
		req.RepoName,
		req.TestPackageName,
		req.FilePath,
		req.TrunkID,
		req.Details)

	// Create the Jira issue request
	jiraReq := CreateIssueRequest{
		Fields: IssueFields{
			Project: Project{
				Key: c.config.ProjectKey,
			},
			Summary:     summary,
			Description: description,
			IssueType: IssueType{
				Name: "Bug", // Default to Bug, could be configurable
			},
			Labels: []string{"flaky-test", "automated", "branch-out"},
		},
	}

	// Convert to JSON
	jsonData, err := json.Marshal(jiraReq)
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to marshal Jira request")
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/rest/api/2/issue", c.config.BaseURL)
	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		c.logger.Error().Err(err).Str("url", url).Msg("Failed to create HTTP request")
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	// Set authentication based on available credentials
	if c.config.Username != "" && c.config.APIToken != "" {
		// Fallback to basic auth
		httpReq.SetBasicAuth(c.config.Username, c.config.APIToken)
	} else {
		c.logger.Error().Msg("No valid authentication credentials available")
		return nil, fmt.Errorf("no valid authentication credentials available")
	}

	// Make the request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to make HTTP request to Jira")
		return nil, fmt.Errorf("failed to make HTTP request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			c.logger.Error().Err(closeErr).Msg("Failed to close response body")
		}
	}()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to read response body")
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check if request was successful
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.logger.Error().
			Int("status_code", resp.StatusCode).
			Str("response_body", string(body)).
			Msg("Jira API returned error")
		return nil, fmt.Errorf("jira API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var jiraResp TicketResponse
	if err := json.Unmarshal(body, &jiraResp); err != nil {
		c.logger.Error().Err(err).Str("response_body", string(body)).Msg("Failed to unmarshal Jira response")
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	c.logger.Info().
		Str("ticket_key", jiraResp.Key).
		Str("ticket_id", jiraResp.ID).
		Msg("Successfully created Jira ticket for flaky test")

	return &jiraResp, nil
}

// GetTicketStatus retrieves the current status of a Jira ticket
func (c *Client) GetTicketStatus(ticketKey string) (*TicketStatus, error) {
	c.logger.Info().Str("ticket_key", ticketKey).Msg("Getting Jira ticket status")

	// Create the API URL
	url := fmt.Sprintf("%s/rest/api/2/issue/%s?fields=status", c.config.BaseURL, ticketKey)

	// Create HTTP request
	httpReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to create HTTP request for getting ticket status")
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Accept", "application/json")

	// Set authentication
	if c.config.Username != "" && c.config.APIToken != "" {
		httpReq.SetBasicAuth(c.config.Username, c.config.APIToken)
	} else {
		c.logger.Error().Msg("No valid authentication credentials available")
		return nil, fmt.Errorf("no valid authentication credentials available")
	}

	// Make the request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to make HTTP request to get ticket status")
		return nil, fmt.Errorf("failed to make HTTP request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			c.logger.Error().Err(closeErr).Msg("Failed to close response body")
		}
	}()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to read response body")
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check if request was successful
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.logger.Error().
			Int("status_code", resp.StatusCode).
			Str("response_body", string(body)).
			Msg("Jira API returned error when getting ticket status")
		return nil, fmt.Errorf("jira API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var ticketStatus TicketStatus
	if err := json.Unmarshal(body, &ticketStatus); err != nil {
		c.logger.Error().Err(err).Str("response_body", string(body)).Msg("Failed to unmarshal ticket status response")
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	c.logger.Info().
		Str("ticket_key", ticketKey).
		Str("status", ticketStatus.Fields.Status.Name).
		Str("status_category", ticketStatus.Fields.Status.StatusCategory.Key).
		Msg("Successfully retrieved ticket status")

	return &ticketStatus, nil
}

// AddCommentToTicket adds a comment to an existing Jira ticket
func (c *Client) AddCommentToTicket(ticketKey string, comment string) error {
	c.logger.Info().Str("ticket_key", ticketKey).Msg("Adding comment to Jira ticket")

	// Create the API URL
	url := fmt.Sprintf("%s/rest/api/2/issue/%s/comment", c.config.BaseURL, ticketKey)

	// Create request body
	requestBody := map[string]interface{}{
		"body": comment,
	}

	// Marshal request body
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to marshal comment request")
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to create HTTP request for adding comment")
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	// Set authentication
	if c.config.Username != "" && c.config.APIToken != "" {
		httpReq.SetBasicAuth(c.config.Username, c.config.APIToken)
	} else {
		c.logger.Error().Msg("No valid authentication credentials available")
		return fmt.Errorf("no valid authentication credentials available")
	}

	// Make the request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to make HTTP request to add comment")
		return fmt.Errorf("failed to make HTTP request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			c.logger.Error().Err(closeErr).Msg("Failed to close response body")
		}
	}()

	// Read response body for error details
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to read response body")
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Check if request was successful
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.logger.Error().
			Int("status_code", resp.StatusCode).
			Str("response_body", string(body)).
			Msg("Jira API returned error when adding comment")
		return fmt.Errorf("jira API error (status %d): %s", resp.StatusCode, string(body))
	}

	c.logger.Info().
		Str("ticket_key", ticketKey).
		Msg("Successfully added comment to Jira ticket")

	return nil
}

// IsOAuthEnabled returns true if OAuth authentication is configured
func (c *Client) IsOAuthEnabled() bool {
	return c.config.AccessToken != ""
}

// AuthType returns the type of authentication being used
func (c *Client) AuthType() string {
	if c.IsOAuthEnabled() {
		return "OAuth"
	} else if c.config.Username != "" && c.config.APIToken != "" {
		return "Basic Auth"
	}
	return "None"
}
