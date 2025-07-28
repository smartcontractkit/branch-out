package telemetry

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestMetrics creates a metrics instance for testing
func setupTestMetrics(t *testing.T) (*Metrics, func()) {
	t.Helper()

	metrics, shutdown, err := NewMetrics(
		WithExporter("stdout"),
		WithContext(context.Background()),
	)
	require.NoError(t, err)
	require.NotNil(t, metrics)
	require.NotNil(t, shutdown)

	cleanup := func() {
		err := shutdown(context.Background())
		assert.NoError(t, err)
	}

	return metrics, cleanup
}

func TestIncSQSOperations(t *testing.T) {
	t.Parallel()
	metrics, cleanup := setupTestMetrics(t)
	defer cleanup()

	ctx := context.Background()

	operations := []string{"send", "receive", "delete"}

	for _, operation := range operations {
		assert.NotPanics(t, func() {
			metrics.IncSQSOperations(ctx, operation)
		})
	}
}

func TestIncHTTPRequest(t *testing.T) {
	t.Parallel()
	metrics, cleanup := setupTestMetrics(t)
	defer cleanup()

	ctx := context.Background()

	testCases := []struct {
		status int
		method string
		uri    string
	}{
		{200, "GET", "/webhook"},
		{201, "POST", "/api/v1/resource"},
		{404, "GET", "/not-found"},
		{500, "POST", "/error"},
	}

	for _, tc := range testCases {
		assert.NotPanics(t, func() {
			metrics.IncHTTPRequest(ctx, tc.status, tc.method, tc.uri)
		})
	}
}

func TestIncHTTPError(t *testing.T) {
	t.Parallel()
	metrics, cleanup := setupTestMetrics(t)
	defer cleanup()

	ctx := context.Background()

	errorStatuses := []int{400, 401, 403, 404, 500, 502, 503}

	for _, status := range errorStatuses {
		assert.NotPanics(t, func() {
			metrics.IncHTTPError(ctx, status)
		})
	}
}

func TestIncWebhook(t *testing.T) {
	t.Parallel()
	metrics, cleanup := setupTestMetrics(t)
	defer cleanup()

	ctx := context.Background()

	testCases := []struct {
		webhookType string
		status      string
	}{
		{"trunk", "received"},
		{"trunk", "processed"},
		{"trunk", "failed"},
		{"github", "received"},
	}

	for _, tc := range testCases {
		assert.NotPanics(t, func() {
			metrics.IncWebhook(ctx, tc.webhookType, tc.status)
		})
	}
}

func TestRecordWebhookDuration(t *testing.T) {
	t.Parallel()
	metrics, cleanup := setupTestMetrics(t)
	defer cleanup()

	ctx := context.Background()

	durations := []time.Duration{
		100 * time.Millisecond,
		500 * time.Millisecond,
		1 * time.Second,
		5 * time.Second,
	}

	for _, duration := range durations {
		assert.NotPanics(t, func() {
			metrics.RecordWebhookDuration(ctx, "trunk", duration)
		})
	}
}

func TestIncWebhookValidationFailure(t *testing.T) {
	t.Parallel()
	metrics, cleanup := setupTestMetrics(t)
	defer cleanup()

	ctx := context.Background()

	reasons := []string{"invalid_signature", "missing_header", "invalid_payload"}

	for _, reason := range reasons {
		assert.NotPanics(t, func() {
			metrics.IncWebhookValidationFailure(ctx, reason)
		})
	}
}

func TestIncWorkerMessage(t *testing.T) {
	t.Parallel()
	metrics, cleanup := setupTestMetrics(t)
	defer cleanup()

	ctx := context.Background()

	testCases := []struct {
		messageType string
		status      string
	}{
		{"webhook", "received"},
		{"webhook", "processed"},
		{"webhook", "failed"},
		{"retry", "processed"},
	}

	for _, tc := range testCases {
		assert.NotPanics(t, func() {
			metrics.IncWorkerMessage(ctx, tc.messageType, tc.status)
		})
	}
}

func TestRecordWorkerProcessingDuration(t *testing.T) {
	t.Parallel()
	metrics, cleanup := setupTestMetrics(t)
	defer cleanup()

	ctx := context.Background()

	durations := []time.Duration{
		50 * time.Millisecond,
		200 * time.Millisecond,
		1 * time.Second,
	}

	for _, duration := range durations {
		assert.NotPanics(t, func() {
			metrics.RecordWorkerProcessingDuration(ctx, "webhook", duration)
		})
	}
}

func TestRecordWorkerPollInterval(t *testing.T) {
	t.Parallel()
	metrics, cleanup := setupTestMetrics(t)
	defer cleanup()

	ctx := context.Background()

	intervals := []time.Duration{
		1 * time.Second,
		5 * time.Second,
		30 * time.Second,
	}

	for _, interval := range intervals {
		assert.NotPanics(t, func() {
			metrics.RecordWorkerPollInterval(ctx, interval)
		})
	}
}

func TestIncQuarantineOperation(t *testing.T) {
	t.Parallel()
	metrics, cleanup := setupTestMetrics(t)
	defer cleanup()

	ctx := context.Background()

	testCases := []struct {
		packageName string
		result      string
	}{
		{"github.com/example/package", "success"},
		{"github.com/example/package", "failed"},
		{"github.com/other/package", "success"},
	}

	for _, tc := range testCases {
		assert.NotPanics(t, func() {
			metrics.IncQuarantineOperation(ctx, tc.packageName, tc.result)
		})
	}
}

func TestRecordQuarantineFilesModified(t *testing.T) {
	t.Parallel()
	metrics, cleanup := setupTestMetrics(t)
	defer cleanup()

	ctx := context.Background()

	counts := []int64{0, 1, 3, 5, 10}

	for _, count := range counts {
		assert.NotPanics(t, func() {
			metrics.RecordQuarantineFilesModified(ctx, count)
		})
	}
}

func TestRecordQuarantineDuration(t *testing.T) {
	t.Parallel()
	metrics, cleanup := setupTestMetrics(t)
	defer cleanup()

	ctx := context.Background()

	durations := []time.Duration{
		100 * time.Millisecond,
		1 * time.Second,
		10 * time.Second,
	}

	for _, duration := range durations {
		assert.NotPanics(t, func() {
			metrics.RecordQuarantineDuration(ctx, duration)
		})
	}
}

func TestRecordGitHubAPILatency(t *testing.T) {
	t.Parallel()
	metrics, cleanup := setupTestMetrics(t)
	defer cleanup()

	ctx := context.Background()

	testCases := []struct {
		operation string
		duration  time.Duration
	}{
		{"create_pr", 200 * time.Millisecond},
		{"list_files", 100 * time.Millisecond},
		{"get_commit", 50 * time.Millisecond},
	}

	for _, tc := range testCases {
		assert.NotPanics(t, func() {
			metrics.RecordGitHubAPILatency(ctx, tc.operation, tc.duration)
		})
	}
}

func TestIncGitHubRateLimitHit(t *testing.T) {
	t.Parallel()
	metrics, cleanup := setupTestMetrics(t)
	defer cleanup()

	ctx := context.Background()

	assert.NotPanics(t, func() {
		metrics.IncGitHubRateLimitHit(ctx)
	})
}

func TestRecordJiraAPILatency(t *testing.T) {
	t.Parallel()
	metrics, cleanup := setupTestMetrics(t)
	defer cleanup()

	ctx := context.Background()

	testCases := []struct {
		operation string
		duration  time.Duration
	}{
		{"create_issue", 300 * time.Millisecond},
		{"search", 150 * time.Millisecond},
		{"update_issue", 200 * time.Millisecond},
	}

	for _, tc := range testCases {
		assert.NotPanics(t, func() {
			metrics.RecordJiraAPILatency(ctx, tc.operation, tc.duration)
		})
	}
}

func TestIncJiraTicket(t *testing.T) {
	t.Parallel()
	metrics, cleanup := setupTestMetrics(t)
	defer cleanup()

	ctx := context.Background()

	actions := []string{"created", "updated", "linked"}

	for _, action := range actions {
		assert.NotPanics(t, func() {
			metrics.IncJiraTicket(ctx, action)
		})
	}
}

func TestRecordSQSSendLatency(t *testing.T) {
	t.Parallel()
	metrics, cleanup := setupTestMetrics(t)
	defer cleanup()

	ctx := context.Background()

	durations := []time.Duration{
		10 * time.Millisecond,
		50 * time.Millisecond,
		100 * time.Millisecond,
	}

	for _, duration := range durations {
		assert.NotPanics(t, func() {
			metrics.RecordSQSSendLatency(ctx, duration)
		})
	}
}

func TestRecordSQSReceiveBatchSize(t *testing.T) {
	t.Parallel()
	metrics, cleanup := setupTestMetrics(t)
	defer cleanup()

	ctx := context.Background()

	sizes := []int64{0, 1, 5, 10}

	for _, size := range sizes {
		assert.NotPanics(t, func() {
			metrics.RecordSQSReceiveBatchSize(ctx, size)
		})
	}
}

func TestIncSQSMessageDelete(t *testing.T) {
	t.Parallel()
	metrics, cleanup := setupTestMetrics(t)
	defer cleanup()

	ctx := context.Background()

	statuses := []string{"success", "failed"}

	for _, status := range statuses {
		assert.NotPanics(t, func() {
			metrics.IncSQSMessageDelete(ctx, status)
		})
	}
}

func TestRecordTrunkAPILatency(t *testing.T) {
	t.Parallel()
	metrics, cleanup := setupTestMetrics(t)
	defer cleanup()

	ctx := context.Background()

	testCases := []struct {
		operation string
		duration  time.Duration
	}{
		{"link_ticket", 200 * time.Millisecond},
		{"quarantined_tests", 500 * time.Millisecond},
	}

	for _, tc := range testCases {
		assert.NotPanics(t, func() {
			metrics.RecordTrunkAPILatency(ctx, tc.operation, tc.duration)
		})
	}
}

func TestIncFlakyTestDetected(t *testing.T) {
	t.Parallel()
	metrics, cleanup := setupTestMetrics(t)
	defer cleanup()

	ctx := context.Background()

	testCases := []struct {
		testName    string
		packageName string
	}{
		{"TestExample", "github.com/example/package"},
		{"TestAnother", "github.com/other/package"},
	}

	for _, tc := range testCases {
		assert.NotPanics(t, func() {
			metrics.IncFlakyTestDetected(ctx, tc.testName, tc.packageName)
		})
	}
}

func TestRecordTimeToQuarantine(t *testing.T) {
	t.Parallel()
	metrics, cleanup := setupTestMetrics(t)
	defer cleanup()

	ctx := context.Background()

	durations := []time.Duration{
		1 * time.Minute,
		5 * time.Minute,
		15 * time.Minute,
	}

	for _, duration := range durations {
		assert.NotPanics(t, func() {
			metrics.RecordTimeToQuarantine(ctx, duration)
		})
	}
}

func TestIncTestRecovered(t *testing.T) {
	t.Parallel()
	metrics, cleanup := setupTestMetrics(t)
	defer cleanup()

	ctx := context.Background()
	assert.NotPanics(t, func() {
		metrics.IncTestRecovered(ctx)
	})
}

func TestMetricsIntegration(t *testing.T) {
	t.Parallel()
	// Test a realistic workflow scenario
	metrics, cleanup := setupTestMetrics(t)
	defer cleanup()

	ctx := context.Background()

	// Simulate webhook processing workflow
	assert.NotPanics(t, func() {
		// Webhook received
		metrics.IncWebhook(ctx, "trunk", "received")

		// HTTP request logged
		metrics.IncHTTPRequest(ctx, 200, "POST", "/webhook")

		// Message sent to SQS
		metrics.IncSQSOperations(ctx, "send")
		metrics.RecordSQSSendLatency(ctx, 25*time.Millisecond)

		// Worker processes message
		metrics.IncWorkerMessage(ctx, "webhook", "received")

		// GitHub operations
		metrics.RecordGitHubAPILatency(ctx, "create_pr", 300*time.Millisecond)

		// Jira operations
		metrics.RecordJiraAPILatency(ctx, "create_ticket", 400*time.Millisecond)
		metrics.IncJiraTicket(ctx, "created")

		// Flaky test detected and quarantined
		metrics.IncFlakyTestDetected(ctx, "TestExample", "github.com/example/package")
		metrics.RecordTimeToQuarantine(ctx, 2*time.Minute)

		// Final processing
		metrics.RecordWebhookDuration(ctx, "trunk", 5*time.Second)
		metrics.IncWorkerMessage(ctx, "webhook", "processed")
	})
}
