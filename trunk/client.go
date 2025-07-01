// Package trunk provides utilities for the Trunk.io API.
package trunk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/smartcontractkit/branch-out/jira"
)

// TrunkConfig holds the configuration for Trunk.io API client
type TrunkConfig struct {
	BaseURL  string
	APIToken string
}

// TrunkClient interface for interacting with Trunk.io API (for testability)
type TrunkClient interface {
	LinkTicketToTestCase(testCaseID string, ticket *jira.JiraTicketResponse, repoURL string) error
}

// HTTPClient interface for HTTP requests (for testability)
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client implements TrunkClient interface
type Client struct {
	config     TrunkConfig
	httpClient HTTPClient
	logger     zerolog.Logger
}

// NewClient creates a new Trunk.io client with configuration from environment variables
func NewClient(logger zerolog.Logger) (*Client, error) {
	apiToken := os.Getenv("TRUNK_API_TOKEN")
	if apiToken == "" {
		return nil, fmt.Errorf("TRUNK_API_TOKEN environment variable is required")
	}

	config := TrunkConfig{
		BaseURL:  "https://api.trunk.io",
		APIToken: apiToken,
	}

	return &Client{
		config:     config,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		logger:     logger,
	}, nil
}

// NewClientWithHTTPClient creates a new Trunk.io client with a custom HTTP client (for testing)
func NewClientWithHTTPClient(config TrunkConfig, httpClient HTTPClient, logger zerolog.Logger) *Client {
	return &Client{
		config:     config,
		httpClient: httpClient,
		logger:     logger,
	}
}

// LinkTicketToTestCase links a Jira ticket to a test case in Trunk.io
// See: https://docs.trunk.io/references/apis/flaky-tests#post-flaky-tests-link-ticket-to-test-case
func (c *Client) LinkTicketToTestCase(testCaseID string, ticket *jira.JiraTicketResponse, repoURL string) error {
	c.logger.Info().
		Str("test_case_id", testCaseID).
		Str("jira_ticket_key", ticket.Key).
		Msg("Linking Jira ticket to Trunk test case")

	// Extract repo information from the repository URL
	owner, repoName := extractRepoInfoFromURL(repoURL)

	// Create the request payload
	linkRequest := TrunkLinkTicketRequest{
		TestCaseID:       testCaseID,
		ExternalTicketID: ticket.Key, // Use Jira ticket key (e.g., "KAN-123")
		Repo: TrunkRepoReference{
			Host:  "github.com", // Default to GitHub for now
			Owner: owner,
			Name:  repoName,
		},
	}

	// Marshal the request
	requestBody, err := json.Marshal(linkRequest)
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to marshal Trunk link ticket request")
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/v1/flaky-tests/link-ticket-to-test-case", c.config.BaseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to create HTTP request for Trunk API")
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("x-api-token", c.config.APIToken)

	// Make the HTTP request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to make request to Trunk API")
		return fmt.Errorf("failed to make request to Trunk API: %w", err)
	}
	defer resp.Body.Close()

	// Read response body for error details
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to read response body")
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.logger.Error().
			Int("status_code", resp.StatusCode).
			Str("status", resp.Status).
			Str("response_body", string(body)).
			Msg("Trunk API returned error status")
		return fmt.Errorf("Trunk API error (status %d): %s", resp.StatusCode, string(body))
	}

	c.logger.Info().
		Int("status_code", resp.StatusCode).
		Str("test_case_id", testCaseID).
		Str("jira_ticket_key", ticket.Key).
		Msg("Successfully linked Jira ticket to Trunk test case")

	return nil
}
