package processing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog"

	"github.com/smartcontractkit/branch-out/telemetry"
	"github.com/smartcontractkit/branch-out/trunk"

	svix "github.com/svix/svix-webhooks/go"
)

// VerifyAndEnqueueWebhook processes an incoming webhook by validating it and queuing for async processing.
func VerifyAndEnqueueWebhook(
	logger zerolog.Logger,
	signingSecret string,
	awsClient AWSClient,
	metrics *telemetry.Metrics,
	req *http.Request,
) error {
	start := time.Now()
	ctx := req.Context()

	// Record webhook received
	metrics.IncWebhook(ctx, "trunk", "received")

	// Verify the webhook signature
	if err := verifyWebhookRequest(logger, req, signingSecret); err != nil {
		metrics.IncWebhookValidationFailure(ctx, "signature_verification")
		return fmt.Errorf("webhook call cannot be verified: %w", err)
	}

	// Read and validate payload
	payload, err := io.ReadAll(req.Body)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to read request body")
		return fmt.Errorf("failed to read request body: %w", err)
	}
	defer func() {
		if err := req.Body.Close(); err != nil {
			logger.Error().Err(err).Msg("Failed to close request body")
		}
	}()

	logger.Debug().Str("payload", string(payload)).Msg("Raw webhook payload")

	// Validate payload structure (Trunk-specific validation)
	var webhookData trunk.TestCaseStatusChange
	if err := json.Unmarshal(payload, &webhookData); err != nil {
		metrics.IncWebhookValidationFailure(ctx, "json_parsing")
		logger.Error().
			Err(err).
			Str("payload", string(payload)).
			Msg("Failed to parse test_case.status_changed payload")
		return fmt.Errorf("failed to parse test_case.status_changed payload: %w", err)
	}

	l := logger.With().
		Str("id", webhookData.TestCase.ID).
		Str("name", webhookData.TestCase.Name).
		Str("current_status", webhookData.StatusChange.CurrentStatus.Value).
		Str("previous_status", webhookData.StatusChange.PreviousStatus).
		Logger()

	// Push to SQS for async processing
	sqsStart := time.Now()
	err = awsClient.PushMessageToQueue(
		context.Background(),
		l,
		string(payload),
	)
	if err != nil {
		metrics.IncWebhook(ctx, "trunk", "sqs_failed")
		l.Error().Err(err).Msg("Failed to push webhook payload to AWS SQS")
		return fmt.Errorf("failed to push webhook payload to AWS SQS: %w", err)
	}

	// Record metrics for successful processing
	metrics.RecordSQSSendLatency(ctx, time.Since(sqsStart))
	metrics.IncWebhook(ctx, "trunk", "processed")
	metrics.RecordWebhookDuration(ctx, "trunk", time.Since(start))

	l.Info().Msg("Webhook payload successfully queued for processing")
	return nil
}

// verifyWebhookRequest verifies a request as a valid svix webhook call.
// https://docs.svix.com/receiving/verifying-payloads/how
func verifyWebhookRequest(l zerolog.Logger, req *http.Request, signingSecret string) error {
	wh, err := svix.NewWebhook(signingSecret)
	if err != nil {
		return fmt.Errorf("bad signing secret: %w", err)
	}

	payload, err := io.ReadAll(req.Body)
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}
	defer func() {
		if err := req.Body.Close(); err != nil {
			l.Error().Err(err).Msg("Failed to close request body")
		}
	}()
	req.Body = io.NopCloser(bytes.NewBuffer(payload))

	return wh.Verify(payload, req.Header)
}

// SelfSignWebhookRequest self-signs a request to create a valid svix webhook call.
// This is useful for testing.
func SelfSignWebhookRequest(l zerolog.Logger, req *http.Request, signingSecret string) (*http.Request, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}

	if req.Body == nil {
		return nil, fmt.Errorf("request body is nil")
	}

	// Create svix webhook for signing
	wh, err := svix.NewWebhook(signingSecret)
	if err != nil {
		return nil, fmt.Errorf("bad signing secret: %w", err)
	}

	if req.Header == nil {
		req.Header = make(http.Header)
	}

	// Generate headers (svix will add the signature)
	req.Header.Set("webhook-id", "self_signed_webhook_id")
	req.Header.Set("webhook-timestamp", fmt.Sprintf("%d", time.Now().Unix()))

	payload, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}
	defer func() {
		if err := req.Body.Close(); err != nil {
			l.Error().Err(err).Msg("Failed to close request body")
		}
	}()
	req.Body = io.NopCloser(bytes.NewBuffer(payload))

	// Sign the payload
	signature, err := wh.Sign(req.Header.Get("webhook-id"), time.Now(), payload)
	if err != nil {
		return nil, fmt.Errorf("failed to sign webhook payload: %w", err)
	}
	req.Header.Set("webhook-signature", signature)

	return req, nil
}
