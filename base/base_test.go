package base

import (
	"bytes"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/internal/testhelpers"
	"github.com/smartcontractkit/branch-out/logging"
)

func TestNewTransport_Logging(t *testing.T) {
	t.Parallel()

	logs := bytes.NewBuffer(nil)
	logger := testhelpers.Logger(t, logging.WithSoleWriter(logs))
	transport := NewTransport("test", WithLogger(logger))
	require.NotNil(t, transport)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	resp, err := transport.RoundTrip(httptest.NewRequest("GET", server.URL, nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	assert.Contains(t, logs.String(), `"component":"test"`, "missing component in logs")
	assert.Contains(t, logs.String(), "HTTP client request", "missing request in logs")
}

func TestNewClient_Logging(t *testing.T) {
	t.Parallel()

	logs := bytes.NewBuffer(nil)
	logger := testhelpers.Logger(t, logging.WithSoleWriter(logs))
	client := NewClient("test", WithLogger(logger))
	require.NotNil(t, client)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	resp, err := client.Get(server.URL)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	assert.Contains(t, logs.String(), `"component":"test"`, "missing component in logs")
	assert.Contains(t, logs.String(), "HTTP client request", "missing request in logs")
	assert.Contains(t, logs.String(), "HTTP client response", "missing response in logs")

	server.Close()
	logs.Reset()
	resp, err = client.Get(server.URL)
	require.Error(t, err, "expected error calling closed server")
	require.Nil(t, resp, "expected nil response")

	assert.Contains(t, logs.String(), "HTTP client request", "missing request in logs")
	assert.Contains(t, logs.String(), "HTTP client error", "missing error in logs")
}

func TestNewClient_RateLimitHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		statusCode   int
		header       http.Header
		expectError  bool
		expectLogMsg string
	}{
		{
			name: "activate rate limit warning",
			header: http.Header{
				"X-RateLimit-Limit":     []string{"100"},
				"X-RateLimit-Remaining": []string{fmt.Sprint(RateLimitWarningThreshold - 1)},
				"X-RateLimit-Used":      []string{"10"},
				"X-RateLimit-Reset":     []string{"1718211600"},
			},
			statusCode:   http.StatusOK,
			expectError:  false,
			expectLogMsg: RateLimitWarningMsg,
		},
		{
			name: "good headers",
			header: http.Header{
				"X-RateLimit-Limit":     []string{"100"},
				"X-RateLimit-Remaining": []string{"10"},
				"X-RateLimit-Used":      []string{"10"},
				"X-RateLimit-Reset":     []string{"1718211600"},
			},
			statusCode: http.StatusOK,
		},
		{
			name: "bad limit header",
			header: http.Header{
				"X-RateLimit-Limit":     []string{"bad"},
				"X-RateLimit-Remaining": []string{"10"},
				"X-RateLimit-Used":      []string{"10"},
				"X-RateLimit-Reset":     []string{"1718211600"},
			},
			statusCode:  http.StatusOK,
			expectError: true,
		},
		{
			name: "bad remaining header",
			header: http.Header{
				"X-RateLimit-Limit":     []string{"100"},
				"X-RateLimit-Remaining": []string{"bad"},
				"X-RateLimit-Used":      []string{"10"},
				"X-RateLimit-Reset":     []string{"1718211600"},
			},
			statusCode:  http.StatusOK,
			expectError: true,
		},
		{
			name: "bad used header",
			header: http.Header{
				"X-RateLimit-Limit":     []string{"100"},
				"X-RateLimit-Remaining": []string{"10"},
				"X-RateLimit-Used":      []string{"bad"},
				"X-RateLimit-Reset":     []string{"1718211600"},
			},
			statusCode:  http.StatusOK,
			expectError: true,
		},
		{
			name: "bad reset header",
			header: http.Header{
				"X-RateLimit-Limit":     []string{"100"},
				"X-RateLimit-Remaining": []string{"10"},
				"X-RateLimit-Used":      []string{"10"},
				"X-RateLimit-Reset":     []string{"bad"},
			},
			statusCode:  http.StatusOK,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				logs = bytes.NewBuffer(nil)
				l    = testhelpers.Logger(t, logging.WithSoleWriter(logs))
			)

			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				maps.Copy(w.Header(), tt.header)
				w.WriteHeader(tt.statusCode)
			}))
			defer ts.Close()

			client := NewClient("test", WithLogger(l))
			require.NotNil(t, client)

			resp, err := client.Get(ts.URL)
			if tt.expectError {
				require.Error(t, err, "expected error")
				return
			}
			require.NoError(t, err, "expected no error")
			require.NotNil(t, resp, "expected non nil response")
			assert.Equal(t, tt.statusCode, resp.StatusCode, "expected status code to be %d", tt.statusCode)
			if tt.expectLogMsg != "" {
				assert.Contains(t, logs.String(), tt.expectLogMsg)
			}
		})
	}
}
