package github

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit"
	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit/github_primary_ratelimit"
	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit/github_secondary_ratelimit"
	"github.com/google/go-github/v72/github"
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
	Rest    *github.Client
	GraphQL *gh_graphql.Client
	token   string
}

// NewClient creates a new GitHub REST and GraphQL API client with the provided token and logger.
// If optionalNext is provided, it will be used as the base client for both REST and GraphQL, handy for testing.
func NewClient(
	l zerolog.Logger,
	githubToken string,
	optionalNext http.RoundTripper,
) (*Client, error) {
	switch {
	case githubToken != "":
		l.Debug().Msg("Using GitHub token from flag")
	case os.Getenv(TokenEnvVar) != "":
		githubToken = os.Getenv(TokenEnvVar)
		l.Debug().Msg("Using GitHub token from environment variable")
	default:
		l.Warn().Msg("GitHub token not provided, some features will be disabled and rate limits might be hit!")
	}

	var (
		next   http.RoundTripper
		client = &Client{}
	)

	if optionalNext != nil {
		next = optionalNext
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

	rateLimiter := github_ratelimit.NewClient(
		clientRoundTripper("REST", l, next),
		github_primary_ratelimit.WithLimitDetectedCallback(onPrimaryRateLimitHit),
		github_secondary_ratelimit.WithLimitDetectedCallback(onSecondaryRateLimitHit),
	)

	client.Rest = github.NewClient(rateLimiter)
	if githubToken != "" {
		client.Rest = client.Rest.WithAuthToken(githubToken)
		client.token = githubToken
	}

	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubToken},
	)
	graphqlClient := oauth2.NewClient(context.Background(), src)
	graphqlClient.Transport = clientRoundTripper("GraphQL", l, graphqlClient.Transport)
	client.GraphQL = gh_graphql.NewClient(graphqlClient)

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
	start := time.Now()

	l := lt.logger.With().
		Str("client_type", lt.clientType).
		Str("method", req.Method).
		Str("request_url", req.URL.String()).
		Str("user_agent", req.Header.Get("User-Agent")).
		Logger()

	resp, err := lt.transport.RoundTrip(req)
	duration := time.Since(start)

	l = l.With().
		Int("status", resp.StatusCode).
		Str("duration", duration.String()).
		Logger()

	if err != nil || resp.StatusCode != http.StatusOK {
		// Probably a rate limit error, let the rate limit library handle it
		return resp, err
	}

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

	l.Trace().Msg("GitHub API request completed")
	return resp, nil
}
