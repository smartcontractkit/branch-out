// Package trunk provides utilities for the Trunk.io API.
package trunk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	go_jira "github.com/andygrunwald/go-jira"
	"github.com/rs/zerolog"

	"github.com/smartcontractkit/branch-out/base"
	"github.com/smartcontractkit/branch-out/config"
	"github.com/smartcontractkit/branch-out/github"
)

// Client is the standard Trunk.io Client.
type Client struct {
	BaseURL    *url.URL
	HTTPClient *http.Client

	secrets config.Trunk
	logger  zerolog.Logger
}

// ClientOption is a function that sets a configuration option for the Trunk.io client.
type ClientOption func(*trunkClientOptions)

type trunkClientOptions struct {
	baseURL *url.URL
	secrets config.Trunk
	logger  zerolog.Logger
}

// WithLogger sets the logger to use for the Trunk.io client.
func WithLogger(logger zerolog.Logger) ClientOption {
	return func(opts *trunkClientOptions) {
		opts.logger = logger
	}
}

// WithConfig sets the config to use for the Trunk.io client.
func WithConfig(config config.Config) ClientOption {
	return func(opts *trunkClientOptions) {
		opts.secrets = config.Trunk
	}
}

// WithBaseURL sets the base URL for the Trunk.io client. Useful for testing.
func WithBaseURL(baseURL *url.URL) ClientOption {
	return func(opts *trunkClientOptions) {
		opts.baseURL = baseURL
	}
}

// NewClient creates a new Trunk.io client.
func NewClient(options ...ClientOption) (*Client, error) {
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
			"trunk",
			base.WithLogger(opts.logger),
			base.WithRequestHeaders(http.Header{
				"x-api-token": []string{opts.secrets.Token},
			}),
		),
		secrets: opts.secrets,
		logger:  opts.logger,
	}, nil
}

// LinkTicketToTestCase links a Jira ticket to a test case in Trunk.io
// See: https://docs.trunk.io/references/apis/flaky-tests#post-flaky-tests-link-ticket-to-test-case
func (c *Client) LinkTicketToTestCase(testCaseID string, ticket *go_jira.Issue, repoURL string) error {
	c.logger.Debug().
		Str("test_case_id", testCaseID).
		Str("jira_ticket_key", ticket.Key).
		Msg("Linking Jira ticket to Trunk test case")

	host, owner, repo, err := github.ParseRepoURL(repoURL)
	if err != nil {
		return fmt.Errorf("failed to parse repository URL: %w", err)
	}

	// Create the request payload
	linkRequest := LinkTicketRequest{
		TestCaseID:       testCaseID,
		ExternalTicketID: ticket.Key, // Use Jira ticket key (e.g., "KAN-123")
		Repo: RepoReference{
			Host:  host,
			Owner: owner,
			Name:  repo,
		},
	}

	// Marshal the request
	requestBody, err := json.Marshal(linkRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal link ticket request: %w", err)
	}

	// Create HTTP request
	url := c.BaseURL.JoinPath("v1/flaky-tests/link-ticket-to-test-case")
	req, err := http.NewRequest("POST", url.String(), bytes.NewBuffer(requestBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")

	// Make the HTTP request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
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

// QuarantinedTests returns the list of tests that are currently quarantined by Trunk.io.
// See: https://docs.trunk.io/references/apis/flaky-tests#post-flaky-tests-list-quarantined-tests
func (c *Client) QuarantinedTests(repoURL string, orgURLSlug string) ([]TestCase, error) {
	host, owner, repo, err := github.ParseRepoURL(repoURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse repository URL: %w", err)
	}

	if orgURLSlug == "" {
		orgURLSlug = owner // Best guess
	}

	l := c.logger.With().
		Str("repo_url", repoURL).
		Str("org_url_slug", orgURLSlug).
		Str("host", host).
		Str("owner", owner).
		Str("repo", repo).
		Logger()
	l.Debug().Msg("Fetching quarantined tests")
	startTime := time.Now()

	tests := []TestCase{}

	request := &QuarantinedTestsRequest{
		Repo: RepoReference{
			Host:  host,
			Owner: owner,
			Name:  repo,
		},
		OrgURLSlug: orgURLSlug,
		PageQuery: PageQuery{
			PageSize: 100,
		},
	}

	for {
		response, err := c.getQuarantinedTests(request)
		if err != nil {
			return nil, fmt.Errorf("failed to get quarantined tests: %w", err)
		}

		tests = append(tests, response.QuarantinedTests...)

		if response.PageResponse.NextPageToken == "" {
			break
		}

		request.PageQuery.PageToken = response.PageResponse.NextPageToken
	}

	l.Info().
		Int("total_tests", len(tests)).
		Str("duration", time.Since(startTime).String()).
		Msg("Fetched all quarantined tests")

	return tests, nil
}

// getQuarantinedTests makes a single request to the Trunk.io API to get the quarantined tests.
func (c *Client) getQuarantinedTests(request *QuarantinedTestsRequest) (*QuarantinedTestsResponse, error) {
	url := c.BaseURL.JoinPath("v1/flaky-tests/quarantined")

	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	req, err := http.NewRequest("POST", url.String(), bytes.NewBuffer(requestBody))
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to create HTTP request for Trunk API")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request to Trunk API: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			c.logger.Error().Err(closeErr).Msg("Failed to close response body")
		}
	}()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var response QuarantinedTestsResponse
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &response, nil
}
