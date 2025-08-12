// Package processing provides background processing functionality for SQS messages.
package processing

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/rs/zerolog"

	"github.com/smartcontractkit/branch-out/telemetry"
)

// Worker handles background processing of SQS messages and webhook business logic.
type Worker struct {
	logger           zerolog.Logger
	awsClient        AWSClient
	webhookProcessor WebhookProcessor
	metrics          *telemetry.Metrics

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
	jiraClient JiraClient, // for webhook_processor
	trunkClient TrunkClient, // for webhook_processor
	githubClient GithubClient, // for webhook_processor
	metrics *telemetry.Metrics,
	config Config,
) *Worker {
	ctx, cancel := context.WithCancel(context.Background())

	if config.PollInterval == 0 {
		config.PollInterval = 5 * time.Second // Default poll interval
	}

	webhookProcessor := NewWebhookProcessor(
		logger,
		jiraClient,
		trunkClient,
		githubClient,
		metrics,
	)

	return &Worker{
		logger:           logger.With().Str("component", "sqs_worker").Logger(),
		awsClient:        awsClient,
		webhookProcessor: *webhookProcessor,
		metrics:          metrics,
		pollInterval:     config.PollInterval,
		ctx:              ctx,
		cancel:           cancel,
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
		w.logger.Error().Err(err).Msg("Failed to receive messages from SQS")
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
		Str("message_id", deref(message.MessageId)).
		Str("receipt_handle", receiptHandle[:20]+"..."). // Log partial receipt handle for debugging
		Logger()

	l.Info().Msg("Processing SQS message")

	// Process the webhook payload directly
	err := w.webhookProcessor.ProcessWebhookPayload(messageBody)
	if err != nil {
		l.Error().Err(err).Msg("Failed to process webhook payload")
		w.metrics.IncWorkerMessage(ctx, "trunk_webhook", "processing_failed")
		w.metrics.RecordWorkerProcessingDuration(ctx, "trunk_webhook", time.Since(start))
		// Note: In a production system, you might want to implement retry logic
		// or send messages to a dead letter queue instead of just deleting them
		return
	}

	// Delete the message from SQS after successful processing
	err = w.awsClient.DeleteMessageFromQueue(ctx, l, receiptHandle)
	if err != nil {
		l.Error().Err(err).Msg("Failed to delete message from SQS after processing")
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

// deref safely dereferences a pointer, returning a zero value if nil.
func deref[T any](ptr *T) T {
	if ptr == nil {
		return *new(T)
	}
	return *ptr
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
	webhookProcessor := NewWebhookProcessor(
		logger,
		jiraClient,
		trunkClient,
		githubClient,
		nil,
	)

	// Create a temporary worker-like struct to reuse the existing methods
	w := &Worker{
		webhookProcessor: *webhookProcessor,
	}
	return w.webhookProcessor.ProcessWebhookPayload(payload)
}
