// Package github provides utilities for manipulating GitHub branches and PRs.
package github

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/google/go-github/v73/github"
	"github.com/rs/zerolog"

	"github.com/smartcontractkit/branch-out/golang"
)

// QuarantineTests quarantines multiple Go tests by adding t.Skip() to the test functions and making a PR to the default branch.
func (c *Client) QuarantineTests(
	ctx context.Context,
	l zerolog.Logger,
	owner, repo string,
	targets []golang.QuarantineTarget,
) error {

	return fmt.Errorf("not implemented")
}

// createBranch creates a new branch from the default branch
func (c *Client) createBranch(ctx context.Context, l zerolog.Logger, owner, repo, branchName, baseBranch string) error {
	// Get the base branch reference
	baseRef, _, err := c.Rest.Git.GetRef(ctx, owner, repo, "refs/heads/"+baseBranch)
	if err != nil {
		return fmt.Errorf("failed to get base branch reference: %w", err)
	}

	// Create new branch reference
	newRef := &github.Reference{
		Ref: github.Ptr(fmt.Sprintf("refs/heads/%s", branchName)),
		Object: &github.GitObject{
			SHA: baseRef.Object.SHA,
		},
	}

	_, _, err = c.Rest.Git.CreateRef(ctx, owner, repo, newRef)
	if err != nil {
		// If branch already exists, that's okay
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("failed to create branch: %w", err)
		}
		l.Debug().Msg("Branch already exists, continuing")
	} else {
		l.Debug().Msg("Created new branch")
	}

	return nil
}

// updateFile updates a file in the repository
func (c *Client) updateFile(
	ctx context.Context,
	l zerolog.Logger,
	owner, repo, filePath, branchName, content, commitMessage, sha string,
) (string, error) {
	updateOptions := &github.RepositoryContentFileOptions{
		Message: github.Ptr(commitMessage),
		Content: []byte(content),
		SHA:     github.Ptr(sha),
		Branch:  github.Ptr(branchName),
	}

	contentResponse, _, err := c.Rest.Repositories.UpdateFile(ctx, owner, repo, filePath, updateOptions)
	if err != nil {
		return "", fmt.Errorf("failed to update file: %w", err)
	}

	l.Debug().Str("commit_sha", contentResponse.Commit.GetSHA()).Msg("Updated file")
	return contentResponse.GetSHA(), nil
}

// createPullRequest creates a pull request from the branch to the base branch
func (c *Client) createPullRequest(
	ctx context.Context,
	l zerolog.Logger,
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
	l.Debug().Str("pr_url", prURL).Msg("Created pull request")
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
	repoPath, err := os.MkdirTemp("", fmt.Sprintf("%s-%s-*", owner, repo))
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory: %w", err)
	}
	progress := bytes.NewBuffer(nil)
	_, err = git.PlainClone(repoPath, false, &git.CloneOptions{
		URL:      fmt.Sprintf("https://github.com/%s/%s.git", owner, repo),
		Progress: progress,
	})
	if err != nil {
		return "", fmt.Errorf("failed to clone repository: %w, %s", err, progress.String())
	}

	return repoPath, nil
}
