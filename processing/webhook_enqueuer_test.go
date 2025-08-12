package processing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/internal/testhelpers"
	"github.com/smartcontractkit/branch-out/telemetry"
	"github.com/smartcontractkit/branch-out/trunk"
)

var (
	flakyTestCase = trunk.TestCase{
		ID:         "test_trunk_id",
		Codeowners: []string{"@test"},
		FilePath:   "test/file_test.go",
		HTMLURL:    "https://github.com/test/repo",
		Name:       "TestFlaky",
		Quarantine: true,
	}

	quarantinedPayload = trunk.TestCaseStatusChange{
		StatusChange: trunk.StatusChange{
			CurrentStatus: trunk.Status{
				Value: "flaky",
			},
			PreviousStatus: "healthy",
		},
		TestCase: flakyTestCase,
	}

	healthyTestCase = trunk.TestCase{
		ID:         "test_trunk_id_healthy",
		Codeowners: []string{"@test"},
		FilePath:   "test/file_test.go",
		HTMLURL:    "https://github.com/test/repo",
		Name:       "TestHealthy",
		Quarantine: false,
	}

	unQuarantinedPayload = trunk.TestCaseStatusChange{
		StatusChange: trunk.StatusChange{
			CurrentStatus: trunk.Status{
				Value: "healthy",
			},
			PreviousStatus: "flaky",
		},
		TestCase: healthyTestCase,
	}

	// webhookSecret is the secret used to sign Trunk webhooks. This is an example secret from the Trunk docs.
	// We use it to sign our own payloads and make them valid for testing.
	webhookSecret = "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"
)

func TestSignWebhookRequest(t *testing.T) {
	t.Parallel()

	l := testhelpers.Logger(t)

	quarantinedPayloadJSON, err := json.Marshal(quarantinedPayload)
	require.NoError(t, err, "failed to marshal payload")

	webhookRequest, err := SelfSignWebhookRequest(l, &http.Request{
		Method: "POST",
		URL:    &url.URL{Path: "/webhooks/trunk"},
		Body:   io.NopCloser(bytes.NewBuffer(quarantinedPayloadJSON)),
	}, webhookSecret)
	require.NoError(t, err, "failed to sign webhook request")

	require.NotNil(t, webhookRequest, "webhook request should not be nil")

	err = verifyWebhookRequest(l, webhookRequest, webhookSecret)
	require.NoError(t, err, "failed to verify webhook request")
}

func SetupRequest(t *testing.T, payload interface{}) *http.Request {
	t.Helper()

	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err, "failed to marshal payload")

	req := &http.Request{
		Method: "POST",
		URL:    &url.URL{Path: "/webhooks/trunk"},
		Body:   io.NopCloser(bytes.NewBuffer(payloadBytes)),
	}

	signed, err := SelfSignWebhookRequest(testhelpers.Logger(t), req, webhookSecret)
	require.NoError(t, err)
	return signed
}

func TestWebhookEnqueuer_VerifyAndEnqueueWebhook(t *testing.T) {
	tests := []struct {
		name             string
		setupRequest     func(t *testing.T) *http.Request
		setupMocks       func(t *testing.T, mockAWS *MockAWSClient)
		expectError      bool
		expectedErrorMsg string
	}{
		{
			name: "successful webhook processing",
			setupRequest: func(t *testing.T) *http.Request {
				return SetupRequest(t, quarantinedPayload)
			},
			setupMocks: func(t *testing.T, mockAWS *MockAWSClient) {
				// Expect successful SQS push
				mockAWS.EXPECT().PushMessageToQueue(
					mock.Anything,
					mock.Anything,
					mock.AnythingOfType("string"),
				).Return(nil).Once()
			},
			expectError: false,
		},
		{
			name: "invalid webhook signature",
			setupRequest: func(t *testing.T) *http.Request {
				payload, err := json.Marshal(quarantinedPayload)
				require.NoError(t, err)

				// Create request without signing it properly
				req := &http.Request{
					Method: "POST",
					URL:    &url.URL{Path: "/webhooks/trunk"},
					Body:   io.NopCloser(bytes.NewBuffer(payload)),
					Header: http.Header{
						"webhook-id":        []string{"test-id"},
						"webhook-timestamp": []string{"1234567890"},
						"webhook-signature": []string{"v1,invalid_signature"},
					},
				}
				return req
			},
			setupMocks: func(t *testing.T, mockAWS *MockAWSClient) {
				// No SQS call expected - should fail at signature verification
			},
			expectError:      true,
			expectedErrorMsg: "webhook call cannot be verified",
		},
		{
			name: "invalid JSON payload",
			setupRequest: func(t *testing.T) *http.Request {
				// Invalid JSON payload
				invalidPayload := []byte("{invalid json")

				req := &http.Request{
					Method: "POST",
					URL:    &url.URL{Path: "/webhooks/trunk"},
					Body:   io.NopCloser(bytes.NewBuffer(invalidPayload)),
				}

				signed, err := SelfSignWebhookRequest(testhelpers.Logger(t), req, webhookSecret)
				require.NoError(t, err)
				return signed
			},
			setupMocks: func(t *testing.T, mockAWS *MockAWSClient) {
				// No SQS call expected - should fail at JSON parsing
			},
			expectError:      true,
			expectedErrorMsg: "failed to parse test_case.status_changed payload",
		},
		{
			name: "SQS push failure",
			setupRequest: func(t *testing.T) *http.Request {
				return SetupRequest(t, quarantinedPayload)
			},
			setupMocks: func(t *testing.T, mockAWS *MockAWSClient) {
				// Expect SQS push to fail
				mockAWS.EXPECT().PushMessageToQueue(
					mock.Anything,
					mock.Anything,
					mock.AnythingOfType("string"),
				).Return(fmt.Errorf("SQS error")).Once()
			},
			expectError:      true,
			expectedErrorMsg: "failed to push webhook payload to AWS SQS",
		},
		{
			name: "healthy test case payload",
			setupRequest: func(t *testing.T) *http.Request {
				return SetupRequest(t, healthyTestCase)
			},
			setupMocks: func(t *testing.T, mockAWS *MockAWSClient) {
				// Expect successful SQS push
				mockAWS.EXPECT().PushMessageToQueue(
					mock.Anything,
					mock.Anything,
					mock.AnythingOfType("string"),
				).Return(nil).Once()
			},
			expectError: false,
		},
		{
			name: "empty request body",
			setupRequest: func(t *testing.T) *http.Request {
				req := &http.Request{
					Method: "POST",
					URL:    &url.URL{Path: "/webhooks/trunk"},
					Body:   io.NopCloser(strings.NewReader("")),
				}

				signed, err := SelfSignWebhookRequest(testhelpers.Logger(t), req, webhookSecret)
				require.NoError(t, err)
				return signed
			},
			setupMocks: func(t *testing.T, mockAWS *MockAWSClient) {
				// No SQS call expected - should fail at JSON parsing
			},
			expectError:      true,
			expectedErrorMsg: "failed to parse test_case.status_changed payload",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mocks
			mockAWS := NewMockAWSClient(t)

			// Create real metrics instance
			metrics, _, err := telemetry.NewMetrics()
			require.NoError(t, err)

			// Setup mock expectations
			tt.setupMocks(t, mockAWS)

			// Create webhook enqueuer
			logger := testhelpers.Logger(t)
			enqueuer := NewWebhookEnqueuer(
				logger,
				webhookSecret,
				mockAWS,
				metrics,
			)

			// Setup request
			req := tt.setupRequest(t)
			req = req.WithContext(context.Background())

			// Execute
			err = enqueuer.VerifyAndEnqueueWebhook(req)

			// Verify results
			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedErrorMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedErrorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
