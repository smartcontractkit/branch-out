package github

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// GitCloneRepo clones a repo to a temp directory and returns the path to it
func (c *Client) GitCloneRepo(owner, repoName string) (*git.Repository, string, error) {
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

// GitCheckoutBranch checks out a branch in the local repository.
func (c *Client) GitCheckoutBranch(repo *git.Repository, branchName string) error {
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
