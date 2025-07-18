package trunk

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/internal/testhelpers"
)

var (
	flakyTestCase = TestCase{
		ID:         "test_trunk_id",
		Codeowners: []string{"@test"},
		FilePath:   "test/file_test.go",
		HTMLURL:    "https://github.com/test/repo",
		Name:       "TestFlaky",
		Quarantine: true,
	}

	quarantinedPayload = TestCaseStatusChange{
		StatusChange: StatusChange{
			CurrentStatus: Status{
				Value: "flaky",
			},
			PreviousStatus: "healthy",
		},
		TestCase: flakyTestCase,
	}

	// Unused for now, but keeping for reference.
	// healthyTestCase = TestCase{
	// 	ID:         trunkID,
	// 	Codeowners: codeowners,
	// 	FilePath:   filePath,
	// 	HTMLURL:    repoURL,
	// 	Name:       testName,
	// 	Quarantine: false,
	// }

	// unQuarantinedPayload = TestCaseStatusChange{
	// 	StatusChange: StatusChange{
	// 		CurrentStatus: CurrentStatus{
	// 			Value: "healthy",
	// 		},
	// 		PreviousStatus: "flaky",
	// 	},
	// 	TestCase: healthyTestCase,
	// }
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
