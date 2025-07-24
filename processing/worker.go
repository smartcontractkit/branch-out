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
	"github.com/smartcontractkit/branch-out/trunk"
)

// Worker handles background processing of SQS messages and webhook business logic.
type Worker struct {
	logger       zerolog.Logger
	awsClient    AWSClient
	jiraClient   JiraClient
	trunkClient  TrunkClient
	githubClient GithubClient

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

// NewWorker creates a new background worker for processing SQS messages.
func NewWorker(
	logger zerolog.Logger,
	awsClient AWSClient,
	jiraClient JiraClient,
	trunkClient TrunkClient,
	githubClient GithubClient,
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
	w.logger.Trace().Msg("Polling SQS for messages")

	// Create a timeout context for this poll operation
	pollCtx, cancel := context.WithTimeout(w.ctx, 30*time.Second)
	defer cancel()

	// Receive messages from SQS
	result, err := w.awsClient.ReceiveMessageFromQueue(pollCtx, w.logger)
	if err != nil {
		w.logger.Error().Err(err).Msg("Failed to receive messages from SQS")
		return
	}

	if len(result.Messages) == 0 {
		w.logger.Trace().Msg("No messages to process")
		return
	}

	// Process each message
	for _, message := range result.Messages {
		w.processMessage(pollCtx, message)
	}
}

// processMessage processes a single SQS message.
func (w *Worker) processMessage(ctx context.Context, message types.Message) {
	if message.Body == nil || message.ReceiptHandle == nil {
		w.logger.Warn().Msg("Received message with nil body or receipt handle")
		return
	}

	messageBody := *message.Body
	receiptHandle := *message.ReceiptHandle

	l := w.logger.With().
		Str("message_id", getStringPtr(message.MessageId)).
		Str("receipt_handle", receiptHandle[:20]+"..."). // Log partial receipt handle for debugging
		Logger()

	l.Info().Msg("Processing SQS message")

	// Process the webhook payload directly
	err := w.processWebhookPayload(messageBody)
	if err != nil {
		l.Error().Err(err).Msg("Failed to process webhook payload")
		// Note: In a production system, you might want to implement retry logic
		// or send messages to a dead letter queue instead of just deleting them
		return
	}

	// Delete the message from SQS after successful processing
	err = w.awsClient.DeleteMessageFromQueue(ctx, l, receiptHandle)
	if err != nil {
		l.Error().Err(err).Msg("Failed to delete message from SQS after processing")
		// Message will become visible again after visibility timeout
		return
	}

	l.Info().Msg("Successfully processed and deleted SQS message")
}

// getStringPtr safely dereferences a string pointer, returning empty string if nil.
func getStringPtr(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}

// processWebhookPayload processes a webhook payload that came from SQS.
func (w *Worker) processWebhookPayload(payload string) error {
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

// handleTestCaseStatusChanged processes when a test case's status changes.
func (w *Worker) handleTestCaseStatusChanged(l zerolog.Logger, statusChange trunk.TestCaseStatusChange) error {
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
func (w *Worker) handleFlakyTest(l zerolog.Logger, statusChange trunk.TestCaseStatusChange) error {
	testCase := statusChange.TestCase

	l.Debug().Msg("Quarantining flaky test")

	// Create a Jira ticket for the flaky test
	_, err := w.createJiraIssueForFlakyTest(l, statusChange)
	if err != nil {
		return fmt.Errorf("failed to create Jira ticket: %w", err)
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

	return nil
}

// handleBrokenTest handles the case where a test is marked as broken.
func (w *Worker) handleBrokenTest(l zerolog.Logger, statusChange trunk.TestCaseStatusChange) error {
	return w.handleFlakyTest(l, statusChange)
}

// handleHealthyTest handles the case where a test is marked as healthy.
func (w *Worker) handleHealthyTest(_ zerolog.Logger, _ trunk.TestCaseStatusChange) error {
	return fmt.Errorf("healthy test handling not implemented")
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
		return jira.FlakyTestIssue{}, fmt.Errorf("failed to marshal test case details: %w", err)
	}

	req := jira.FlakyTestIssueRequest{
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
