package processing

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/base"
	"github.com/smartcontractkit/branch-out/internal/testhelpers"
	"github.com/smartcontractkit/branch-out/trunk"
)

func TestServer_Integration_TrunkWebhook(t *testing.T) {
	t.Parallel()
	t.Skip("Skipping integration test until I implement mocking")

	l := testhelpers.Logger(t)
	s, _, _, _ := runServer(t, l)

	trunkWebhookURL := url.URL{
		Scheme: "http",
		Host:   s.Addr,
		Path:   "/webhooks/trunk",
	}
	jsonPayload, err := json.Marshal(trunkPayloadHealthyToFlaky)
	require.NoError(t, err, "failed to marshal trunk payload to json")
	req, err := http.NewRequest(http.MethodPost, trunkWebhookURL.String(), bytes.NewBuffer(jsonPayload))
	require.NoError(t, err, "failed to create request")
	req.Header.Set("Content-Type", "application/json")

	req, err = SelfSignWebhookRequest(l, req, s.config.Trunk.WebhookSecret)
	require.NoError(t, err, "failed to self-sign webhook request")

	c := base.NewClient("test", base.WithLogger(l))
	resp, err := c.Do(req)
	require.NoError(t, err, "failed to send request")

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "failed to read response body")
	var response WebhookResponse
	err = json.Unmarshal(respBody, &response)
	require.NoError(t, err, "failed to unmarshal response body")
	require.True(t, response.Success, "expected a successful response, got '%s'", response.Message)
	require.Equal(t, http.StatusOK, resp.StatusCode, "expected a healthy status, got '%s'", response.Message)
}

var (
	trunkPayloadHealthyToFlaky = trunk.TestCaseStatusChange{
		StatusChange: trunk.StatusChange{
			CurrentStatus: trunk.Status{
				Reason:    "Inconsistent results on main",
				Timestamp: time.Now().Format(time.RFC3339),
				Value:     "flaky",
			},
			PreviousStatus: "healthy",
		},
		TestCase: trunk.TestCase{
			Codeowners:                 []string{"@backend"},
			FailureRateLast7D:          0.1,
			FilePath:                   "test/flaky_test.go",
			HTMLURL:                    "https://fake.app.trunk.io/test/flaky_test.go",
			ID:                         "123",
			Name:                       "TestFlaky",
			TestSuite:                  "github.com/smartcontractkit/branch-out",
			PullRequestsImpactedLast7D: 1,
			Quarantine:                 true,
			Repository:                 trunk.Repository{HTMLURL: "https://github.com/smartcontractkit/branch-out"},
		},
	}
)
