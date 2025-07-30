// Package trunk provides utilities for the Trunk.io API.
package trunk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/rs/zerolog"

	"github.com/smartcontractkit/branch-out/base"
	"github.com/smartcontractkit/branch-out/config"
	"github.com/smartcontractkit/branch-out/github"
	"github.com/smartcontractkit/branch-out/telemetry"
)

// Operation name constants for consistent logging and metrics
const (
	opLinkTicket       = "link_ticket"
	opQuarantinedTests = "quarantined_tests"
)

// TrunkAPIError provides detailed context about API failures for better caller logging
type TrunkAPIError struct {
	Operation   string // The operation being performed (e.g., "link_ticket", "quarantined_tests")
	TestCaseID  string // The test case ID being operated on (if applicable)
	StatusCode  int    // HTTP status code from the API response
	APIResponse string // Raw API response body for debugging (truncated if large)
	Underlying  error  // The underlying error that caused the failure
}

func (e *TrunkAPIError) Error() string {
	if e.TestCaseID != "" {
		return fmt.Sprintf("trunk %s operation failed for test case %s (status %d): %v",
			e.Operation, e.TestCaseID, e.StatusCode, e.Underlying)
	}
	return fmt.Sprintf("trunk %s operation failed (status %d): %v",
		e.Operation, e.StatusCode, e.Underlying)
}

func (e *TrunkAPIError) Unwrap() error {
	return e.Underlying
}

// Doer interface for HTTP client to enable easier testing
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client is the standard Trunk.io Client.
type Client struct {
	BaseURL    *url.URL
	HTTPClient Doer

	secrets config.Trunk
	logger  zerolog.Logger
	metrics *telemetry.Metrics
}

// ClientOption is a function that sets a configuration option for the Trunk.io client.
type ClientOption func(*trunkClientOptions)

type trunkClientOptions struct {
	baseURL *url.URL
	secrets config.Trunk
	logger  zerolog.Logger
	metrics *telemetry.Metrics
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

// WithMetrics sets the metrics instance for the Trunk client.
func WithMetrics(metrics *telemetry.Metrics) ClientOption {
	return func(opts *trunkClientOptions) {
		opts.metrics = metrics
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
		metrics: opts.metrics,
	}, nil
}

// truncateAPIResponse truncates large API responses to avoid excessive logging
func truncateAPIResponse(response string, maxLen int) string {
	if len(response) <= maxLen {
		return response
	}
	return response[:maxLen] + "... (truncated)"
}

// doTrunkPost performs a POST request to the Trunk API with consistent error handling and metrics
func (c *Client) doTrunkPost(ctx context.Context, path string, payload any, opName string, testCaseID string) ([]byte, error) {
	start := time.Now()
	defer c.metrics.RecordTrunkAPILatency(ctx, opName, time.Since(start))

	// Marshal the request payload
	requestBody, err := json.Marshal(payload)
	if err != nil {
		return nil, &TrunkAPIError{
			Operation:  opName,
			TestCaseID: testCaseID,
			StatusCode: 0,
			Underlying: fmt.Errorf("failed to marshal request: %w", err),
		}
	}

	// Create HTTP request
	url := c.BaseURL.JoinPath(path)
	req, err := http.NewRequestWithContext(ctx, "POST", url.String(), bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, &TrunkAPIError{
			Operation:  opName,
			TestCaseID: testCaseID,
			StatusCode: 0,
			Underlying: fmt.Errorf("failed to create request: %w", err),
		}
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")

	// Make the HTTP request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, &TrunkAPIError{
			Operation:  opName,
			TestCaseID: testCaseID,
			StatusCode: 0,
			Underlying: fmt.Errorf("HTTP request failed: %w", err),
		}
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			c.logger.Debug().Err(closeErr).Msg("Failed to close response body")
		}
	}()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &TrunkAPIError{
			Operation:  opName,
			TestCaseID: testCaseID,
			StatusCode: resp.StatusCode,
			Underlying: fmt.Errorf("failed to read response body: %w", err),
		}
	}

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &TrunkAPIError{
			Operation:   opName,
			TestCaseID:  testCaseID,
			StatusCode:  resp.StatusCode,
			APIResponse: truncateAPIResponse(string(body), 1024),
			Underlying:  fmt.Errorf("non-2xx status code: %d", resp.StatusCode),
		}
	}

	return body, nil
}

// LinkTicketToTestCase links a Jira ticket to a test case in Trunk.io
// See: https://docs.trunk.io/references/apis/flaky-tests#post-flaky-tests-link-ticket-to-test-case
func (c *Client) LinkTicketToTestCase(testCaseID, issueKey string, repoURL string) error {
	ctx := context.Background()

	l := c.logger.With().
		Str("test_case_id", testCaseID).
		Str("jira_ticket_key", issueKey).
		Str("operation", opLinkTicket).
		Logger()
	l.Debug().Msg("Linking Jira ticket to Trunk test case")

	host, owner, repo, err := github.ParseRepoURL(repoURL)
	if err != nil {
		return &TrunkAPIError{
			Operation:  opLinkTicket,
			TestCaseID: testCaseID,
			StatusCode: 0,
			Underlying: fmt.Errorf("failed to parse repository URL: %w", err),
		}
	}

	// Create the request payload
	linkRequest := LinkTicketRequest{
		TestCaseID:       testCaseID,
		ExternalTicketID: issueKey, // Use Jira ticket key (e.g., "KAN-123")
		Repo: RepoReference{
			Host:  host,
			Owner: owner,
			Name:  repo,
		},
	}

	// Make the API call using the helper
	_, err = c.doTrunkPost(ctx, "v1/flaky-tests/link-ticket-to-test-case", linkRequest, opLinkTicket, testCaseID)
	if err != nil {
		l.Error().Err(err).Msg("Failed to link Jira ticket to Trunk test case")
		return err
	}

	l.Debug().Msg("Successfully linked Jira ticket to Trunk test case")
	return nil
}

// QuarantinedTests returns the list of tests that are currently quarantined by Trunk.io.
// See: https://docs.trunk.io/references/apis/flaky-tests#post-flaky-tests-list-quarantined-tests
func (c *Client) QuarantinedTests(repoURL string, orgURLSlug string) ([]TestCase, error) {
	host, owner, repo, err := github.ParseRepoURL(repoURL)
	if err != nil {
		return nil, &TrunkAPIError{
			Operation:  opQuarantinedTests,
			StatusCode: 0,
			Underlying: fmt.Errorf("failed to parse repository URL: %w", err),
		}
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
		Str("operation", opQuarantinedTests).
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
			l.Error().Err(err).Msg("Failed to get quarantined tests")
			return nil, err
		}

		tests = append(tests, response.QuarantinedTests...)

		if response.PageResponse.NextPageToken == "" {
			break
		}

		request.PageQuery.PageToken = response.PageResponse.NextPageToken
	}

	l.Debug().
		Int("total_tests", len(tests)).
		Dur("duration", time.Since(startTime)).
		Msg("Fetched all quarantined tests")

	return tests, nil
}

// getQuarantinedTests makes a single request to the Trunk.io API to get the quarantined tests.
func (c *Client) getQuarantinedTests(request *QuarantinedTestsRequest) (*QuarantinedTestsResponse, error) {
	ctx := context.Background()

	// Make the API call using the helper
	body, err := c.doTrunkPost(ctx, "v1/flaky-tests/quarantined", request, opQuarantinedTests, "")
	if err != nil {
		return nil, err
	}

	// Unmarshal the response
	var response QuarantinedTestsResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, &TrunkAPIError{
			Operation:   opQuarantinedTests,
			StatusCode:  0,
			APIResponse: truncateAPIResponse(string(body), 1024),
			Underlying:  fmt.Errorf("failed to unmarshal response: %w", err),
		}
	}

	return &response, nil
}
