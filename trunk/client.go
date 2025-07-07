// Package trunk provides utilities for the Trunk.io API.
package trunk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog"

	"github.com/smartcontractkit/branch-out/config"
	"github.com/smartcontractkit/branch-out/jira"
)

// Interface for interacting with Trunk.io API (for testability)
type Interface interface {
	LinkTicketToTestCase(testCaseID string, ticket *jira.TicketResponse, repoURL string) error
}

// HTTPClient interface for HTTP requests (for testability)
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client is the Trunk.io client.
type Client struct {
	baseURL    string
	secrets    config.Trunk
	httpClient HTTPClient
	logger     zerolog.Logger
}

// Option is a function that sets a configuration option for the Trunk.io client.
type Option func(*trunkClientOptions)

type trunkClientOptions struct {
	baseURL    string
	cfg        *config.Config
	logger     zerolog.Logger
	httpClient HTTPClient
}

// WithLogger sets the logger to use for the Trunk.io client.
func WithLogger(logger zerolog.Logger) Option {
	return func(opts *trunkClientOptions) {
		opts.logger = logger
	}
}

// WithConfig sets the config to use for the Trunk.io client.
func WithConfig(config *config.Config) Option {
	return func(opts *trunkClientOptions) {
		opts.cfg = config
	}
}

// WithHTTPClient sets the HTTP client to use for the Trunk.io client.
func WithHTTPClient(httpClient HTTPClient) Option {
	return func(opts *trunkClientOptions) {
		opts.httpClient = httpClient
	}
}

// WithBaseURL sets the base URL for the Trunk.io client. Useful for testing.
func WithBaseURL(baseURL string) Option {
	return func(opts *trunkClientOptions) {
		opts.baseURL = baseURL
	}
}

// NewClient creates a new Trunk.io client.
func NewClient(options ...Option) (*Client, error) {
	opts := &trunkClientOptions{
		baseURL:    "https://api.trunk.io",
		logger:     zerolog.Nop(),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range options {
		opt(opts)
	}

	var (
		appConfig = opts.cfg
		err       error
	)
	if appConfig == nil {
		appConfig, err = config.Load()
		if err != nil {
			return nil, err
		}
	}

	return &Client{
		baseURL:    opts.baseURL,
		secrets:    appConfig.Trunk,
		httpClient: opts.httpClient,
		logger:     opts.logger,
	}, nil
}

// LinkTicketToTestCase links a Jira ticket to a test case in Trunk.io
// See: https://docs.trunk.io/references/apis/flaky-tests#post-flaky-tests-link-ticket-to-test-case
func (c *Client) LinkTicketToTestCase(testCaseID string, ticket *jira.TicketResponse, repoURL string) error {
	c.logger.Info().
		Str("test_case_id", testCaseID).
		Str("jira_ticket_key", ticket.Key).
		Msg("Linking Jira ticket to Trunk test case")

	// Extract repo information from the repository URL
	owner, repoName := extractRepoInfoFromURL(repoURL)

	// Create the request payload
	linkRequest := LinkTicketRequest{
		TestCaseID:       testCaseID,
		ExternalTicketID: ticket.Key, // Use Jira ticket key (e.g., "KAN-123")
		Repo: RepoReference{
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
	url := fmt.Sprintf("%s/v1/flaky-tests/link-ticket-to-test-case", c.baseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to create HTTP request for Trunk API")
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("x-api-token", c.secrets.Token)

	// Make the HTTP request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to make request to Trunk API")
		return fmt.Errorf("failed to make request to Trunk API: %w", err)
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

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.logger.Error().
			Int("status_code", resp.StatusCode).
			Str("status", resp.Status).
			Str("response_body", string(body)).
			Msg("Trunk API returned error status")
		return fmt.Errorf("trunk API error (status %d): %s", resp.StatusCode, string(body))
	}

	c.logger.Info().
		Int("status_code", resp.StatusCode).
		Str("test_case_id", testCaseID).
		Str("jira_ticket_key", ticket.Key).
		Msg("Successfully linked Jira ticket to Trunk test case")

	return nil
}
