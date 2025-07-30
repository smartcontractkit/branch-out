// Package github provides utilities for manipulating GitHub branches and PRs.
package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/go-github/v73/github"
	"github.com/rs/zerolog"

	"github.com/smartcontractkit/branch-out/golang"
)

// QuarantineTestsOptions describes the options for the QuarantineTests function.
type quarantineTestsOptions struct {
	buildFlags []string // Any build flags to pass to the go command (e.g. ["-tags", "integration"])
}

// QuarantineOption is a function that can be used to configure the QuarantineTests function.
type QuarantineOption func(*quarantineTestsOptions)

// WithBuildFlags sets the build flags to use when loading packages.
func WithBuildFlags(buildFlags []string) QuarantineOption {
	return func(options *quarantineTestsOptions) {
		options.buildFlags = buildFlags
	}
}

// QuarantineTests quarantines multiple Go tests by adding t.Skip() to the test functions and making a PR to the default branch.
func (c *Client) QuarantineTests(
	ctx context.Context,
	l zerolog.Logger,
	repoURL string,
	targets []golang.QuarantineTarget,
	options ...QuarantineOption,
) error {
	opts := &quarantineTestsOptions{}
	for _, opt := range options {
		opt(opts)
	}

	host, owner, repo, err := ParseRepoURL(repoURL)
	if err != nil {
		return fmt.Errorf("failed to parse repo URL: %w", err)
	}

	start := time.Now()
	l = l.With().Str("host", host).Str("owner", owner).Str("repo", repo).Logger()

	// Record quarantine operation start
	var packageNames []string
	for _, target := range targets {
		packageNames = append(packageNames, target.Package)
	}

	// 1. Get branch names
	apiStart := time.Now()
	defaultBranch, prBranch, err := c.getBranchNames(ctx, owner, repo)
	if err != nil {
		c.metrics.RecordGitHubAPILatency(ctx, "get_default_branch", time.Since(apiStart))
		return &GitHubAPIError{
			Operation:  "get_default_branch",
			Owner:      owner,
			Repo:       repo,
			Underlying: err,
		}
	}
	c.metrics.RecordGitHubAPILatency(ctx, "get_default_branch", time.Since(apiStart))
	l.Debug().Str("default_branch", defaultBranch).Str("pr_branch", prBranch).Msg("Got branches")
	l = l.With().Str("pr_branch", prBranch).Logger()

	// 2. Clone the repository to a temporary directory
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

	// 3. Get or create the PR branch
	branchHeadSHA, err := c.getOrCreateRemoteBranch(ctx, owner, repo, prBranch)
	if err != nil {
		l.Error().Err(err).Msg("Failed to get/create branch")
		return &GitHubAPIError{
			Operation:  "get_create_branch",
			Owner:      owner,
			Repo:       repo,
			Branch:     prBranch,
			Underlying: err,
		}
	}

	// 4. Checkout the branch locally
	err = c.checkoutBranchLocal(repository, prBranch)
	if err != nil {
		l.Error().Err(err).Msg("Failed to checkout branch")
		return fmt.Errorf("failed to checkout branch: %w", err)
	}

	// 5. Quarantine tests - writing to the local repository
	results, err := golang.QuarantineTests(l, repoPath, targets, golang.WithBuildFlags(opts.buildFlags))
	if err != nil {
		return fmt.Errorf("failed to quarantine tests: %w", err)
	}

	// 6. Create a commit with the quarantined tests
	sha, err := c.generateCommitAndPush(ctx, owner, repo, prBranch, branchHeadSHA, &results)
	if err != nil {
		l.Error().Err(err).Msg("Failed to create commit")
		return &GitHubAPIError{
			Operation:  "create_commit",
			Owner:      owner,
			Repo:       repo,
			Branch:     prBranch,
			Underlying: err,
		}
	}
	l = l.With().Str("commit_sha", sha).Logger()

	// 7. Create or update the pull request
	prStart := time.Now()
	prURL, err := c.createOrUpdatePullRequest(ctx, l, owner, repo, prBranch, defaultBranch, &results)
	if err != nil {
		c.metrics.RecordGitHubAPILatency(ctx, "create_update_pr", time.Since(prStart))
		l.Error().Err(err).Msg("Failed to create or update pull request")
		return &GitHubAPIError{
			Operation:  "create_update_pr",
			Owner:      owner,
			Repo:       repo,
			Branch:     prBranch,
			Underlying: err,
		}
	}
	c.metrics.RecordGitHubAPILatency(ctx, "create_update_pr", time.Since(prStart))
	// Record final success metrics
	for _, packageName := range packageNames {
		c.metrics.IncQuarantineOperation(ctx, packageName, "success")
	}
	c.metrics.RecordQuarantineFilesModified(ctx, int64(len(results)))
	c.metrics.RecordQuarantineDuration(ctx, time.Since(start))

	l.Info().
		Str("pr_url", prURL).
		Str("commit_sha", sha).
		Dur("duration", time.Since(start)).
		Msg("Created or updated pull request")

	return nil
}

// getBranchNames retrieves the default branch and a deterministic PR branch name based on the current date.
func (c *Client) getBranchNames(ctx context.Context, owner, repo string) (string, string, error) {
	defaultBranch, err := c.getDefaultBranch(ctx, owner, repo)
	if err != nil {
		return "", "", &GitHubAPIError{
			Operation:  "get_default_branch",
			Owner:      owner,
			Repo:       repo,
			Underlying: err,
		}
	}
	// Use deterministic branch name based on date
	prBranch := fmt.Sprintf("branch-out/quarantine-tests-%s", time.Now().Format("2006-01-02"))

	return defaultBranch, prBranch, nil
}

// getOrCreateBranch returns the HEAD SHA of a branch, creating it if it doesn't exist.
func (c *Client) getOrCreateRemoteBranch(ctx context.Context, owner, repo, branchName string) (string, error) {
	branchHeadSHA, branchExists, err := c.getBranchHeadSHA(ctx, owner, repo, branchName)
	if err != nil {
		return "", &GitHubAPIError{
			Operation:  "get_branch_head_sha",
			Owner:      owner,
			Repo:       repo,
			Branch:     branchName,
			Underlying: err,
		}
	}

	if branchExists {
		return branchHeadSHA, nil
	}

	// Create the branch if it doesn't exist
	ref := fmt.Sprintf("refs/heads/%s", branchName)
	_, resp, err := c.Rest.Git.CreateRef(ctx, owner, repo, &github.Reference{
		Ref:    &ref,
		Object: &github.GitObject{SHA: &branchHeadSHA},
	})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return "", &GitHubAPIError{
			Operation:  "create_branch",
			Owner:      owner,
			Repo:       repo,
			Branch:     branchName,
			Underlying: err,
		}
	}
	if resp.StatusCode != http.StatusCreated {
		return "", &GitHubAPIError{
			Operation:  "create_branch",
			Owner:      owner,
			Repo:       repo,
			Branch:     branchName,
			StatusCode: resp.StatusCode,
			Underlying: fmt.Errorf("failed to create branch %s: %s", branchName, resp.Status),
		}
	}

	return branchHeadSHA, nil
}

// generateCommitAndPush creates a commit with the quarantined tests and pushes it to the PR branch
func (c *Client) generateCommitAndPush(
	ctx context.Context,
	owner, repo, prBranch, branchHeadSHA string,
	results *golang.QuarantineResults) (string, error) {
	var commitMessage = strings.Builder{}
	commitMessage.WriteString("branch-out quarantine tests\n")

	allFileUpdates := make(map[string]string)
	for _, result := range *results {
		// Process successes
		for _, file := range result.Successes {
			commitMessage.WriteString(fmt.Sprintf("%s: %s\n", file.File, strings.Join(file.TestNames(), ", ")))
			allFileUpdates[file.File] = file.ModifiedSourceCode
		}
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
		return "", &GitHubAPIError{
			Operation:  "create_commit_on_branch",
			Owner:      owner,
			Repo:       repo,
			Branch:     prBranch,
			Underlying: err,
		}
	}

	return sha, nil
}

// createOrUpdatePullRequest creates a new pull request or updates an existing one with the quarantined tests
func (c *Client) createOrUpdatePullRequest(
	ctx context.Context, l zerolog.Logger,
	owner, repo, prBranch, defaultBranch string,
	results *golang.QuarantineResults,
) (string, error) {
	title := fmt.Sprintf("[Auto] [branch-out] Quarantine Flaky Tests: %s", time.Now().Format("2006-01-02"))
	prBody := results.Markdown(owner, repo, prBranch)

	existingPR, err := c.findExistingPR(ctx, owner, repo, prBranch, defaultBranch)
	if err != nil {
		l.Error().Err(err).Msg("Failed to check for existing PR")
		return "", &GitHubAPIError{
			Operation:  "find_existing_pr",
			Owner:      owner,
			Repo:       repo,
			Branch:     prBranch,
			Underlying: err,
		}
	}

	var prURL string
	if existingPR != nil {
		l.Debug().Int("pr_number", existingPR.GetNumber()).Msg("Found existing PR, updating")
		prURL, err = c.updatePullRequest(ctx, owner, repo, existingPR.GetNumber(), title, prBody)
		if err != nil {
			l.Error().Err(err).Msg("Failed to update pull request")
			return "", &GitHubAPIError{
				Operation:  "update_pull_request",
				Owner:      owner,
				Repo:       repo,
				Branch:     prBranch,
				Underlying: err,
			}
		}
	} else {
		l.Debug().Msg("No existing PR found, creating new one")
		prURL, err = c.createPullRequest(ctx, owner, repo, prBranch, defaultBranch, title, prBody)
		if err != nil {
			l.Error().Err(err).Msg("Failed to create pull request")
			return "", &GitHubAPIError{
				Operation:  "create_pull_request",
				Owner:      owner,
				Repo:       repo,
				Branch:     prBranch,
				Underlying: err,
			}
		}
	}

	return prURL, nil
}

// ParseRepoURL parses a GitHub repository URL and returns owner and repo name
func ParseRepoURL(repoURL string) (host, owner, repo string, err error) {
	if repoURL == "" {
		return "", "", "", fmt.Errorf("repository URL is required")
	}

	// Parse the URL
	u, err := url.Parse(repoURL)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid repository URL: %w", err)
	}

	host = u.Host

	// Extract owner and repo from path
	path := strings.Trim(u.Path, "/")
	parts := strings.Split(path, "/")

	if len(parts) < 2 {
		return "", "", "", fmt.Errorf("invalid repository URL format, expected: https://github.com/owner/repo")
	}

	owner = parts[0]
	repo = parts[1]

	// Remove .git suffix if present
	repo = strings.TrimSuffix(repo, ".git")

	return host, owner, repo, nil
}
