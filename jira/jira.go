// Package jira provides utilities for the Jira API.
package jira

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/oauth2"

	"github.com/smartcontractkit/branch-out/config"
)

// FlakyTestTicketRequest represents the data needed to create a Jira ticket for a flaky test
type FlakyTestTicketRequest struct {
	RepoName        string `json:"repo_name"`
	TestPackageName string `json:"test_package_name"`
	FilePath        string `json:"file_path"`
	TrunkID         string `json:"trunk_id"` // UUID
	Details         string `json:"details"`  // JSON string with additional details (trunk Payload for example)
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
	baseURL    string
	secrets    config.Jira
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

// Option is a function that sets a configuration option for the Jira client.
type Option func(*jiraClientOptions)

type jiraClientOptions struct {
	config     *config.Config
	logger     zerolog.Logger
	httpClient HTTPClient
}

// WithLogger sets the logger to use for the Jira client.
func WithLogger(logger zerolog.Logger) Option {
	return func(cfg *jiraClientOptions) {
		cfg.logger = logger
	}
}

// WithConfig sets the config to use for the Jira client.
func WithConfig(config *config.Config) Option {
	return func(cfg *jiraClientOptions) {
		cfg.config = config
	}
}

// WithHTTPClient sets the HTTP client to use for the Jira client.
func WithHTTPClient(httpClient HTTPClient) Option {
	return func(cfg *jiraClientOptions) {
		cfg.httpClient = httpClient
	}
}

// NewClient creates a new Jira client with configuration from environment variables
func NewClient(options ...Option) (*Client, error) {
	opts := &jiraClientOptions{
		logger: zerolog.Nop(),
	}
	for _, opt := range options {
		opt(opts)
	}

	var (
		appConfig = opts.config
		err       error
	)
	if appConfig == nil {
		appConfig, err = config.Load()
		if err != nil {
			return nil, err
		}
	}

	if appConfig.Jira.BaseDomain == "" {
		return nil, fmt.Errorf("jira base domain is required")
	}

	if appConfig.Jira.ProjectKey == "" {
		return nil, fmt.Errorf("jira project key is required")
	}

	// Check if OAuth credentials are provided
	hasOAuth := appConfig.Jira.OAuthClientID != "" && appConfig.Jira.OAuthClientSecret != "" &&
		appConfig.Jira.OAuthAccessToken != ""
	hasBasicAuth := appConfig.Jira.Username != "" && appConfig.Jira.Token != ""

	if !hasOAuth && !hasBasicAuth {
		return nil, fmt.Errorf(
			"jira OAuth credentials or basic auth credentials are required",
		)
	}

	baseURL := fmt.Sprintf("https://%s", appConfig.Jira.BaseDomain)
	tokenURL, err := url.JoinPath(baseURL, "plugins/servlet/oauth/access-token")
	if err != nil {
		return nil, fmt.Errorf("failed to join path: %w", err)
	}

	// Create HTTP client with OAuth if available
	var httpClient HTTPClient
	if hasOAuth {
		oauthConfig := &oauth2.Config{
			ClientID:     appConfig.Jira.OAuthClientID,
			ClientSecret: appConfig.Jira.OAuthClientSecret,
			Endpoint: oauth2.Endpoint{
				TokenURL: tokenURL,
			},
		}

		token := &oauth2.Token{
			AccessToken:  appConfig.Jira.OAuthAccessToken,
			RefreshToken: appConfig.Jira.OAuthRefreshToken,
			TokenType:    "Bearer",
		}

		httpClient = oauthConfig.Client(context.Background(), token)
	} else {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &Client{
		baseURL:    baseURL,
		secrets:    appConfig.Jira,
		httpClient: httpClient,
		logger:     opts.logger,
	}, nil
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
				Key: c.secrets.ProjectKey,
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
	url := fmt.Sprintf("%s/rest/api/2/issue", c.baseURL)
	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		c.logger.Error().Err(err).Str("url", url).Msg("Failed to create HTTP request")
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	// Set authentication based on available credentials
	if c.secrets.Username != "" && c.secrets.Token != "" {
		// Fallback to basic auth
		httpReq.SetBasicAuth(c.secrets.Username, c.secrets.Token)
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
	url := fmt.Sprintf("%s/rest/api/2/issue/%s?fields=status", c.baseURL, ticketKey)

	// Create HTTP request
	httpReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to create HTTP request for getting ticket status")
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Accept", "application/json")

	// Set authentication
	if c.secrets.Username != "" && c.secrets.Token != "" {
		httpReq.SetBasicAuth(c.secrets.Username, c.secrets.Token)
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
	url := fmt.Sprintf("%s/rest/api/2/issue/%s/comment", c.baseURL, ticketKey)

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
	if c.secrets.Username != "" && c.secrets.Token != "" {
		httpReq.SetBasicAuth(c.secrets.Username, c.secrets.Token)
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
	return c.secrets.OAuthAccessToken != ""
}

// AuthType returns the type of authentication being used
func (c *Client) AuthType() string {
	if c.IsOAuthEnabled() {
		return "OAuth"
	} else if c.secrets.Username != "" && c.secrets.Token != "" {
		return "Basic Auth"
	}
	return "None"
}
