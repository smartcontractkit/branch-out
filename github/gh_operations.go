package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/go-github/v73/github"
	"github.com/rs/zerolog"
	gh_graphql "github.com/shurcooL/githubv4"

	"github.com/smartcontractkit/branch-out/golang"
)

// GetBranchNames retrieves the default branch and a deterministic PR branch name based on the current date.
func (c *Client) GetBranchNames(ctx context.Context, owner, repo string) (string, string, error) {
	defaultBranch, err := c.getDefaultBranch(ctx, owner, repo)
	if err != nil {
		return "", "", fmt.Errorf("failed to get default branch: %w", err)
	}
	// Use deterministic branch name based on date
	prBranch := fmt.Sprintf("branch-out/quarantine-tests-%s", time.Now().Format("2006-01-02"))

	return defaultBranch, prBranch, nil
}

// GetOrCreateRemoteBranch returns the HEAD SHA of a branch, creating it if it doesn't exist.
func (c *Client) GetOrCreateRemoteBranch(ctx context.Context, owner, repo, branchName string) (string, error) {
	branchHeadSHA, branchExists, err := c.getBranchHeadSHA(ctx, owner, repo, branchName)
	if err != nil {
		return "", fmt.Errorf("failed to get branch head SHA: %w", err)
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
		return "", fmt.Errorf("failed to create branch %s: %w", branchName, err)
	}
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("failed to create branch %s: %s", branchName, resp.Status)
	}

	return branchHeadSHA, nil
}

// GenerateCommitAndPush creates a commit with the quarantined tests and pushes it to the PR branch
func (c *Client) GenerateCommitAndPush(
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
		return "", fmt.Errorf("failed to create commit: %w", err)
	}

	return sha, nil
}

// CreateOrUpdatePullRequest creates a new pull request or updates an existing one with the quarantined tests
func (c *Client) CreateOrUpdatePullRequest(
	ctx context.Context, l zerolog.Logger,
	owner, repo, prBranch, defaultBranch string,
	results *golang.QuarantineResults,
) (string, error) {
	title := fmt.Sprintf("[Auto] [branch-out] Quarantine Flaky Tests: %s", time.Now().Format("2006-01-02"))
	prBody := results.Markdown(owner, repo, prBranch)

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
