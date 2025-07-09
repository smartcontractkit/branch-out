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

	"github.com/rs/zerolog"
	"golang.org/x/oauth2"

	"github.com/smartcontractkit/branch-out/base"
	"github.com/smartcontractkit/branch-out/config"
)

// Client implements Interface
type Client struct {
	BaseURL    *url.URL
	HTTPClient *http.Client

	secrets config.Jira
	logger  zerolog.Logger
}

// Option is a function that sets a configuration option for the Jira client.
type Option func(*jiraClientOptions)

type jiraClientOptions struct {
	baseURL *url.URL
	config  *config.Config
	logger  zerolog.Logger
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

// WithBaseURL sets the base URL for the Jira client.
func WithBaseURL(baseURL *url.URL) Option {
	return func(cfg *jiraClientOptions) {
		cfg.baseURL = baseURL
	}
}

// IClient is the interface that wraps the basic Jira client methods.
// Helpful for mocking in tests.
type IClient interface {
	CreateFlakyTestTicket(req FlakyTestTicketRequest) (*TicketResponse, error)
	GetTicketStatus(ticketKey string) (*TicketStatus, error)
	AddCommentToTicket(ticketKey string, comment string) error
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

	baseURL := &url.URL{
		Scheme: "https",
		Host:   appConfig.Jira.BaseDomain,
	}
	tokenURL := baseURL.JoinPath("plugins/servlet/oauth/access-token")

	baseTransport := base.NewTransport(
		base.WithLogger(opts.logger),
		base.WithComponent("jira"),
	)

	var httpClient *http.Client
	if hasOAuth {
		oauthConfig := &oauth2.Config{
			ClientID:     appConfig.Jira.OAuthClientID,
			ClientSecret: appConfig.Jira.OAuthClientSecret,
			Endpoint: oauth2.Endpoint{
				TokenURL: tokenURL.String(),
			},
		}

		token := &oauth2.Token{
			AccessToken:  appConfig.Jira.OAuthAccessToken,
			RefreshToken: appConfig.Jira.OAuthRefreshToken,
			TokenType:    "Bearer",
		}

		// Use context to pass the base transport to the oauth2 client
		clientCtx := context.WithValue(context.Background(), oauth2.HTTPClient, baseTransport)
		client := oauthConfig.Client(clientCtx, token)
		httpClient = client
	}

	return &Client{
		BaseURL:    baseURL,
		HTTPClient: httpClient,
		secrets:    appConfig.Jira,
		logger:     opts.logger,
	}, nil
}

// CreateFlakyTestTicket creates a new Jira ticket for a flaky test
func (c *Client) CreateFlakyTestTicket(req FlakyTestTicketRequest) (*TicketResponse, error) {
	c.logger.Debug().
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
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := c.BaseURL.JoinPath("rest/api/2/issue")
	httpReq, err := http.NewRequest("POST", url.String(), bytes.NewBuffer(jsonData))
	if err != nil {
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
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
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
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check if request was successful
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("jira API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var jiraResp TicketResponse
	if err := json.Unmarshal(body, &jiraResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	c.logger.Info().
		Str("ticket_key", jiraResp.Key).
		Str("ticket_id", jiraResp.ID).
		Msg("Created Jira ticket for flaky test")

	return &jiraResp, nil
}

// GetTicketStatus retrieves the current status of a Jira ticket
func (c *Client) GetTicketStatus(ticketKey string) (*TicketStatus, error) {
	c.logger.Debug().Str("ticket_key", ticketKey).Msg("Getting Jira ticket status")
	// Create the API URL
	url := c.BaseURL.JoinPath("rest/api/2/issue", ticketKey, "?fields=status")

	// Create HTTP request
	httpReq, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Accept", "application/json")

	// Set authentication
	if c.secrets.Username != "" && c.secrets.Token != "" {
		httpReq.SetBasicAuth(c.secrets.Username, c.secrets.Token)
	} else {
		return nil, fmt.Errorf("no valid authentication credentials available")
	}

	// Make the request
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
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
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check if request was successful
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("jira API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var ticketStatus TicketStatus
	if err := json.Unmarshal(body, &ticketStatus); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	c.logger.Debug().
		Str("ticket_key", ticketKey).
		Str("status", ticketStatus.Fields.Status.Name).
		Str("status_category", ticketStatus.Fields.Status.StatusCategory.Key).
		Msg("Retrieved Jira ticket status")

	return &ticketStatus, nil
}

// AddCommentToTicket adds a comment to an existing Jira ticket
func (c *Client) AddCommentToTicket(ticketKey string, comment string) error {
	c.logger.Debug().Str("ticket_key", ticketKey).Msg("Adding comment to Jira ticket")

	// Create the API URL
	url := c.BaseURL.JoinPath("rest/api/2/issue", ticketKey, "comment")

	// Create request body
	requestBody := map[string]interface{}{
		"body": comment,
	}

	// Marshal request body
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequest("POST", url.String(), bytes.NewBuffer(jsonBody))
	if err != nil {
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
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
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
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Check if request was successful
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("jira API error (status %d): %s", resp.StatusCode, string(body))
	}

	c.logger.Debug().
		Str("ticket_key", ticketKey).
		Msg("Added comment to Jira ticket")

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
