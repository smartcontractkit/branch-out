package processing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/smartcontractkit/branch-out/golang"
	"github.com/smartcontractkit/branch-out/jira"
	"github.com/smartcontractkit/branch-out/telemetry"
	"github.com/smartcontractkit/branch-out/trunk"
)

type WebhookProcessor struct {
	logger       zerolog.Logger
	jiraClient   JiraClient
	trunkClient  TrunkClient
	githubClient GithubClient
	// golangClient golang.Client
	metrics *telemetry.Metrics
}

func NewWebhookProcessor(
	logger zerolog.Logger,
	jiraClient JiraClient,
	trunkClient TrunkClient,
	githubClient GithubClient,
	// golangClient golang.Client,
	metrics *telemetry.Metrics,
) *WebhookProcessor {
	return &WebhookProcessor{
		logger:       logger.With().Str("component", "webhook_processor").Logger(),
		githubClient: githubClient,
		// golangClient: golangClient,
		metrics: metrics,
	}
}

// processWebhookPayload processes a webhook payload that came from SQS.
func (w *WebhookProcessor) ProcessWebhookPayload(payload string) error {
	if err := w.verifyClients(); err != nil {
		return err
	}

	w.logger.Debug().Str("payload", payload).Msg("Processing webhook payload from SQS")

	var webhookData trunk.TestCaseStatusChange
	if err := json.Unmarshal([]byte(payload), &webhookData); err != nil {
		w.logger.Error().
			Err(err).
			Str("payload", payload).
			Msg("Failed to parse test_case.status_changed payload from SQS")
		return fmt.Errorf("failed to parse test_case.status_changed payload: %w", err)
	}

	l := w.logger.With().
		Str("id", webhookData.TestCase.ID).
		Str("name", webhookData.TestCase.Name).
		Str("current_status", webhookData.StatusChange.CurrentStatus.Value).
		Str("previous_status", webhookData.StatusChange.PreviousStatus).
		Logger()

	return w.handleTestCaseStatusChanged(l, webhookData)
}

// verifyClients verifies that all the clients are not nil.
func (w *WebhookProcessor) verifyClients() error {
	if w.jiraClient == nil {
		return fmt.Errorf("jira client is nil")
	}
	if w.trunkClient == nil {
		return fmt.Errorf("trunk client is nil")
	}
	if w.githubClient == nil {
		return fmt.Errorf("github client is nil")
	}
	return nil
}

// handleTestCaseStatusChanged processes when a test case's status changes.
func (w *WebhookProcessor) handleTestCaseStatusChanged(l zerolog.Logger, statusChange trunk.TestCaseStatusChange) error {
	testCase := statusChange.TestCase
	currentStatus := statusChange.StatusChange.CurrentStatus.Value

	l = l.With().
		Str("repo_url", testCase.Repository.HTMLURL).
		Str("package", testCase.TestSuite).
		Str("file_path", testCase.FilePath).
		Logger()

	l.Info().Msg("Processing test case status change")

	switch currentStatus {
	case trunk.TestCaseStatusFlaky:
		return w.handleFlakyTest(l, statusChange)
	case trunk.TestCaseStatusBroken:
		return w.handleBrokenTest(l, statusChange)
	case trunk.TestCaseStatusHealthy:
		return w.handleHealthyTest(l, statusChange)
	}

	return nil
}

// handleFlakyTest handles the case where a test is marked as flaky.
func (w *WebhookProcessor) handleFlakyTest(l zerolog.Logger, statusChange trunk.TestCaseStatusChange) error {
	start := time.Now()
	testCase := statusChange.TestCase

	l.Debug().Msg("Quarantining flaky test")

	// Record flaky test detection
	w.metrics.IncFlakyTestDetected(context.Background(), testCase.Name, testCase.TestSuite)

	// Create a Jira ticket for the flaky test
	issue, err := w.createJiraIssueForFlakyTest(l, statusChange)
	if err != nil {
		return fmt.Errorf("failed to create Jira ticket: %w", err)
	}

	// Add a comment with the current status details (for both new and existing tickets)
	err = w.jiraClient.AddCommentToFlakyTestIssue(issue, statusChange)
	if err != nil {
		l.Warn().
			Err(err).
			Str("jira_issue_key", issue.Key).
			Msg("Failed to add comment to Jira ticket (non-blocking)")
	} else {
		l.Debug().
			Str("jira_issue_key", issue.Key).
			Msg("Successfully added status comment to Jira ticket")
	}

	// Quarantine the test in GitHub
	err = w.githubClient.QuarantineTests(
		context.Background(),
		l,
		testCase.Repository.HTMLURL,
		[]golang.QuarantineTarget{
			{
				Package: testCase.TestSuite,
				Tests:   []string{testCase.Name},
			},
		},
	)
	if err != nil {
		return fmt.Errorf("failed to quarantine test: %w", err)
	}

	// Record time to quarantine
	w.metrics.RecordTimeToQuarantine(context.Background(), time.Since(start))

	return nil
}

// handleBrokenTest handles the case where a test is marked as broken.
func (w *WebhookProcessor) handleBrokenTest(l zerolog.Logger, statusChange trunk.TestCaseStatusChange) error {
	return w.handleFlakyTest(l, statusChange)
}

// handleHealthyTest handles the case where a test is marked as healthy.
func (w *WebhookProcessor) handleHealthyTest(l zerolog.Logger, statusChange trunk.TestCaseStatusChange) error {
	testCase := statusChange.TestCase

	// Record test recovery if it was previously flaky
	if statusChange.StatusChange.PreviousStatus == trunk.TestCaseStatusFlaky {
		w.metrics.IncTestRecovered(context.Background())
		l.Info().Msg("Test recovered from flaky status")
	}

	// Look for an existing open ticket for this test
	issue, err := w.jiraClient.GetOpenFlakyTestIssue(testCase.TestSuite, testCase.Name)
	if errors.Is(err, jira.ErrNoOpenFlakyTestIssueFound) {
		l.Debug().
			Str("test_suite", testCase.TestSuite).
			Str("test_name", testCase.Name).
			Msg("No open flaky test ticket found for healthy test")
		return nil
	} else if err != nil {
		l.Error().
			Err(err).
			Str("test_suite", testCase.TestSuite).
			Str("test_name", testCase.Name).
			Msg("Failed to check for existing Jira ticket")
		return fmt.Errorf("failed to check for existing Jira ticket: %w", err)
	}

	l.Info().
		Str("jira_issue_key", issue.Key).
		Msg("Found open flaky test ticket for healthy test - will close it")

	// Close the ticket with a formatted healthy comment
	err = w.jiraClient.CloseIssueWithHealthyComment(issue.Key, statusChange)
	if err != nil {
		l.Warn().
			Err(err).
			Str("jira_issue_key", issue.Key).
			Msg("Failed to close Jira ticket for healthy test (non-blocking)")
		// Don't fail the whole operation if closing fails
	} else {
		l.Info().
			Str("jira_issue_key", issue.Key).
			Msg("Successfully closed Jira ticket for recovered test")
	}

	return nil
}

// createJiraIssueForFlakyTest looks for an existing open ticket or creates a new one.
func (w *WebhookProcessor) createJiraIssueForFlakyTest(
	l zerolog.Logger,
	statusChange trunk.TestCaseStatusChange,
) (jira.FlakyTestIssue, error) {
	testCase := statusChange.TestCase

	// Create details JSON from test case data
	details, err := json.Marshal(map[string]any{
		"failure_rate_last_7d":           testCase.FailureRateLast7D,
		"pull_requests_impacted_last_7d": testCase.PullRequestsImpactedLast7D,
		"codeowners":                     testCase.Codeowners,
		"html_url":                       testCase.HTMLURL,
		"repository_url":                 testCase.Repository.HTMLURL,
		"current_status":                 statusChange.StatusChange.CurrentStatus,
		"previous_status":                statusChange.StatusChange.PreviousStatus,
		"test_suite":                     testCase.TestSuite,
		"variant":                        testCase.Variant,
	})
	if err != nil {
		return jira.FlakyTestIssue{}, fmt.Errorf("failed to marshal test case details: %w", err)
	}

	req := jira.FlakyTestIssueRequest{
		ProjectKey:        w.jiraClient.GetProjectKey(),
		RepoURL:           testCase.Repository.HTMLURL,
		Package:           testCase.TestSuite,
		Test:              testCase.Name,
		FilePath:          testCase.FilePath,
		TrunkID:           testCase.ID,
		AdditionalDetails: string(details),
	}

	// Try to get an existing Jira ticket for the flaky test
	issue, err := w.jiraClient.GetOpenFlakyTestIssue(testCase.TestSuite, testCase.Name)
	if errors.Is(err, jira.ErrNoOpenFlakyTestIssueFound) {
		// No existing ticket found, create a new one
		issue, err = w.jiraClient.CreateFlakyTestIssue(req)
		if err != nil {
			return jira.FlakyTestIssue{}, fmt.Errorf("failed to create Jira ticket: %w", err)
		}
	} else if err != nil {
		// Some other error occurred
		return jira.FlakyTestIssue{}, fmt.Errorf("failed to get existing Jira ticket: %w", err)
	}

	// Link the Jira ticket back to the Trunk test case
	if err := w.trunkClient.LinkTicketToTestCase(testCase.ID, issue.Key, testCase.Repository.HTMLURL); err != nil {
		l.Warn().Err(err).Msg("Failed to link Jira ticket to Trunk test case (non-blocking)")
		// Don't return error as the ticket was created successfully
	}

	return issue, nil
}
