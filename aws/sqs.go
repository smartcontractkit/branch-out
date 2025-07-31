package aws

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// PushMessageToQueue sends a message to the configured SQS queue.
func (c *Client) PushMessageToQueue(
	ctx context.Context,
	l zerolog.Logger,
	payload string) error {
	start := time.Now()

	if c.sqsClient == nil {
		l.Error().Msg("SQS client is not initialized")
		c.metrics.IncSQSOperations(ctx, "send_failed")
		return fmt.Errorf("SQS client is not initialized")
	}

	if payload == "" {
		l.Error().Msg("Message payload cannot be empty")
		c.metrics.IncSQSOperations(ctx, "send_failed")
		return fmt.Errorf("message payload cannot be empty")
	}

	// Log the queue URL for debugging
	l.Debug().Str("queue_url", c.queueURL).Msg("Attempting to send message to SQS queue")

	// Create the message to send to the SQS queue
	message := &sqs.SendMessageInput{
		MessageBody: &payload,
		QueueUrl:    &c.queueURL,
	}

	// Check if this is a FIFO queue and add required parameters
	if strings.HasSuffix(c.queueURL, ".fifo") {
		// Use a simple message group ID for FIFO queues
		messageGroupID := "branch-out-messages"
		message.MessageGroupId = &messageGroupID

		// Add deduplication ID to prevent duplicate messages
		// Using a hash of the payload to ensure identical messages are deduplicated
		hasher := sha256.New()
		hasher.Write([]byte(payload))
		deduplicationID := fmt.Sprintf("%x", hasher.Sum(nil))[:64] // AWS limit is 128 chars, using 64 for safety
		message.MessageDeduplicationId = &deduplicationID

		l.Debug().
			Str("message_group_id", messageGroupID).
			Str("deduplication_id", deduplicationID).
			Msg("Added FIFO queue parameters")
	}

	// Send the message to the SQS queue
	res, err := c.sqsClient.SendMessage(ctx, message)
	if err != nil {
		c.metrics.RecordSQSSendLatency(ctx, time.Since(start))
		c.metrics.IncSQSOperations(ctx, "send_failed")
		l.Error().Err(err).Msg("Failed to send message to SQS queue")
		return fmt.Errorf("failed to send message to SQS queue: %w", err)
	}

	// Record success metrics
	c.metrics.RecordSQSSendLatency(ctx, time.Since(start))
	c.metrics.IncSQSOperations(ctx, "send_success")

	// Handle potential nil pointers in response
	messageID := "unknown"
	if res.MessageId != nil {
		messageID = *res.MessageId
	}

	logEvent := l.Info().Str("MessageId", messageID)

	// SequenceNumber is only present for FIFO queues
	if res.SequenceNumber != nil {
		logEvent = logEvent.Str("SequenceNumber", *res.SequenceNumber)
	}

	logEvent.Msg("Message sent to SQS queue successfully")
	return nil
}

// ReceiveMessageFromQueue receives a message from the configured SQS queue.
func (c *Client) ReceiveMessageFromQueue(
	ctx context.Context,
	l zerolog.Logger,
) (*sqs.ReceiveMessageOutput, error) {
	if c.sqsClient == nil {
		l.Error().Msg("SQS client is not initialized")
		c.metrics.IncSQSOperations(ctx, "receive_failed")
		return nil, fmt.Errorf("SQS client is not initialized")
	}

	l.Debug().Str("queue_url", c.queueURL).Msg("Attempting to receive messages from SQS queue")

	// Receive messages from the SQS queue
	res, err := c.sqsClient.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		MaxNumberOfMessages: 1,
		QueueUrl:            &c.queueURL,
	})
	if err != nil {
		c.metrics.IncSQSOperations(ctx, "receive_failed")
		l.Error().Err(err).Msg("Failed to receive messages from SQS queue")
		return nil, fmt.Errorf("failed to receive messages from SQS queue: %w", err)
	}

	// Record batch size metrics
	c.metrics.RecordSQSReceiveBatchSize(ctx, int64(len(res.Messages)))
	c.metrics.IncSQSOperations(ctx, "receive_success")

	if len(res.Messages) == 0 {
		l.Debug().Msg("No messages received from SQS queue")
		return res, nil
	}

	l.Info().Int("num_messages", len(res.Messages)).Msg("Received messages from SQS queue")
	return res, nil
}

// DeleteMessageFromQueue deletes a message from the configured SQS queue.
func (c *Client) DeleteMessageFromQueue(
	ctx context.Context,
	l zerolog.Logger,
	receiptHandle string,
) error {
	if c.sqsClient == nil {
		l.Error().Msg("SQS client is not initialized")
		return fmt.Errorf("SQS client is not initialized")
	}

	if receiptHandle == "" {
		l.Error().Msg("Receipt handle cannot be empty")
		return fmt.Errorf("receipt handle cannot be empty")
	}

	l.Debug().Str("queue_url", c.queueURL).Msg("Attempting to delete message from SQS queue")

	// Delete the message from the SQS queue
	_, err := c.sqsClient.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      &c.queueURL,
		ReceiptHandle: &receiptHandle,
	})
	if err != nil {
		l.Error().Err(err).Msg("Failed to delete message from SQS queue")
		return fmt.Errorf("failed to delete message from SQS queue: %w", err)
	}

	l.Info().Msg("Message deleted from SQS queue successfully")
	return nil
}
