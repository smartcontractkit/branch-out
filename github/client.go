package github

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit"
	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit/github_primary_ratelimit"
	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit/github_secondary_ratelimit"
	"github.com/google/go-github/v73/github"
	"github.com/rs/zerolog"
	gh_graphql "github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

const (
	// TokenEnvVar is the environment variable that contains the GitHub token.
	TokenEnvVar = "GITHUB_TOKEN"
	// RateLimitWarningThreshold is the number of remaining requests before a warning is logged.
	RateLimitWarningThreshold = 5
	// RateLimitWarningMsg is the message logged when the number of remaining requests is below the warning threshold.
	RateLimitWarningMsg = "GitHub API requests nearing rate limit"
	// RateLimitHitMsg is the message logged when the number of remaining requests is 0.
	RateLimitHitMsg = "GitHub API rate limit hit, sleeping until limit reset"
)

// Client is a wrapper around the GitHub REST and GraphQL API clients
type Client struct {
	// Rest is the GitHub REST API client.
	Rest *github.Client
	// GraphQL is the GitHub GraphQL API client.
	GraphQL *gh_graphql.Client
	// BaseURL is the base URL of the GitHub API. Defaults to the public GitHub API.
	BaseURL *url.URL
	// tokenSource is the GitHub tokenSource used to authenticate requests.
	tokenSource oauth2.TokenSource
	// next is the next HTTP round tripper.
	next http.RoundTripper
}

// ClientOption is a function that can be used to configure the GitHub client.
type ClientOption func(*Client)

// WithBaseURL sets the base URL of the GitHub API.
func WithBaseURL(baseURL *url.URL) ClientOption {
	return func(c *Client) {
		if baseURL != nil {
			c.BaseURL = baseURL
		}
	}
}

// WithToken sets the GitHub token used to authenticate requests.
func WithToken(token string) ClientOption {
	return func(c *Client) {
		if token != "" {
			c.tokenSource = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		}
	}
}

// WithNext sets the next HTTP round tripper. Helpful for testing.
func WithNext(next http.RoundTripper) ClientOption {
	return func(c *Client) {
		c.next = next
	}
}

// setupToken configures the authentication mechanism for the GitHub client.
// It tries authentication methods in the following order:
// 1. GitHub token from flag (if provided via WithToken)
// 2. GitHub App authentication (automatically uses installation tokens if available)
// 3. GitHub token from environment variable
func setupToken(client *Client, l zerolog.Logger) error {
	// Priority 1: Token from flag (already set via WithToken)
	if client.tokenSource != nil {
		l.Debug().Msg("Using GitHub token from flag")
		return nil
	}

	// Priority 2: GitHub App authentication
	appTokenSource, err := LoadInstallationTokenSource()
	if err != nil {
		return fmt.Errorf("failed to load GitHub App configuration: %w", err)
	}
	if appTokenSource != nil {
		client.tokenSource = appTokenSource
		l.Debug().Msg("Using GitHub App authentication")
		return nil
	}

	// Priority 3: Token from environment variable
	envToken := os.Getenv(TokenEnvVar)
	if envToken != "" {
		client.tokenSource = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: envToken})
		l.Debug().Msg("Using GitHub token from environment variable")
		return nil
	}

	// No authentication configured
	l.Warn().Msg("No GitHub authentication configured, some features will be disabled and rate limits might be hit!")
	return nil
}

// NewClient creates a new GitHub REST and GraphQL API client with the provided token and logger.
// If optionalNext is provided, it will be used as the base client for both REST and GraphQL, handy for testing.
func NewClient(
	l zerolog.Logger,
	opts ...ClientOption,
) (*Client, error) {
	client := &Client{}
	for _, opt := range opts {
		opt(client)
	}

	err := setupToken(client, l)
	if err != nil {
		return nil, fmt.Errorf("failed to setup authentication: %w", err)
	}

	onPrimaryRateLimitHit := func(ctx *github_primary_ratelimit.CallbackContext) {
		l := l.Warn().Str("limit", "primary")
		if ctx.Request != nil {
			l = l.Str("request_url", ctx.Request.URL.String())
		}
		if ctx.Response != nil {
			l = l.Int("status", ctx.Response.StatusCode)
		}
		if ctx.Category != "" {
			l = l.Str("category", string(ctx.Category))
		}
		if ctx.ResetTime != nil {
			l = l.Str("reset_time", ctx.ResetTime.String())
		}
		l.Msg(RateLimitHitMsg)
	}

	onSecondaryRateLimitHit := func(ctx *github_secondary_ratelimit.CallbackContext) {
		l := l.Warn().Str("limit", "secondary")
		if ctx.Request != nil {
			l = l.Str("request_url", ctx.Request.URL.String())
		}
		if ctx.Response != nil {
			l = l.Int("status", ctx.Response.StatusCode)
		}
		if ctx.ResetTime != nil {
			l = l.Str("reset_time", ctx.ResetTime.String())
		}
		if ctx.TotalSleepTime != nil {
			l = l.Str("total_sleep_time", ctx.TotalSleepTime.String())
		}
		l.Msg(RateLimitHitMsg)
	}

	var baseTransport = client.next
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}

	// Build the transport chain: OAuth2 (if needed) -> Logging -> Base
	var restTransport = clientRoundTripper("REST", l, baseTransport)
	if client.tokenSource != nil {
		restTransport = &oauth2.Transport{
			Source: client.tokenSource,
			Base:   restTransport,
		}
	}

	// Create rate limiter with the transport chain
	rateLimiter := github_ratelimit.NewClient(
		restTransport,
		github_primary_ratelimit.WithLimitDetectedCallback(onPrimaryRateLimitHit),
		github_secondary_ratelimit.WithLimitDetectedCallback(onSecondaryRateLimitHit),
	)

	client.Rest = github.NewClient(rateLimiter)

	l = l.With().Str("base_url", client.Rest.BaseURL.String()).Logger()

	// Setup GraphQL client with the same transport pattern
	var graphqlTransport = clientRoundTripper("GraphQL", l, nil)
	if client.tokenSource != nil {
		graphqlTransport = &oauth2.Transport{
			Source: client.tokenSource,
			Base:   graphqlTransport,
		}
	}

	graphqlClient := &http.Client{Transport: graphqlTransport}

	if client.BaseURL != nil {
		client.GraphQL = gh_graphql.NewEnterpriseClient(client.BaseURL.String(), graphqlClient)
	} else {
		client.GraphQL = gh_graphql.NewClient(graphqlClient)
	}

	return client, nil
}

// clientRoundTripper returns a RoundTripper that logs requests and responses to the GitHub API.
// You can pass a custom RoundTripper to use a different transport, or nil to use the default transport.
func clientRoundTripper(clientType string, l zerolog.Logger, next http.RoundTripper) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}

	return &loggingTransport{
		transport:  next,
		logger:     l,
		clientType: clientType,
	}
}

// loggingTransport is a RoundTripper that logs requests and responses to the GitHub API.
type loggingTransport struct {
	transport  http.RoundTripper
	logger     zerolog.Logger
	clientType string
}

// RoundTrip logs the request and response details.
func (lt *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var (
		reqBody []byte
		err     error
		start   = time.Now()
	)

	l := lt.logger.With().
		Str("client_type", lt.clientType).
		Str("method", req.Method).
		Str("request_url", req.URL.String()).
		Str("user_agent", req.Header.Get("User-Agent")).
		Logger()

	if req.Body != nil {
		reqBody, err = io.ReadAll(req.Body)
		if err != nil {
			l.Error().Err(err).Msg("Failed to read request body")
		}
	}
	if reqBody != nil {
		req.Body = io.NopCloser(bytes.NewBuffer(reqBody))
	}
	l.Trace().Bytes("request_body", reqBody).Msg("GitHub API request started")

	resp, err := lt.transport.RoundTrip(req)
	if err != nil {
		l.Error().Err(err).Msg("GitHub API request failed")
		// Probably a rate limit error, let the rate limit library handle it
		return resp, err
	}

	duration := time.Since(start)

	var respBody []byte
	if resp.Body != nil {
		respBody, err = io.ReadAll(resp.Body)
		if err != nil {
			l.Error().Err(err).Msg("Failed to read response body")
		}
	}
	if respBody != nil {
		resp.Body = io.NopCloser(bytes.NewBuffer(respBody))
	}
	l = l.With().
		Int("status", resp.StatusCode).
		Str("duration", duration.String()).
		Logger()

	// Process rate limit headers
	callsRemainingStr := resp.Header.Get("X-RateLimit-Remaining")
	if callsRemainingStr == "" {
		callsRemainingStr = "0"
	}
	callLimitStr := resp.Header.Get("X-RateLimit-Limit")
	if callLimitStr == "" {
		callLimitStr = "0"
	}
	callsUsedStr := resp.Header.Get("X-RateLimit-Used")
	if callsUsedStr == "" {
		callsUsedStr = "0"
	}
	limitResetStr := resp.Header.Get("X-RateLimit-Reset")
	if limitResetStr == "" {
		limitResetStr = "0"
	}
	callsRemaining, err := strconv.Atoi(callsRemainingStr)
	if err != nil {
		return nil, fmt.Errorf("failed to convert callsRemaining header to int: %w", err)
	}
	callLimit, err := strconv.Atoi(callLimitStr)
	if err != nil {
		return nil, fmt.Errorf("failed to convert callLimit header to int: %w", err)
	}
	callsUsed, err := strconv.Atoi(callsUsedStr)
	if err != nil {
		return nil, fmt.Errorf("failed to convert callsUsed header to int: %w", err)
	}
	limitReset, err := strconv.Atoi(limitResetStr)
	if err != nil {
		return nil, fmt.Errorf("failed to convert limitReset header to int: %w", err)
	}
	limitResetTime := time.Unix(int64(limitReset), 0)

	l = l.With().
		Int("calls_remaining", callsRemaining).
		Int("call_limit", callLimit).
		Int("calls_used", callsUsed).
		Time("limit_reset", limitResetTime).
		Logger()

	if resp.Request != nil {
		l = l.With().Str("response_request_url", resp.Request.URL.String()).Logger()
	}

	if callsRemaining <= RateLimitWarningThreshold {
		l.Warn().Msg(RateLimitWarningMsg)
	}

	l.Trace().Bytes("response_body", respBody).Msg("GitHub API request completed")
	return resp, nil
}
