package processing

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"

	"github.com/smartcontractkit/branch-out/golang"
	"github.com/smartcontractkit/branch-out/trunk"
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
func (w *WebhookProcessor) QuarantineTests(
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

	host, owner, repo, err := trunk.ParseRepoURL(repoURL)
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
	defaultBranch, prBranch, err := w.githubClient.GetBranchNames(ctx, owner, repo)
	if err != nil {
		w.metrics.RecordGitHubAPILatency(ctx, "get_default_branch", time.Since(apiStart))
		l.Error().Err(err).Msg("Failed to get default and/or PR branch names")
		return fmt.Errorf("failed to get default and/or PR branch names: %w", err)
	}
	w.metrics.RecordGitHubAPILatency(ctx, "get_default_branch", time.Since(apiStart))
	l.Debug().Str("default_branch", defaultBranch).Str("pr_branch", prBranch).Msg("Got branches")
	l = l.With().Str("pr_branch", prBranch).Logger()

	// 2. Clone the repository to a temporary directory
	repository, repoPath, err := w.githubClient.GitCloneRepo(owner, repo)
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
	branchHeadSHA, err := w.githubClient.GetOrCreateRemoteBranch(ctx, owner, repo, prBranch)
	if err != nil {
		l.Error().Err(err).Msg("Failed to get/create branch")
		return fmt.Errorf("failed to get/create branch: %w", err)
	}

	// 4. Checkout the branch locally
	err = w.githubClient.GitCheckoutBranch(repository, prBranch)
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
	sha, err := w.githubClient.GenerateCommitAndPush(ctx, owner, repo, prBranch, branchHeadSHA, &results)
	if err != nil {
		l.Error().Err(err).Msg("Failed to create commit")
		return fmt.Errorf("failed to create commit: %w", err)
	}
	l = l.With().Str("commit_sha", sha).Logger()

	// 7. Create or update the pull request
	prStart := time.Now()
	prURL, err := w.githubClient.CreateOrUpdatePullRequest(ctx, l, owner, repo, prBranch, defaultBranch, &results)
	if err != nil {
		w.metrics.RecordGitHubAPILatency(ctx, "create_update_pr", time.Since(prStart))
		l.Error().Err(err).Msg("Failed to create or update pull request")
		return fmt.Errorf("failed to create or update pull request: %w", err)
	}
	w.metrics.RecordGitHubAPILatency(ctx, "create_update_pr", time.Since(prStart))
	// Record final success metrics
	for _, packageName := range packageNames {
		w.metrics.IncQuarantineOperation(ctx, packageName, "success")
	}
	w.metrics.RecordQuarantineFilesModified(ctx, int64(len(results)))
	w.metrics.RecordQuarantineDuration(ctx, time.Since(start))

	l.Info().
		Str("pr_url", prURL).
		Str("commit_sha", sha).
		Dur("duration", time.Since(start)).
		Msg("Created or updated pull request")

	return nil
}
