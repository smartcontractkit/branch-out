package client

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/internal/testhelpers"
	"github.com/smartcontractkit/branch-out/logging"
)

func TestNewResty(t *testing.T) {
	t.Parallel()

	logs := bytes.NewBuffer(nil)
	logger := testhelpers.Logger(t, logging.WithSoleWriter(logs))
	client := NewResty(logger, "test")
	require.NotNil(t, client)

	var (
		responseBody = "test"
		responseCode = http.StatusOK
		writeErr     error
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, writeErr = w.Write([]byte(responseBody))
		w.WriteHeader(responseCode)
	}))

	resp, err := client.R().
		Get(server.URL)
	require.NoError(t, err)
	require.Equal(t, responseCode, resp.StatusCode())
	require.Equal(t, responseBody, resp.String())

	assert.Contains(t, logs.String(), "HTTP client request", "expected logs to contain request data")
	assert.Contains(t, logs.String(), `"method":"GET"`)
	assert.Contains(t, logs.String(), fmt.Sprintf(`"url":"%s"`, server.URL))

	assert.Contains(t, logs.String(), "HTTP client response", "expected logs to contain response data")
	assert.Contains(t, logs.String(), fmt.Sprintf(`"status_code":%d`, responseCode))
	assert.Contains(t, logs.String(), `"duration":"`)
	assert.Contains(t, logs.String(), fmt.Sprintf(`"response_body":"%s"`, responseBody))

	require.NoError(t, writeErr, "error writing response body")
}

func TestNewResty_ClientError(t *testing.T) {
	t.Parallel()

	logs := bytes.NewBuffer(nil)
	logger := testhelpers.Logger(t, logging.WithSoleWriter(logs))
	client := NewResty(logger, "test")
	require.NotNil(t, client)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	serverURL := server.URL
	server.Close() // Close the server to trigger connection refused error

	_, err := client.R().Get(serverURL)
	require.Error(t, err, "expected error when server is closed")

	// Check that the error was logged
	assert.Contains(t, logs.String(), "HTTP client error", "expected logs to contain error")
	assert.Contains(t, logs.String(), `"method":"GET"`)
	assert.Contains(t, logs.String(), fmt.Sprintf(`"url":"%s"`, serverURL))
	assert.Contains(t, logs.String(), "connection refused", "expected connection refused error")
}
