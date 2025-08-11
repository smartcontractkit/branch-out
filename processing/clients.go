package processing

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/go-git/go-git/v5"
	"github.com/rs/zerolog"

	"github.com/smartcontractkit/branch-out/golang"
	"github.com/smartcontractkit/branch-out/jira"
	"github.com/smartcontractkit/branch-out/trunk"
)

// We define interfaces in the consumer package (processing) to keep with Go idioms.
// Namely, accept interfaces, return structs: https://bryanftan.medium.com/accept-interfaces-return-structs-in-go-d4cab29a301b

// AWSClient interacts with AWS services.
type AWSClient interface {
	PushMessageToQueue(
		ctx context.Context,
		l zerolog.Logger,
		payload string) error
	ReceiveMessageFromQueue(
		ctx context.Context,
		l zerolog.Logger,
	) (*sqs.ReceiveMessageOutput, error)
	DeleteMessageFromQueue(
		ctx context.Context,
		l zerolog.Logger,
		receiptHandle string,
	) error
}

// JiraClient interacts with Jira.
type JiraClient interface {
	CreateFlakyTestIssue(req jira.FlakyTestIssueRequest) (jira.FlakyTestIssue, error)
	GetOpenFlakyTestIssues() ([]jira.FlakyTestIssue, error)
	GetOpenFlakyTestIssue(packageName, testName string) (jira.FlakyTestIssue, error)
	GetProjectKey() string
	AddCommentToFlakyTestIssue(issue jira.FlakyTestIssue, statusChange trunk.TestCaseStatusChange) error
	CloseIssue(issueKey, comment string) error
	CloseIssueWithHealthyComment(issueKey string, statusChange trunk.TestCaseStatusChange) error
}

// TrunkClient interacts with Trunk.io.
type TrunkClient interface {
	QuarantinedTests(repoURL string, orgURLSlug string) ([]trunk.TestCase, error)
	LinkTicketToTestCase(testCaseID string, issueKey string, repoURL string) error
}

// GithubClient interacts with GitHub.
type GithubClient interface {
	GetBranchNames(ctx context.Context, owner, repo string) (string, string, error)
	GetOrCreateRemoteBranch(ctx context.Context, owner, repo, branchName string) (string, error)
	GitCloneRepo(owner, repoName string) (*git.Repository, string, error)
	GitCheckoutBranch(repo *git.Repository, branchName string) error
	GenerateCommitAndPush(
		ctx context.Context,
		owner, repoName, branchName, brancHeadSHA string,
		results *golang.QuarantineResults,
	) (string, error)
	CreateOrUpdatePullRequest(
		ctx context.Context,
		l zerolog.Logger,
		owner, repo, prBranch, defaultBranch string,
		results *golang.QuarantineResults,
	) (string, error)
}
