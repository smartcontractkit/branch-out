package github

import (
	"context"
	"fmt"
	"net/url"

	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit"
	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit/github_primary_ratelimit"
	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit/github_secondary_ratelimit"
	"github.com/google/go-github/v73/github"
	"github.com/rs/zerolog"
	gh_graphql "github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"

	"github.com/smartcontractkit/branch-out/base"
	"github.com/smartcontractkit/branch-out/config"
	"github.com/smartcontractkit/branch-out/golang"
)

// IClient is the interface for the GitHub client.
type IClient interface {
	QuarantineTests(
		ctx context.Context,
		l zerolog.Logger,
		repoURL string,
		targets []golang.QuarantineTarget,
		options ...QuarantineOption,
	) error
}

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
}

// ClientOption is a function that can be used to configure the GitHub client.
type ClientOption func(*clientOptions)

type clientOptions struct {
	secrets config.GitHub
	logger  zerolog.Logger
}

// WithConfig uses a GitHub config to setup authentication.
func WithConfig(config config.Config) ClientOption {
	return func(c *clientOptions) {
		c.secrets = config.GitHub
	}
}

// WithLogger sets the logger for the GitHub client.
func WithLogger(logger zerolog.Logger) ClientOption {
	return func(c *clientOptions) {
		c.logger = logger
	}
}

// NewClient creates a new GitHub REST and GraphQL API client with the provided token and logger.
// If optionalNext is provided, it will be used as the base client for both REST and GraphQL, handy for testing.
func NewClient(
	options ...ClientOption,
) (*Client, error) {
	opts := &clientOptions{}
	for _, opt := range options {
		opt(opts)
	}

	client := &Client{}

	var err error
	client.tokenSource, err = setupAuth(opts.secrets)
	if err != nil {
		return nil, fmt.Errorf("failed to setup authentication: %w", err)
	}

	onPrimaryRateLimitHit := func(ctx *github_primary_ratelimit.CallbackContext) {
		l := opts.logger.Warn().Str("limit", "primary")
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
		l.Msg(base.RateLimitHitMsg)
	}

	onSecondaryRateLimitHit := func(ctx *github_secondary_ratelimit.CallbackContext) {
		l := opts.logger.Warn().Str("limit", "secondary")
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
		l.Msg(base.RateLimitHitMsg)
	}

	// Create base HTTP client with logging transport
	baseTransport := base.NewClient("github-rest", base.WithLogger(opts.logger))

	// Add OAuth2 transport if token source is available
	if client.tokenSource != nil {
		baseTransport.Transport = &oauth2.Transport{
			Source: client.tokenSource,
			Base:   baseTransport.Transport,
		}
	}

	// Create rate limiter with the transport chain
	rateLimiter := github_ratelimit.NewClient(
		baseTransport.Transport,
		github_primary_ratelimit.WithLimitDetectedCallback(onPrimaryRateLimitHit),
		github_secondary_ratelimit.WithLimitDetectedCallback(onSecondaryRateLimitHit),
	)

	client.Rest = github.NewClient(rateLimiter)

	opts.logger = opts.logger.With().Str("base_url", client.Rest.BaseURL.String()).Logger()

	// Setup GraphQL client with the same transport pattern
	graphQLClient := base.NewClient("github-graphql", base.WithLogger(opts.logger))

	if client.tokenSource != nil {
		graphQLClient.Transport = &oauth2.Transport{
			Source: client.tokenSource,
			Base:   graphQLClient.Transport,
		}
	}

	if client.BaseURL != nil {
		client.GraphQL = gh_graphql.NewEnterpriseClient(client.BaseURL.String(), graphQLClient)
	} else {
		client.GraphQL = gh_graphql.NewClient(graphQLClient)
	}

	return client, nil
}
