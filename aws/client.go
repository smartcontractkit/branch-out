// Package aws provides an AWS client for interacting with AWS services.
package aws

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"

	"github.com/smartcontractkit/branch-out/config"
	"github.com/smartcontractkit/branch-out/telemetry"

	aws_config "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// SQSAPIError provides enhanced error information for SQS operations,
// allowing callers to handle errors with appropriate business context and logging.
type SQSAPIError struct {
	Operation  string // The operation being performed (e.g., "send_message", "receive_message", "delete_message")
	QueueURL   string // The SQS queue URL involved in the operation
	StatusCode int    // HTTP status code if available
	Underlying error  // The underlying error that occurred
}

func (e *SQSAPIError) Error() string {
	if e.StatusCode != 0 {
		return fmt.Sprintf("AWS %s operation failed (status %d): %v", e.Operation, e.StatusCode, e.Underlying)
	}
	return fmt.Sprintf("AWS %s operation failed: %v", e.Operation, e.Underlying)
}

// Client is the collection of AWS clients used by the application.
type Client struct {
	awsConfig aws_config.Config
	queueURL  string
	sqsClient *sqs.Client
	logger    zerolog.Logger
	metrics   *telemetry.Metrics
}

// ClientOption is a function that can be used to configure the AWS client.
type ClientOption func(*clientOptions)

type clientOptions struct {
	region   string
	queueURL string
	logger   zerolog.Logger
	metrics  *telemetry.Metrics
}

// WithConfig sets the AWS region and SQS queue URL from the provided config.
func WithConfig(config config.Config) ClientOption {
	return func(c *clientOptions) {
		c.region = config.Aws.Region
		c.queueURL = config.Aws.SqsQueueURL
	}
}

// WithLogger sets the logger to use for the AWS client.
func WithLogger(logger zerolog.Logger) ClientOption {
	return func(opts *clientOptions) {
		opts.logger = logger
	}
}

// WithMetrics sets the metrics instance for the AWS client.
func WithMetrics(metrics *telemetry.Metrics) ClientOption {
	return func(opts *clientOptions) {
		opts.metrics = metrics
	}
}

// NewClient creates a new AWS client with configuration from the provided options.
func NewClient(options ...ClientOption) (*Client, error) {
	clientOptions := &clientOptions{
		logger: zerolog.Nop(),
	}

	for _, option := range options {
		option(clientOptions)
	}

	l := clientOptions.logger.With().
		Str("aws_region", clientOptions.region).
		Str("sqs_queue_url", clientOptions.queueURL).
		Logger()

	// Add debug logging for configuration values
	l.Debug().Msg("Initializing AWS client with configuration")

	if clientOptions.region == "" {
		return nil, fmt.Errorf("AWS region is required")
	}

	if clientOptions.queueURL == "" {
		return nil, fmt.Errorf("SQS queue URL is required")
	}

	cfg, err := aws_config.LoadDefaultConfig(context.Background(), aws_config.WithRegion(clientOptions.region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Log credential information for debugging
	creds, err := cfg.Credentials.Retrieve(context.Background())
	if err != nil {
		l.Error().Err(err).Msg("Failed to retrieve AWS credentials")
	} else {
		l.Debug().
			Str("access_key_id", creds.AccessKeyID[:8]+"...").
			Bool("has_session_token", creds.SessionToken != "").
			Str("source", creds.Source).
			Msg("AWS credentials loaded")
	}

	svc := sqs.NewFromConfig(cfg)

	client := &Client{
		sqsClient: svc,
		awsConfig: cfg,
		queueURL:  clientOptions.queueURL,
		logger:    l,
		metrics:   clientOptions.metrics,
	}

	return client, nil
}
