package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit"
	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit/github_primary_ratelimit"
	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit/github_secondary_ratelimit"
	"github.com/google/go-github/v74/github"
	"github.com/rs/zerolog"
	gh_graphql "github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"

	"github.com/smartcontractkit/branch-out/base"
	"github.com/smartcontractkit/branch-out/config"
	"github.com/smartcontractkit/branch-out/golang"
	"github.com/smartcontractkit/branch-out/telemetry"
)

// Client is the standard GitHub Client.
type Client struct {
	// Rest is the GitHub REST API client.
	Rest *github.Client
	// GraphQL is the GitHub GraphQL API client.
	GraphQL *gh_graphql.Client
	// BaseURL is the base URL of the GitHub API. Defaults to the public GitHub API.
	BaseURL *url.URL
	// tokenSource is the GitHub tokenSource used to authenticate requests.
	tokenSource oauth2.TokenSource
	// metrics is the telemetry metrics instance
	metrics *telemetry.Metrics
}

// ClientOption is a function that can be used to configure the GitHub client.
type ClientOption func(*clientOptions)

type clientOptions struct {
	secrets config.GitHub
	logger  zerolog.Logger
	metrics *telemetry.Metrics
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

// WithMetrics sets the metrics instance for the GitHub client.
func WithMetrics(metrics *telemetry.Metrics) ClientOption {
	return func(c *clientOptions) {
		c.metrics = metrics
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

	client := &Client{
		metrics: opts.metrics,
	}

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

		// Record rate limit hit metrics
		if ctx.Request != nil {
			client.metrics.IncGitHubRateLimitHit(ctx.Request.Context())
		}
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

		// Record rate limit hit metrics
		if ctx.Request != nil {
			client.metrics.IncGitHubRateLimitHit(ctx.Request.Context())
		}
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

// Flaky test operations

// getOrCreateRemoteBranch returns the HEAD SHA of a branch, creating it if it doesn't exist.
// Handles concurrent webhook operations that may try to create the same daily branch.
func (c *Client) getOrCreateRemoteBranch(ctx context.Context, owner, repo, branchName string) (string, error) {
	branchHeadSHA, branchExists, err := c.getBranchHeadSHA(ctx, owner, repo, branchName)
	if err != nil {
		return "", fmt.Errorf("failed to get branch head SHA: %w", err)
	}

	if branchExists {
		return branchHeadSHA, nil
	}

	// Get the default branch SHA to create the new branch from
	defaultBranch, err := c.getDefaultBranch(ctx, owner, repo)
	if err != nil {
		return "", fmt.Errorf("failed to get default branch: %w", err)
	}

	defaultBranchSHA, _, err := c.getBranchHeadSHA(ctx, owner, repo, defaultBranch)
	if err != nil {
		return "", fmt.Errorf("failed to get default branch SHA: %w", err)
	}

	// Create the branch if it doesn't exist - using atomic GitHub API operation
	ref := fmt.Sprintf("refs/heads/%s", branchName)
	_, resp, err := c.Rest.Git.CreateRef(ctx, owner, repo, &github.Reference{
		Ref:    &ref,
		Object: &github.GitObject{SHA: &defaultBranchSHA},
	})

	// Handle race condition: another webhook may have created the branch concurrently
	if err != nil && strings.Contains(err.Error(), "already exists") {
		// Branch was created by another process, get its current SHA
		currentSHA, _, err := c.getBranchHeadSHA(ctx, owner, repo, branchName)
		if err != nil {
			return "", fmt.Errorf("failed to get SHA of concurrently created branch %s: %w", branchName, err)
		}
		return currentSHA, nil
	}

	if err != nil {
		return "", fmt.Errorf("failed to create branch %s: %w", branchName, err)
	}

	if resp != nil && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("failed to create branch %s: %s", branchName, resp.Status)
	}

	return defaultBranchSHA, nil
}

// getBranchNames retrieves the default branch and a deterministic PR branch name for a given operation type.
// Creates one branch per operation type per day to consolidate all operations into a single PR.
func (c *Client) getBranchNames(ctx context.Context, owner, repo, operationType string) (string, string, error) {
	defaultBranch, err := c.getDefaultBranch(ctx, owner, repo)
	if err != nil {
		return "", "", fmt.Errorf("failed to get default branch: %w", err)
	}
	// Use deterministic branch name based on operation type and date
	// This ensures all operations of the same type on the same day use the same branch/PR
	prBranch := fmt.Sprintf("branch-out/%s-tests-%s", operationType, time.Now().Format("2006-01-02"))

	return defaultBranch, prBranch, nil
}

// createOrUpdatePullRequest creates a new pull request or updates an existing one
func (c *Client) createOrUpdatePullRequest(
	ctx context.Context, l zerolog.Logger,
	owner, repo, prBranch, defaultBranch, title, prBody string,
) (string, error) {
	existingPR, err := c.findExistingPR(ctx, owner, repo, prBranch, defaultBranch)
	if err != nil {
		l.Error().Err(err).Msg("Failed to check for existing PR")
		return "", fmt.Errorf("failed to check for existing PR: %w", err)
	}

	var prURL string
	if existingPR != nil {
		l.Debug().Int("pr_number", existingPR.GetNumber()).Msg("Found existing PR, updating")
		prURL, err = c.updatePullRequest(ctx, owner, repo, existingPR.GetNumber(), title, prBody)
		if err != nil {
			l.Error().Err(err).Msg("Failed to update pull request")
			return "", fmt.Errorf("failed to update pull request: %w", err)
		}
	} else {
		l.Debug().Msg("No existing PR found, creating new one")
		prURL, err = c.createPullRequest(ctx, owner, repo, prBranch, defaultBranch, title, prBody)
		if err != nil {
			l.Error().Err(err).Msg("Failed to create pull request")
			return "", fmt.Errorf("failed to create pull request: %w", err)
		}
	}

	return prURL, nil
}

// ResultsCommitter defines the interface for types that can be committed to Git.
type ResultsCommitter interface {
	golang.QuarantineResults | golang.UnquarantineResults
}

// TestOperationConfig contains the operation-specific configuration for processing tests.
type TestOperationConfig[T ResultsCommitter] struct {
	OperationType    string
	PRTitlePrefix    string
	CoreFunc         func(zerolog.Logger, string, []golang.TestTarget, ...interface{}) (T, error)
	BuildFlagsOption func([]string) interface{}
	MetricsInc       func(context.Context, string, string)
	MetricsRecord    func(context.Context, int64)
	MetricsDuration  func(context.Context, time.Duration)
	LogMessage       string
}

// processTests is a generic function that handles both quarantine and unquarantine operations.
// It encapsulates the common workflow pattern shared between both operations.
func processTests[T ResultsCommitter](
	ctx context.Context,
	c *Client,
	l zerolog.Logger,
	repoURL string,
	targets []golang.TestTarget,
	config TestOperationConfig[T],
	options ...FlakyTestOption,
) error {
	opts := &FlakyTestOptions{}
	for _, opt := range options {
		opt(opts)
	}

	host, owner, repo, err := ParseRepoURL(repoURL)
	if err != nil {
		return fmt.Errorf("failed to parse repo URL: %w", err)
	}

	start := time.Now()
	l = l.With().Str("host", host).Str("owner", owner).Str("repo", repo).Logger()

	var packageNames []string
	for _, target := range targets {
		packageNames = append(packageNames, target.Package)
	}

	apiStart := time.Now()
	defaultBranch, prBranch, err := c.getBranchNames(ctx, owner, repo, config.OperationType)
	if err != nil {
		c.metrics.RecordGitHubAPILatency(ctx, "get_default_branch", time.Since(apiStart))
		l.Error().Err(err).Msg("Failed to get default and/or PR branch names")
		return fmt.Errorf("failed to get default and/or PR branch names: %w", err)
	}
	c.metrics.RecordGitHubAPILatency(ctx, "get_default_branch", time.Since(apiStart))
	l.Debug().Str("default_branch", defaultBranch).Str("pr_branch", prBranch).Msg("Got branches")
	l = l.With().Str("pr_branch", prBranch).Logger()

	repository, repoPath, err := c.cloneRepo(owner, repo)
	if err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(repoPath); err != nil {
			l.Error().Err(err).Msg("Failed to remove temporary repository directory")
		}
	}()
	l = l.With().Str("repo_path", repoPath).Logger()
	l.Debug().Msg("Cloned repository")

	branchHeadSHA, err := c.getOrCreateRemoteBranch(ctx, owner, repo, prBranch)
	if err != nil {
		l.Error().Err(err).Msg("Failed to get/create branch")
		return fmt.Errorf("failed to get/create branch: %w", err)
	}

	err = c.checkoutBranchLocal(repository, prBranch)
	if err != nil {
		l.Error().Err(err).Msg("Failed to checkout branch")
		return fmt.Errorf("failed to checkout branch: %w", err)
	}

	results, err := config.CoreFunc(l, repoPath, targets, config.BuildFlagsOption(opts.buildFlags))
	if err != nil {
		return fmt.Errorf("failed to %s tests: %w", config.OperationType, err)
	}

	sha, err := generateCommitAndPush(ctx, c, owner, repo, prBranch, branchHeadSHA, results, config.OperationType)
	if err != nil {
		l.Error().Err(err).Msg("Failed to create commit")
		return fmt.Errorf("failed to create commit: %w", err)
	}
	l = l.With().Str("commit_sha", sha).Logger()

	prStart := time.Now()
	title := fmt.Sprintf("[Auto] [branch-out] %s: %s", config.PRTitlePrefix, time.Now().Format("2006-01-02"))

	// Get markdown using the Markdown method via interface
	var prBody string
	switch r := any(results).(type) {
	case golang.QuarantineResults:
		prBody = r.Markdown(owner, repo, prBranch)
	case golang.UnquarantineResults:
		prBody = r.Markdown(owner, repo, prBranch)
	}

	prURL, err := c.createOrUpdatePullRequest(ctx, l, owner, repo, prBranch, defaultBranch, title, prBody)
	if err != nil {
		c.metrics.RecordGitHubAPILatency(ctx, "create_update_pr", time.Since(prStart))
		l.Error().Err(err).Msg("Failed to create or update pull request")
		return fmt.Errorf("failed to create or update pull request: %w", err)
	}
	c.metrics.RecordGitHubAPILatency(ctx, "create_update_pr", time.Since(prStart))

	// Record final success metrics
	for _, packageName := range packageNames {
		config.MetricsInc(ctx, packageName, "success")
	}
	config.MetricsRecord(ctx, int64(len(results)))
	config.MetricsDuration(ctx, time.Since(start))

	l.Info().
		Str("pr_url", prURL).
		Str("commit_sha", sha).
		Dur("duration", time.Since(start)).
		Msg(config.LogMessage)

	return nil
}

// generateCommitAndPush creates a commit with the modified tests and pushes it to the PR branch.
// It handles both quarantine and unquarantine operations using generics for type safety.
func generateCommitAndPush[T ResultsCommitter](
	ctx context.Context,
	c *Client,
	owner, repo, prBranch, branchHeadSHA string,
	results T,
	_ string,
) (string, error) {
	var commitMessage = strings.Builder{}
	allFileUpdates := make(map[string]string)

	// Handle different result types using type assertion
	switch r := any(results).(type) {
	case golang.QuarantineResults:
		commitMessage.WriteString("branch-out quarantine tests\n")
		for _, result := range r {
			// Process successes
			for _, file := range result.Successes {
				commitMessage.WriteString(fmt.Sprintf("%s: %s\n", file.File, strings.Join(file.TestNames(), ", ")))
				allFileUpdates[file.File] = file.ModifiedSourceCode
			}
		}
	case golang.UnquarantineResults:
		commitMessage.WriteString("branch-out unquarantine tests\n")
		for _, result := range r {
			// Process successes
			for _, file := range result.Successes {
				commitMessage.WriteString(fmt.Sprintf("%s: %s\n", file.File, strings.Join(file.TestNames(), ", ")))
				allFileUpdates[file.File] = file.ModifiedSourceCode
			}
		}
	default:
		return "", fmt.Errorf("unsupported result type: %T", results)
	}

	// Update files on the branch
	sha, err := c.createCommitOnBranch(
		ctx,
		owner,
		repo,
		prBranch,
		commitMessage.String(),
		branchHeadSHA,
		allFileUpdates,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create commit: %w", err)
	}

	return sha, nil
}
