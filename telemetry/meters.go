package telemetry

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	httpMeter       = otel.Meter("branch-out/http")
	githubMeter     = otel.Meter("branch-out/github")
	awsMeter        = otel.Meter("branch-out/aws")
	jiraMeter       = otel.Meter("branch-out/jira")
	webhookMeter    = otel.Meter("branch-out/webhook")
	workerMeter     = otel.Meter("branch-out/worker")
	quarantineMeter = otel.Meter("branch-out/quarantine")
	trunkMeter      = otel.Meter("branch-out/trunk")
)

// IncSQSOperations increments the AWS SQS operations counter with the specified operation.
// It uses the provided context to add the operation as an attribute.
func (m *Metrics) IncSQSOperations(ctx context.Context, operation string) {
	counter, _ := awsMeter.Int64Counter("aws.sqs.operations",
		metric.WithDescription("Count of AWS SQS operations"),
		metric.WithUnit("1"))
	counter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("operation", operation),
	))
}

// IncHTTPRequest increments the HTTP operations counter with the specified operation.
// It uses the provided context to add the operation as an attribute.
func (m *Metrics) IncHTTPRequest(ctx context.Context, status int, method, uri string) {
	counter, _ := httpMeter.Int64Counter("http.requests",
		metric.WithDescription("Count of HTTP requests"),
		metric.WithUnit("1"))
	counter.Add(ctx, 1, metric.WithAttributes(
		attribute.Int("status", status),
		attribute.String("method", method),
		attribute.String("uri", uri),
	))
}

// IncHTTPError increments the HTTP error counter with the specified status code.
// It uses the provided context to add the status code as an attribute.
func (m *Metrics) IncHTTPError(ctx context.Context, status int) {
	counter, _ := httpMeter.Int64Counter("http.errors",
		metric.WithDescription("Count of HTTP errors"),
		metric.WithUnit("1"))
	counter.Add(ctx, 1, metric.WithAttributes(
		attribute.Int("status", status),
	))
}

// Webhook Processing Metrics

// IncWebhook increments the webhook counter by type and status.
func (m *Metrics) IncWebhook(ctx context.Context, webhookType, status string) {
	counter, _ := webhookMeter.Int64Counter("webhook.received",
		metric.WithDescription("Count of webhooks received"),
		metric.WithUnit("1"))
	counter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("type", webhookType),
		attribute.String("status", status),
	))
}

// RecordWebhookDuration records the time taken to process a webhook.
func (m *Metrics) RecordWebhookDuration(ctx context.Context, webhookType string, duration time.Duration) {
	histogram, _ := webhookMeter.Float64Histogram("webhook.duration",
		metric.WithDescription("Duration of webhook processing"),
		metric.WithUnit("ms"))
	histogram.Record(ctx, duration.Seconds()*1000, metric.WithAttributes(
		attribute.String("type", webhookType),
	))
}

// IncWebhookValidationFailure increments webhook validation failures.
func (m *Metrics) IncWebhookValidationFailure(ctx context.Context, reason string) {
	counter, _ := webhookMeter.Int64Counter("webhook.validation.failures",
		metric.WithDescription("Count of webhook validation failures"),
		metric.WithUnit("1"))
	counter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("reason", reason),
	))
}

// Worker Processing Metrics

// IncWorkerMessage increments the worker message counter by type and status.
func (m *Metrics) IncWorkerMessage(ctx context.Context, messageType, status string) {
	counter, _ := workerMeter.Int64Counter("worker.messages.processed",
		metric.WithDescription("Count of messages processed by worker"),
		metric.WithUnit("1"))
	counter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("type", messageType),
		attribute.String("status", status),
	))
}

// RecordWorkerProcessingDuration records the time taken to process a message.
func (m *Metrics) RecordWorkerProcessingDuration(ctx context.Context, messageType string, duration time.Duration) {
	histogram, _ := workerMeter.Float64Histogram("worker.processing.duration",
		metric.WithDescription("Duration of message processing"),
		metric.WithUnit("ms"))
	histogram.Record(ctx, duration.Seconds()*1000, metric.WithAttributes(
		attribute.String("type", messageType),
	))
}

// RecordWorkerPollInterval records the time between worker polls.
func (m *Metrics) RecordWorkerPollInterval(ctx context.Context, interval time.Duration) {
	histogram, _ := workerMeter.Float64Histogram("worker.poll.interval",
		metric.WithDescription("Time between SQS polls"),
		metric.WithUnit("s"))
	histogram.Record(ctx, interval.Seconds())
}

// Test Quarantine Metrics

// IncQuarantineOperation increments quarantine operations by package and result.
func (m *Metrics) IncQuarantineOperation(ctx context.Context, packageName, result string) {
	counter, _ := quarantineMeter.Int64Counter("quarantine.operations",
		metric.WithDescription("Count of test quarantine operations"),
		metric.WithUnit("1"))
	counter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("package", packageName),
		attribute.String("result", result),
	))
}

// RecordQuarantineFilesModified records the number of files modified in a quarantine operation.
func (m *Metrics) RecordQuarantineFilesModified(ctx context.Context, count int64) {
	histogram, _ := quarantineMeter.Int64Histogram("quarantine.files.modified",
		metric.WithDescription("Number of files modified per quarantine"),
		metric.WithUnit("1"))
	histogram.Record(ctx, count)
}

// RecordQuarantineDuration records the duration of a quarantine operation.
func (m *Metrics) RecordQuarantineDuration(ctx context.Context, duration time.Duration) {
	histogram, _ := quarantineMeter.Float64Histogram("quarantine.duration",
		metric.WithDescription("Duration of quarantine operation"),
		metric.WithUnit("ms"))
	histogram.Record(ctx, duration.Seconds()*1000)
}

// GitHub API Metrics

// RecordGitHubAPILatency records GitHub API call latency.
func (m *Metrics) RecordGitHubAPILatency(ctx context.Context, operation string, duration time.Duration) {
	histogram, _ := githubMeter.Float64Histogram("github.api.latency",
		metric.WithDescription("GitHub API call latency"),
		metric.WithUnit("ms"))
	histogram.Record(ctx, duration.Seconds()*1000, metric.WithAttributes(
		attribute.String("operation", operation),
	))
}

// IncGitHubRateLimitHit increments GitHub rate limit hits.
func (m *Metrics) IncGitHubRateLimitHit(ctx context.Context) {
	counter, _ := githubMeter.Int64Counter("github.rate.limit.hits",
		metric.WithDescription("Count of GitHub rate limit hits"),
		metric.WithUnit("1"))
	counter.Add(ctx, 1)
}

// Jira API Metrics

// RecordJiraAPILatency records Jira API call latency.
func (m *Metrics) RecordJiraAPILatency(ctx context.Context, operation string, duration time.Duration) {
	histogram, _ := jiraMeter.Float64Histogram("jira.api.latency",
		metric.WithDescription("Jira API call latency"),
		metric.WithUnit("ms"))
	histogram.Record(ctx, duration.Seconds()*1000, metric.WithAttributes(
		attribute.String("operation", operation),
	))
}

// IncJiraTicket increments Jira ticket operations.
func (m *Metrics) IncJiraTicket(ctx context.Context, action string) {
	counter, _ := jiraMeter.Int64Counter("jira.ticket.operations",
		metric.WithDescription("Count of Jira ticket operations"),
		metric.WithUnit("1"))
	counter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("action", action), // created, updated, found_existing
	))
}

// AWS SQS Metrics

// RecordSQSSendLatency records SQS message send latency.
func (m *Metrics) RecordSQSSendLatency(ctx context.Context, duration time.Duration) {
	histogram, _ := awsMeter.Float64Histogram("aws.sqs.send.latency",
		metric.WithDescription("SQS message send latency"),
		metric.WithUnit("ms"))
	histogram.Record(ctx, duration.Seconds()*1000)
}

// RecordSQSReceiveBatchSize records SQS receive batch size.
func (m *Metrics) RecordSQSReceiveBatchSize(ctx context.Context, size int64) {
	histogram, _ := awsMeter.Int64Histogram("aws.sqs.receive.batch.size",
		metric.WithDescription("SQS receive batch size"),
		metric.WithUnit("1"))
	histogram.Record(ctx, size)
}

// IncSQSMessageDelete increments SQS message delete operations.
func (m *Metrics) IncSQSMessageDelete(ctx context.Context, status string) {
	counter, _ := awsMeter.Int64Counter("aws.sqs.message.deletes",
		metric.WithDescription("Count of SQS message delete operations"),
		metric.WithUnit("1"))
	counter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("status", status), // success, failure
	))
}

// Trunk API Metrics

// RecordTrunkAPILatency records Trunk API call latency.
func (m *Metrics) RecordTrunkAPILatency(ctx context.Context, operation string, duration time.Duration) {
	histogram, _ := trunkMeter.Float64Histogram("trunk.api.latency",
		metric.WithDescription("Trunk API call latency"),
		metric.WithUnit("ms"))
	histogram.Record(ctx, duration.Seconds()*1000, metric.WithAttributes(
		attribute.String("operation", operation),
	))
}

// Business Metrics

// IncFlakyTestDetected increments flaky test detection counter.
func (m *Metrics) IncFlakyTestDetected(ctx context.Context, testName, packageName string) {
	counter, _ := quarantineMeter.Int64Counter("flaky.tests.detected",
		metric.WithDescription("Count of flaky tests detected"),
		metric.WithUnit("1"))
	counter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("test", testName),
		attribute.String("package", packageName),
	))
}

// RecordTimeToQuarantine records time from detection to quarantine.
func (m *Metrics) RecordTimeToQuarantine(ctx context.Context, duration time.Duration) {
	histogram, _ := quarantineMeter.Float64Histogram("flaky.time.to.quarantine",
		metric.WithDescription("Time from flaky detection to quarantine PR"),
		metric.WithUnit("s"))
	histogram.Record(ctx, duration.Seconds())
}

// IncTestRecovered increments test recovery counter.
func (m *Metrics) IncTestRecovered(ctx context.Context) {
	counter, _ := quarantineMeter.Int64Counter("flaky.tests.recovered",
		metric.WithDescription("Count of tests marked healthy after being flaky"),
		metric.WithUnit("1"))
	counter.Add(ctx, 1)
}
