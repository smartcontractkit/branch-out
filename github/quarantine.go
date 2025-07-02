// Package github provides utilities for manipulating GitHub branches and PRs.
package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/google/go-github/v73/github"
	"github.com/rs/zerolog"
	gh_graphql "github.com/shurcooL/githubv4"

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
	owner, repo string,
	targets []golang.QuarantineTarget,
	options ...QuarantineOption,
) error {
	opts := &quarantineTestsOptions{}
	for _, opt := range options {
		opt(opts)
	}

	start := time.Now()

	l = l.With().Str("owner", owner).Str("repo", repo).Logger()

	repoPath, err := c.cloneRepo(owner, repo)
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

	results, err := golang.QuarantineTests(l, repoPath, targets, golang.WithBuildFlags(opts.buildFlags))
	if err != nil {
		return fmt.Errorf("failed to quarantine tests: %w", err)
	}

	defaultBranch, err := c.getDefaultBranch(ctx, owner, repo)
	if err != nil {
		l.Error().Err(err).Msg("Failed to get default branch")
		return fmt.Errorf("failed to get default branch: %w", err)
	}
	l.Debug().Str("default_branch", defaultBranch).Msg("Got default branch")

	branchName := fmt.Sprintf("branch-out/quarantine-tests-%s", time.Now().Format("20060102150405"))
	newBranchHeadSHA, err := c.createBranch(ctx, l, owner, repo, branchName, defaultBranch)
	if err != nil {
		l.Error().Err(err).Msg("Failed to create branch")
		return fmt.Errorf("failed to create branch: %w", err)
	}

	// Build PR description
	var (
		commitMessage = strings.Builder{}
	)

	commitMessage.WriteString("branch-out quarantine tests\n")

	allFileUpdates := make(map[string]string)
	for _, result := range results {
		// Process successes
		for _, file := range result.Successes {
			commitMessage.WriteString(fmt.Sprintf("%s: %s\n", file.File, strings.Join(file.TestNames(), ", ")))
			allFileUpdates[file.File] = file.ModifiedSourceCode
		}
	}
	title := fmt.Sprintf("[Auto] Quarantine Flaky Tests: %s", time.Now().Format("2006-01-02"))

	// Update files
	sha, err := c.updateFiles(ctx, owner, repo, branchName, commitMessage.String(), newBranchHeadSHA, allFileUpdates)
	if err != nil {
		l.Error().Err(err).Msg("Failed to update files")
		return fmt.Errorf("failed to update files: %w", err)
	}
	l = l.With().Str("commit_sha", sha).Logger()
	l.Debug().Int("files_updated", len(allFileUpdates)).Msg("Updated files")

	prURL, err := c.createPullRequest(ctx, owner, repo, branchName, defaultBranch, title, results.Markdown())
	if err != nil {
		l.Error().Err(err).Msg("Failed to create pull request")
		return fmt.Errorf("failed to create pull request: %w", err)
	}

	l.Info().Str("pr_url", prURL).Str("commit_sha", sha).Dur("duration", time.Since(start)).Msg("Created pull request")
	return nil
}

// createBranch creates a new branch from the default branch and returns the head SHA
func (c *Client) createBranch(
	ctx context.Context,
	l zerolog.Logger,
	owner, repo, branchName, baseBranch string,
) (string, error) {
	// Get the base branch reference
	l = l.With().Str("base_branch", baseBranch).Str("branch", branchName).Logger()
	baseRef, _, err := c.Rest.Git.GetRef(ctx, owner, repo, "refs/heads/"+baseBranch)
	if err != nil {
		return "", fmt.Errorf("failed to get base branch reference: %w", err)
	}

	// Create new branch reference
	newRef := &github.Reference{
		Ref: github.Ptr(fmt.Sprintf("refs/heads/%s", branchName)),
		Object: &github.GitObject{
			SHA: baseRef.Object.SHA,
		},
	}

	ref, _, err := c.Rest.Git.CreateRef(ctx, owner, repo, newRef)
	if err != nil {
		// If branch already exists, that's okay
		if !strings.Contains(err.Error(), "already exists") {
			return "", fmt.Errorf("failed to create branch: %w", err)
		}
		l.Debug().Str("ref", ref.GetRef()).Msg("Branch already exists, continuing")
	} else {
		l.Debug().Str("ref", ref.GetRef()).Msg("Created new branch")
	}

	return ref.GetObject().GetSHA(), nil
}

// updateFiles updates multiple files in the repository in a single, signed commit.
// Inspired by https://github.com/planetscale/ghcommit
func (c *Client) updateFiles(
	ctx context.Context,
	owner, repo, branchName, commitMessage, expectedHeadOid string,
	files map[string]string,
) (string, error) {
	// process added / modified files:
	additions := make([]gh_graphql.FileAddition, 0, len(files))
	for file, contents := range files {
		enc, err := base64EncodeFile(contents)
		if err != nil {
			return "", fmt.Errorf("failed to encode file %s: %w", file, err)
		}
		additions = append(additions, gh_graphql.FileAddition{
			Path:     gh_graphql.String(file),
			Contents: gh_graphql.Base64String(enc),
		})
	}
	// the actual mutation request
	var m struct {
		CreateCommitOnBranch struct {
			Commit struct {
				URL string
			}
		} `graphql:"createCommitOnBranch(input:$input)"`
	}

	headline, body := parseCommitMessage(commitMessage)

	// create the $input struct for the graphQL createCommitOnBranch mutation request:
	input := gh_graphql.CreateCommitOnBranchInput{
		Branch: gh_graphql.CommittableBranch{
			RepositoryNameWithOwner: gh_graphql.NewString(gh_graphql.String(fmt.Sprintf("%s/%s", owner, repo))),
			BranchName:              gh_graphql.NewString(gh_graphql.String(branchName)),
		},
		Message: gh_graphql.CommitMessage{
			Headline: gh_graphql.String(headline),
			Body:     gh_graphql.NewString(gh_graphql.String(body)),
		},
		FileChanges: &gh_graphql.FileChanges{
			Additions: &additions,
		},
		ExpectedHeadOid: gh_graphql.GitObjectID(expectedHeadOid),
	}

	err := c.GraphQL.Mutate(ctx, &m, input, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create commit: %w", err)
	}

	return m.CreateCommitOnBranch.Commit.URL, nil
}

// getDefaultBranch gets the default branch of a repository
func (c *Client) getDefaultBranch(ctx context.Context, owner, repo string) (string, error) {
	ghRepo, resp, err := c.Rest.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return "", fmt.Errorf("failed to get repository: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get repository: %s", resp.Status)
	}
	return ghRepo.GetDefaultBranch(), nil
}

// createPullRequest creates a pull request from the branch to the base branch
func (c *Client) createPullRequest(
	ctx context.Context,
	owner, repo, headBranch, baseBranch, title, body string,
) (string, error) {
	pr := &github.NewPullRequest{
		Title: github.Ptr(title),
		Head:  github.Ptr(headBranch),
		Base:  github.Ptr(baseBranch),
		Body:  github.Ptr(body),
	}

	createdPR, _, err := c.Rest.PullRequests.Create(ctx, owner, repo, pr)
	if err != nil {
		return "", fmt.Errorf("failed to create pull request: %w", err)
	}

	prURL := createdPR.GetHTMLURL()
	return prURL, nil
}

// ParseRepoURL parses a GitHub repository URL and returns owner and repo name
func ParseRepoURL(repoURL string) (owner, repo string, err error) {
	if repoURL == "" {
		return "", "", fmt.Errorf("repository URL is required")
	}

	// Parse the URL
	u, err := url.Parse(repoURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid repository URL: %w", err)
	}

	// Extract owner and repo from path
	path := strings.Trim(u.Path, "/")
	parts := strings.Split(path, "/")

	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid repository URL format, expected: https://github.com/owner/repo")
	}

	owner = parts[0]
	repo = parts[1]

	// Remove .git suffix if present
	repo = strings.TrimSuffix(repo, ".git")

	return owner, repo, nil
}

// cloneRepo clones a repo to a temp directory and returns the path to it
func (c *Client) cloneRepo(owner, repo string) (string, error) {
	repoPath, err := os.MkdirTemp("./", fmt.Sprintf("%s-%s-*", owner, repo))
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory: %w", err)
	}
	progress := bytes.NewBuffer(nil)

	// Use url.JoinPath for safe URL construction
	repoURL, err := url.JoinPath(c.Rest.BaseURL.String(), owner, repo)
	if err != nil {
		return "", fmt.Errorf("failed to construct repository URL: %w", err)
	}
	repoURL = strings.Replace(repoURL, "api.github.com", "github.com", 1)
	repoURL += ".git"

	_, err = git.PlainClone(repoPath, false, &git.CloneOptions{
		URL:      repoURL,
		Progress: progress,
	})
	if err != nil {
		return "", fmt.Errorf("failed to clone repository at %s: %w, %s", repoURL, err, progress.String())
	}

	return repoPath, nil
}

// base64EncodeFile encodes a file's contents to base64
func base64EncodeFile(contents string) (string, error) {
	buf := bytes.Buffer{}
	encoder := base64.NewEncoder(base64.StdEncoding, &buf)

	if _, err := io.Copy(encoder, strings.NewReader(contents)); err != nil {
		return "", err
	}
	if err := encoder.Close(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// parseCommitMessage parses a commit message into a headline and body
func parseCommitMessage(msg string) (string, string) {
	parts := strings.SplitN(msg, "\n", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}
