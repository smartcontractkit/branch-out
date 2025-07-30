// Package processing provides background processing functionality for SQS messages.
package processing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/rs/zerolog"

	"github.com/smartcontractkit/branch-out/golang"
	"github.com/smartcontractkit/branch-out/jira"
	"github.com/smartcontractkit/branch-out/telemetry"
	"github.com/smartcontractkit/branch-out/trunk"
)

// Worker handles background processing of SQS messages and webhook business logic.
type Worker struct {
	logger       zerolog.Logger
	awsClient    AWSClient
	jiraClient   JiraClient
	trunkClient  TrunkClient
	githubClient GithubClient
	metrics      *telemetry.Metrics

	// Configuration
	pollInterval time.Duration

	// State management
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running bool
	mu      sync.RWMutex
}

// Config holds configuration for the worker.
type Config struct {
	PollInterval time.Duration
}

// Operation name constants for consistent logging
const (
	opReceiveMessages  = "receive_messages"
	opDeleteMessage    = "delete_message"
	opQuarantineTest   = "quarantine_test"
	opCreateJiraTicket = "create_jira_ticket"
	opGetOpenTicket    = "get_open_ticket"
	opCloseTicket      = "close_ticket"
	opAddJiraComment   = "add_jira_comment"
	opLinkToTrunk      = "link_to_trunk"
	opProcessWebhook   = "process_webhook"
)



// NewWorker creates a new background worker for processing SQS messages.
func NewWorker(
	logger zerolog.Logger,
	awsClient AWSClient,
	jiraClient JiraClient,
	trunkClient TrunkClient,
	githubClient GithubClient,
	metrics *telemetry.Metrics,
	config Config,
) *Worker {
	ctx, cancel := context.WithCancel(context.Background())

	if config.PollInterval == 0 {
		config.PollInterval = 5 * time.Second // Default poll interval
	}

	return &Worker{
		logger:       logger.With().Str("component", "sqs_worker").Logger(),
		awsClient:    awsClient,
		jiraClient:   jiraClient,
		trunkClient:  trunkClient,
		githubClient: githubClient,
		metrics:      metrics,
		pollInterval: config.PollInterval,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start begins the background worker process.
func (w *Worker) Start() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.running {
		return fmt.Errorf("worker is already running")
	}

	w.logger.Info().
		Str("poll_interval", w.pollInterval.String()).
		Msg("Starting SQS worker")

	w.running = true
	w.wg.Add(1)

	go w.run()

	return nil
}

// Stop gracefully stops the worker and waits for current processing to complete.
func (w *Worker) Stop() error {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return nil
	}
	w.running = false
	w.mu.Unlock()

	w.logger.Info().Msg("Stopping SQS worker")

	// Cancel the context to signal shutdown
	w.cancel()

	// Wait for the worker goroutine to finish
	w.wg.Wait()

	w.logger.Info().Msg("SQS worker stopped")
	return nil
}

// IsRunning returns whether the worker is currently running.
func (w *Worker) IsRunning() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.running
}

// run is the main worker loop that polls SQS for messages.
func (w *Worker) run() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			w.logger.Debug().Msg("Worker context cancelled, stopping")
			return
		case <-ticker.C:
			w.pollAndProcess()
		}
	}
}

// pollAndProcess polls SQS for messages and processes them.
func (w *Worker) pollAndProcess() {
	pollStart := time.Now()
	w.logger.Trace().Msg("Polling SQS for messages")

	// Create a timeout context for this poll operation
	pollCtx, cancel := context.WithTimeout(w.ctx, 30*time.Second)
	defer cancel()

	// Record poll interval metrics
	w.metrics.RecordWorkerPollInterval(w.ctx, time.Since(pollStart))

	// Receive messages from SQS
	result, err := w.awsClient.ReceiveMessageFromQueue(pollCtx, w.logger)
	if err != nil {
		w.logger.Error().
			Err(err).
			Str("operation", opReceiveMessages).
			Msg("Failed to receive messages from SQS")
		return
	}

	messageCount := len(result.Messages)
	if messageCount == 0 {
		w.logger.Trace().Msg("No messages to process")
		return
	}

	// Record batch size metrics
	w.metrics.RecordSQSReceiveBatchSize(w.ctx, int64(messageCount))

	// Process each message
	for _, message := range result.Messages {
		w.processMessage(pollCtx, message)
	}
}

// processMessage processes a single SQS message.
func (w *Worker) processMessage(ctx context.Context, message types.Message) {
	start := time.Now()

	if message.Body == nil || message.ReceiptHandle == nil {
		w.logger.Warn().Msg("Received message with nil body or receipt handle")
		w.metrics.IncWorkerMessage(ctx, "unknown", "invalid_message")
		return
	}

	messageBody := *message.Body
	receiptHandle := *message.ReceiptHandle

	l := w.logger.With().
		Str("message_id", getStringPtr(message.MessageId)).
		Str("receipt_handle_prefix", truncateString(receiptHandle, 20)).
		Int("message_size", len(messageBody)).
		Logger()

	l.Info().Msg("Processing SQS message")

	// Process the webhook payload directly
	err := w.processWebhookPayload(messageBody)
	if err != nil {
		l.Error().
			Err(err).
			Str("operation", opProcessWebhook).
			Msg("Failed to process webhook payload")
		w.metrics.IncWorkerMessage(ctx, "trunk_webhook", "processing_failed")
		w.metrics.RecordWorkerProcessingDuration(ctx, "trunk_webhook", time.Since(start))
		// Note: In a production system, you might want to implement retry logic
		// or send messages to a dead letter queue instead of just deleting them
		return
	}

	// Delete the message from SQS after successful processing
	err = w.awsClient.DeleteMessageFromQueue(ctx, l, receiptHandle)
	if err != nil {
		l.Error().
			Err(err).
			Str("operation", opDeleteMessage).
			Msg("Failed to delete message from SQS after processing")
		w.metrics.IncSQSMessageDelete(ctx, "failure")
		// Message will become visible again after visibility timeout
		return
	}

	// Record success metrics
	w.metrics.IncSQSMessageDelete(ctx, "success")
	w.metrics.IncWorkerMessage(ctx, "trunk_webhook", "processed")
	w.metrics.RecordWorkerProcessingDuration(ctx, "trunk_webhook", time.Since(start))

	l.Info().Msg("Successfully processed and deleted SQS message")
}

// getStringPtr safely dereferences a string pointer, returning empty string if nil.
func getStringPtr(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}

// truncateString truncates a string to maxLen characters (rune-safe), adding "..." if truncated.
func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// processWebhookPayload processes a webhook payload that came from SQS.
func (w *Worker) processWebhookPayload(payload string) error {
	if err := w.verifyClients(); err != nil {
		return err
	}

	l := w.logger.With().
		Int("payload_size", len(payload)).
		Logger()

	l.Debug().
		Str("payload_sample", truncateString(payload, 200)).
		Msg("Processing webhook payload from SQS")

	var webhookData trunk.TestCaseStatusChange
	if err := json.Unmarshal([]byte(payload), &webhookData); err != nil {
		l.Error().
			Err(err).
			Str("payload_sample", truncateString(payload, 200)).
			Msg("Failed to parse test_case.status_changed payload from SQS")
		return fmt.Errorf("failed to parse test_case.status_changed payload: %w", err)
	}

	testCase := webhookData.TestCase
	currentStatus := webhookData.StatusChange.CurrentStatus.Value

	// Create enriched logger with all test context that will be used throughout processing
	enrichedLogger := l.With().
		Str("test_id", testCase.ID).
		Str("test_name", testCase.Name).
		Str("test_suite", testCase.TestSuite).
		Str("test_repo_url", testCase.Repository.HTMLURL).
		Str("test_file_path", testCase.FilePath).
		Str("test_current_status", currentStatus).
		Str("test_previous_status", webhookData.StatusChange.PreviousStatus).
		Logger()

	return w.handleTestCaseStatusChanged(enrichedLogger, webhookData)
}

// handleTestCaseStatusChanged processes when a test case's status changes.
func (w *Worker) handleTestCaseStatusChanged(l zerolog.Logger, statusChange trunk.TestCaseStatusChange) error {
	currentStatus := statusChange.StatusChange.CurrentStatus.Value

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
func (w *Worker) handleFlakyTest(l zerolog.Logger, statusChange trunk.TestCaseStatusChange) error {
	start := time.Now()
	testCase := statusChange.TestCase

	l.Debug().Msg("Quarantining flaky test")

	// Record flaky test detection
	w.metrics.IncFlakyTestDetected(context.Background(), testCase.Name, testCase.TestSuite)

	// Create a Jira ticket for the flaky test
	issue, err := w.createJiraIssueForFlakyTest(l, statusChange)
	if err != nil {
		l.Error().
			Err(err).
			Str("operation", opCreateJiraTicket).
			Msg("Failed to create or update Jira ticket for flaky test")
		return fmt.Errorf("failed to create Jira ticket: %w", err)
	}

	l.Info().
		Str("operation", opCreateJiraTicket).
		Str("jira_issue_key", issue.Key).
		Msg("Successfully created or updated Jira ticket for flaky test")

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
		l.Error().
			Err(err).
			Str("operation", opQuarantineTest).
			Str("test_package", testCase.TestSuite).
			Str("test_name", testCase.Name).
			Msg("Failed to quarantine test")
		return fmt.Errorf("failed to quarantine test: %w", err)
	}

	l.Info().
		Str("operation", opQuarantineTest).
		Msg("Successfully quarantined flaky test")

	// Record time to quarantine
	quarantineDuration := time.Since(start)
	w.metrics.RecordTimeToQuarantine(context.Background(), quarantineDuration)

	l.Info().
		Dur("quarantine_duration", quarantineDuration).
		Msg("Completed flaky test processing")

	return nil
}

// handleBrokenTest handles the case where a test is marked as broken.
func (w *Worker) handleBrokenTest(l zerolog.Logger, statusChange trunk.TestCaseStatusChange) error {
	return w.handleFlakyTest(l, statusChange)
}

// handleHealthyTest handles the case where a test is marked as healthy.
func (w *Worker) handleHealthyTest(l zerolog.Logger, statusChange trunk.TestCaseStatusChange) error {
	testCase := statusChange.TestCase

	// Record test recovery if it was previously flaky
	if statusChange.StatusChange.PreviousStatus == trunk.TestCaseStatusFlaky {
		w.metrics.IncTestRecovered(context.Background())
		l.Info().Msg("Test recovered from flaky status")
	}

	// Look for an existing open ticket for this test
	issue, err := w.jiraClient.GetOpenFlakyTestIssue(testCase.TestSuite, testCase.Name)
	if errors.Is(err, jira.ErrNoOpenFlakyTestIssueFound) {
		// No open ticket found - this is normal for healthy tests
		l.Debug().
			Str("operation", opGetOpenTicket).
			Msg("No open flaky test ticket found for healthy test (expected)")
		return nil
	} else if err != nil {
		l.Error().
			Err(err).
			Str("operation", opGetOpenTicket).
			Str("test_suite", testCase.TestSuite).
			Str("test_name", testCase.Name).
			Msg("Failed to check for existing Jira ticket")
		return fmt.Errorf("failed to check for existing Jira ticket: %w", err)
	}

	l.Info().
		Str("operation", opGetOpenTicket).
		Str("jira_issue_key", issue.Key).
		Msg("Found open flaky test ticket for healthy test - will close it")

	// There's an open ticket - close it with a comprehensive comment about the test being healthy
	closeComment := `*Test Status Update: HEALTHY* ✅ - *Automatically Closing Ticket*

The test has recovered and is now healthy! This ticket is being automatically closed.

*Status Change:* %s → %s
*Failure Rate (Last 7d):* %g%%
*Pull Requests Impacted (Last 7d):* %d
*Test URL:* %s

If the test becomes flaky again, a new comment will be added or a new ticket will be created as needed.

This action was automatically performed by [branch-out|https://github.com/smartcontractkit/branch-out].`

	// Format the comment with status change details
	currentStatus := statusChange.StatusChange.CurrentStatus.Value
	previousStatus := statusChange.StatusChange.PreviousStatus

	formattedComment := fmt.Sprintf(closeComment,
		previousStatus,
		currentStatus,
		testCase.FailureRateLast7D,
		testCase.PullRequestsImpactedLast7D,
		testCase.HTMLURL,
	)

	err = w.jiraClient.CloseIssue(issue.Key, formattedComment)
	if err != nil {
		l.Warn().
			Err(err).
			Str("operation", opCloseTicket).
			Str("jira_issue_key", issue.Key).
			Msg("Failed to close Jira ticket for healthy test (non-blocking)")
		// Don't fail the whole operation if closing fails
	} else {
		l.Info().
			Str("operation", opCloseTicket).
			Str("jira_issue_key", issue.Key).
			Msg("Successfully closed Jira ticket for recovered test")
	}

	return nil
}

// verifyClients verifies that all the clients are not nil.
func (w *Worker) verifyClients() error {
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

// createJiraIssueForFlakyTest looks for an existing open ticket or creates a new one.
func (w *Worker) createJiraIssueForFlakyTest(
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
		l.Error().
			Err(err).
			Str("operation", opCreateJiraTicket).
			Msg("Failed to marshal test case details for Jira ticket")
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
		l.Info().Msg("No existing Jira ticket found, creating new one")
		issue, err = w.jiraClient.CreateFlakyTestIssue(req)
		if err != nil {
			l.Error().
				Err(err).
				Str("operation", opCreateJiraTicket).
				Msg("Failed to create Jira ticket")
			return jira.FlakyTestIssue{}, fmt.Errorf("failed to create Jira ticket: %w", err)
		}
		l.Info().
			Str("jira_issue_key", issue.Key).
			Str("operation", opCreateJiraTicket).
			Msg("Successfully created new Jira ticket")
	} else if err != nil {
		// Some other error occurred
		l.Error().
			Err(err).
			Str("operation", opGetOpenTicket).
			Msg("Failed to get existing Jira ticket")
		return jira.FlakyTestIssue{}, fmt.Errorf("failed to get existing Jira ticket: %w", err)
	} else {
		// Existing open ticket found
		l.Info().
			Str("jira_issue_key", issue.Key).
			Str("operation", opCreateJiraTicket).
			Msg("Found existing open Jira ticket, will add comment")
	}

	// Add a comment with the current status details (for both new and existing tickets)
	err = w.jiraClient.AddCommentToFlakyTestIssue(issue, statusChange)
	if err != nil {
		l.Warn().
			Err(err).
			Str("operation", opAddJiraComment).
			Str("jira_issue_key", issue.Key).
			Msg("Failed to add comment to Jira ticket (non-blocking)")
	} else {
		l.Debug().
			Str("jira_issue_key", issue.Key).
			Str("operation", opAddJiraComment).
			Msg("Successfully added status comment to Jira ticket")
	}

	// Link the Jira ticket back to the Trunk test case
	if err := w.trunkClient.LinkTicketToTestCase(testCase.ID, issue.Key, testCase.Repository.HTMLURL); err != nil {
		l.Warn().
			Err(err).
			Str("operation", opLinkToTrunk).
			Str("jira_issue_key", issue.Key).
			Msg("Failed to link Jira ticket to Trunk test case (non-blocking)")
	} else {
		l.Debug().
			Str("jira_issue_key", issue.Key).
			Str("operation", opLinkToTrunk).
			Msg("Successfully linked Jira ticket to Trunk test case")
	}

	return issue, nil
}

// ProcessWebhookPayload is a standalone function for processing webhook payloads.
// This is used by both the worker and CLI commands.
func ProcessWebhookPayload(
	logger zerolog.Logger,
	jiraClient JiraClient,
	trunkClient TrunkClient,
	githubClient GithubClient,
	payload string,
) error {
	// Create a temporary worker-like struct to reuse the existing methods
	w := &Worker{
		logger:       logger.With().Str("component", "webhook_processor").Logger(),
		jiraClient:   jiraClient,
		trunkClient:  trunkClient,
		githubClient: githubClient,
	}

	return w.processWebhookPayload(payload)
}
