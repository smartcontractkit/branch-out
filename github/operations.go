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

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/google/go-github/v73/github"
	gh_graphql "github.com/shurcooL/githubv4"
)

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

// getBranchHeadSHA returns the HEAD SHA of a branch if it exists, along with a boolean indicating if it exists
func (c *Client) getBranchHeadSHA(ctx context.Context, owner, repo, branchName string) (string, bool, error) {
	ref, _, err := c.Rest.Git.GetRef(ctx, owner, repo, "refs/heads/"+branchName)
	if err != nil {
		// If branch doesn't exist, that's not an error for our purposes
		if strings.Contains(err.Error(), "Not Found") {
			return "", false, nil
		}
		return "", false, fmt.Errorf("failed to get branch reference: %w", err)
	}

	sha := ref.GetObject().GetSHA()
	if sha == "" {
		return "", false, fmt.Errorf("branch %s has no SHA", branchName)
	}

	return sha, true, nil
}

// checkoutBranchLocal checks out a branch in the local repository.
func (c *Client) checkoutBranchLocal(repo *git.Repository, branchName string) error {
	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Pull the latest changes to ensure we have the most up-to-date state
	err = worktree.Pull(&git.PullOptions{
		RemoteName: "origin",
		Force:      true,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to pull latest changes for repo: %w", err)
	}

	// Check if the local branch already exists
	branchRef := plumbing.NewBranchReferenceName(branchName)
	_, err = repo.Reference(branchRef, true)

	if err != nil {
		// Local branch doesn't exist, so we need to create it from the remote branch
		// First, get the remote branch reference
		remoteBranchRef := plumbing.NewRemoteReferenceName("origin", branchName)
		remoteRef, err := repo.Reference(remoteBranchRef, true)
		if err != nil {
			return fmt.Errorf("failed to get remote branch reference %s: %w", remoteBranchRef, err)
		}

		// Create a local branch that tracks the remote branch
		err = worktree.Checkout(&git.CheckoutOptions{
			Branch: branchRef,
			Create: true,
			Hash:   remoteRef.Hash(),
		})
		if err != nil {
			return fmt.Errorf("failed to create and checkout branch %s: %w", branchName, err)
		}
	} else {
		// Local branch exists, just checkout
		err = worktree.Checkout(&git.CheckoutOptions{
			Branch: branchRef,
			Create: false,
		})
		if err != nil {
			return fmt.Errorf("failed to checkout existing branch %s: %w", branchName, err)
		}
	}

	return nil
}

// cloneRepo clones a repo to a temp directory and returns the path to it
func (c *Client) cloneRepo(owner, repoName string) (*git.Repository, string, error) {
	repoPath, err := os.MkdirTemp("/tmp", fmt.Sprintf("%s-%s-*", owner, repoName))
	if err != nil {
		return nil, "", fmt.Errorf("failed to create temporary directory: %w", err)
	}
	progress := bytes.NewBuffer(nil)

	// Use url.JoinPath for safe URL construction
	repoURL, err := url.JoinPath(c.Rest.BaseURL.String(), owner, repoName)
	if err != nil {
		return nil, "", fmt.Errorf("failed to construct repository URL: %w", err)
	}
	repoURL = strings.Replace(repoURL, "api.github.com", "github.com", 1)
	repoURL += ".git"

	repo, err := git.PlainClone(repoPath, false, &git.CloneOptions{
		URL:      repoURL,
		Progress: progress,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to clone repository at %s: %w, %s", repoURL, err, progress.String())
	}

	return repo, repoPath, nil
}

// createCommitOnBranch creates a commit on a specific branch with the given files.
func (c *Client) createCommitOnBranch(
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

// parseCommitMessage parses a commit message into a headline and body
func parseCommitMessage(msg string) (string, string) {
	parts := strings.SplitN(msg, "\n", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
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

	// Add the "branch-out" label immediately after creation
	// Note: GitHub API doesn't support setting labels during PR creation,
	// so we need a separate API call to the Issues endpoint.
	// Label addition failure is non-fatal - we ignore any errors since the main operation succeeded.
	_, _, _ = c.Rest.Issues.AddLabelsToIssue(ctx, owner, repo, createdPR.GetNumber(), []string{"branch-out"})

	prURL := createdPR.GetHTMLURL()
	return prURL, nil
}

// findExistingPR finds an existing open PR from the given branch to the base branch
func (c *Client) findExistingPR(
	ctx context.Context,
	owner, repo, headBranch, baseBranch string,
) (*github.PullRequest, error) {
	// List open PRs for the repository
	prs, _, err := c.Rest.PullRequests.List(ctx, owner, repo, &github.PullRequestListOptions{
		State: "open",
		Head:  fmt.Sprintf("%s:%s", owner, headBranch),
		Base:  baseBranch,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pull requests: %w", err)
	}

	// Return the first matching PR (there should be at most one)
	if len(prs) > 0 {
		return prs[0], nil
	}

	return nil, nil
}

// updatePullRequest updates an existing pull request
func (c *Client) updatePullRequest(
	ctx context.Context,
	owner, repo string,
	prNumber int,
	title, body string,
) (string, error) {
	pr := &github.PullRequest{
		Title: github.Ptr(title),
		Body:  github.Ptr(body),
	}

	updatedPR, _, err := c.Rest.PullRequests.Edit(ctx, owner, repo, prNumber, pr)
	if err != nil {
		return "", fmt.Errorf("failed to update pull request: %w", err)
	}

	return updatedPR.GetHTMLURL(), nil
}
