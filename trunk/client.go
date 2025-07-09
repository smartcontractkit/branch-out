// Package trunk provides utilities for the Trunk.io API.
package trunk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/rs/zerolog"

	"github.com/smartcontractkit/branch-out/base"
	"github.com/smartcontractkit/branch-out/config"
	"github.com/smartcontractkit/branch-out/jira"
)

// Client is the Trunk.io client.
type Client struct {
	BaseURL    *url.URL
	HTTPClient *http.Client

	secrets config.Trunk
	logger  zerolog.Logger
}

// IClient is the interface that wraps the basic Trunk.io client methods.
// Helpful for mocking in tests.
type IClient interface {
	LinkTicketToTestCase(testCaseID string, ticket *jira.TicketResponse, repoURL string) error
}

// Option is a function that sets a configuration option for the Trunk.io client.
type Option func(*trunkClientOptions)

type trunkClientOptions struct {
	baseURL *url.URL
	secrets config.Trunk
	logger  zerolog.Logger
}

// WithLogger sets the logger to use for the Trunk.io client.
func WithLogger(logger zerolog.Logger) Option {
	return func(opts *trunkClientOptions) {
		opts.logger = logger
	}
}

// WithConfig sets the config to use for the Trunk.io client.
func WithConfig(config config.Config) Option {
	return func(opts *trunkClientOptions) {
		opts.secrets = config.Trunk
	}
}

// WithBaseURL sets the base URL for the Trunk.io client. Useful for testing.
func WithBaseURL(baseURL *url.URL) Option {
	return func(opts *trunkClientOptions) {
		opts.baseURL = baseURL
	}
}

// NewClient creates a new Trunk.io client.
func NewClient(options ...Option) (*Client, error) {
	opts := &trunkClientOptions{
		baseURL: &url.URL{Scheme: "https", Host: "api.trunk.io"},
		logger:  zerolog.Nop(),
	}
	for _, opt := range options {
		opt(opts)
	}

	return &Client{
		BaseURL: opts.baseURL,
		HTTPClient: base.NewClient(
			base.WithLogger(opts.logger),
			base.WithComponent("trunk"),
		),
		secrets: opts.secrets,
		logger:  opts.logger,
	}, nil
}

// LinkTicketToTestCase links a Jira ticket to a test case in Trunk.io
// See: https://docs.trunk.io/references/apis/flaky-tests#post-flaky-tests-link-ticket-to-test-case
func (c *Client) LinkTicketToTestCase(testCaseID string, ticket *jira.TicketResponse, repoURL string) error {
	c.logger.Debug().
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
	url := c.BaseURL.JoinPath("v1/flaky-tests/link-ticket-to-test-case")
	req, err := http.NewRequest("POST", url.String(), bytes.NewBuffer(requestBody))
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to create HTTP request for Trunk API")
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("x-api-token", c.secrets.Token)

	// Make the HTTP request
	resp, err := c.HTTPClient.Do(req)
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
