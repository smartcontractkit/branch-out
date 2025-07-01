package github

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/google/go-github/v73/github"
	"github.com/rs/zerolog"
	gh_graphql "github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/internal/testhelpers"
	"github.com/smartcontractkit/branch-out/logging"
)

func TestNewClient(t *testing.T) {
	// Uses t.Setenv, so we can't run it in parallel.

	tests := []struct {
		name     string
		token    string
		envToken string
	}{
		{
			name: "no token",
		},
		{
			name:     "token overrides env",
			token:    "arg-token",
			envToken: "env-token",
		},
		{
			name:     "only env token",
			envToken: "env-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Uses t.Setenv, so we can't run it in parallel.

			// Clear GitHub App environment variables to prevent interference
			t.Setenv("GITHUB_APP_ID", "")
			t.Setenv("GITHUB_PRIVATE_KEY", "")
			t.Setenv("GITHUB_PRIVATE_KEY_FILE", "")
			t.Setenv("GITHUB_INSTALLATION_ID", "")

			t.Setenv(TokenEnvVar, tt.envToken)
			l := testhelpers.Logger(t)

			opts := []ClientOption{}
			if tt.token != "" {
				// If a token is provided, use the WithToken option
				opts = append(opts, WithToken(tt.token))
			}

			client, err := NewClient(l, opts...)
			require.NoError(t, err, "expected no error")

			require.NotNil(t, client)
			require.NotNil(t, client.Rest)
			require.NotNil(t, client.GraphQL)
			require.IsType(t, &github.Client{}, client.Rest)
			require.IsType(t, &gh_graphql.Client{}, client.GraphQL)

			switch {
			case tt.token != "":
				require.NotNil(t, client.tokenSource, "expected token source to be set")
				token, err := client.tokenSource.Token()
				require.NoError(t, err)
				assert.Equal(t, tt.token, token.AccessToken, "expected arg token to be set")
			case tt.envToken != "":
				require.NotNil(t, client.tokenSource, "expected token source to be set")
				token, err := client.tokenSource.Token()
				require.NoError(t, err)
				assert.Equal(t, tt.envToken, token.AccessToken, "expected env token to be set")
			default:
				assert.Nil(t, client.tokenSource, "expected no token source")
			}
		})
	}
}

func TestRateLimit(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping rate limit test in short mode")
	}

	tests := []struct {
		name        string
		statusCode  int
		header      http.Header
		expectMsgs  []string
		expectError bool
		body        string
	}{
		{
			name:       "warning",
			statusCode: http.StatusOK,
			header: http.Header{
				"X-RateLimit-Limit":     []string{"100"},
				"X-RateLimit-Remaining": []string{fmt.Sprint(RateLimitWarningThreshold - 1)},
				"X-RateLimit-Used":      []string{"10"},
				"X-RateLimit-Reset":     []string{"1718211600"},
			},
			body:        `{"login": "testuser"}`,
			expectMsgs:  []string{RateLimitWarningMsg},
			expectError: false,
		},
		{
			name:       "primary limit hit",
			statusCode: http.StatusTooManyRequests,
			header: http.Header{
				"X-RateLimit-Limit":     []string{"100"},
				"X-RateLimit-Remaining": []string{"0"},
				"X-RateLimit-Used":      []string{"100"},
				"X-RateLimit-Reset": []string{
					fmt.Sprint(time.Now().Add(time.Millisecond * 1).Unix()),
				},
				"X-RateLimit-Resource": []string{"core"},
			},
			body: `{"message": "API rate limit exceeded"}`,
			expectMsgs: []string{
				RateLimitHitMsg,
				`"limit":"primary"`,
			},
			expectError: true,
		},
		// Secondary rate limits will retry automatically
		{
			name:       "secondary limit hit",
			statusCode: http.StatusTooManyRequests,
			header: http.Header{
				"X-RateLimit-Limit": []string{"100"},
				"X-RateLimit-Used":  []string{"100"},
				"X-RateLimit-Reset": []string{
					fmt.Sprint(time.Now().Add(time.Millisecond * 100).Unix()),
				},
				"X-RateLimit-Resource": []string{"core"},
				"Retry-After":          []string{"1"}, // Retry after 1 second
			},
			body: `{"message": "You have exceeded a secondary rate limit", "documentation_url": "https://docs.github.com/rest/overview/resources-in-the-rest-api#secondary-rate-limits"}`,
			expectMsgs: []string{
				RateLimitHitMsg,
				`"limit":"secondary"`,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				logs = bytes.NewBuffer(nil)
				l    = testhelpers.Logger(
					t,
					logging.WithWriters(logs),
					logging.WithLevel("trace"),
				)
			)

			var (
				retryCount = 0
				writeErr   error
			)
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				l.Trace().
					Str("method", r.Method).
					Str("url", r.URL.String()).
					Interface("headers", r.Header).
					Msg("Request to mock server")

				if retryCount >= 1 { // Pass after first retry
					w.WriteHeader(http.StatusOK)
					_, writeErr = w.Write([]byte(`{"login": "testuser"}`))
					return
				}

				maps.Copy(w.Header(), tt.header)
				w.WriteHeader(tt.statusCode)
				_, writeErr = w.Write([]byte(tt.body))
				retryCount++
			}))
			defer ts.Close()

			client, err := NewClient(l, WithToken("test-token"))
			require.NoError(t, err)
			require.NotNil(t, client)

			// Point the client to our test server
			client.Rest.BaseURL, err = url.Parse(ts.URL + "/")
			require.NoError(t, err)

			_, _, err = client.Rest.Users.Get(context.Background(), "testuser")

			// Check for expected log messages
			for _, expectMsg := range tt.expectMsgs {
				assert.Contains(t, logs.String(), expectMsg, "Did not find expected message in logs")
			}

			if tt.expectError {
				require.Error(t, err, "expected error")
			} else {
				require.NoError(t, err, "expected no error")
			}
			require.NoError(t, writeErr, "expected no error writing to response writer")
		})
	}
}

func TestRateLimitHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		statusCode  int
		header      http.Header
		expectError bool
	}{
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

			l := testhelpers.Logger(t)

			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				maps.Copy(w.Header(), tt.header)
				w.WriteHeader(tt.statusCode)
			}))
			defer ts.Close()

			client, err := NewClient(l, WithToken("test-token"))
			require.NoError(t, err)
			require.NotNil(t, client)

			resp, err := client.Rest.Client().Get(ts.URL)
			if tt.expectError {
				require.Error(t, err, "expected error")
				return
			}
			require.NoError(t, err, "expected no error")
			require.NotNil(t, resp, "expected non nil response")
			assert.Equal(t, tt.statusCode, resp.StatusCode, "expected status code to be %d", tt.statusCode)
		})
	}
}

const MockRoundTripperMsg = "Request to mock round tripper"

func TestCustomRoundTripper(t *testing.T) {
	t.Parallel()
	logs := bytes.NewBuffer(nil)
	l := testhelpers.Logger(t, logging.WithSoleWriter(logs))

	mockRT := &mockRoundTripper{
		logger:     l,
		statusCode: http.StatusOK,
		header:     http.Header{},
		body:       `{"login": "testuser"}`,
	}

	client, err := NewClient(mockRT.logger, WithNext(mockRT), WithToken("test-token"))
	require.NoError(t, err)
	require.NotNil(t, client)

	resp, err := client.Rest.Client().Get("https://api.github.com/users/testuser")
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Contains(t, logs.String(), MockRoundTripperMsg, "expected log message")
}

// mockRoundTripper is a mock implementation of http.RoundTripper for testing
type mockRoundTripper struct {
	logger     zerolog.Logger
	statusCode int
	header     http.Header
	body       string
	custom     func(req *http.Request) (*http.Response, error)
	err        error
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	m.logger.Trace().
		Str("method", req.Method).
		Interface("headers", req.Header).
		Str("url", req.URL.String()).
		Msg(MockRoundTripperMsg)

	if m.err != nil {
		return nil, m.err
	}

	if m.custom != nil {
		return m.custom(req)
	}

	// Default response
	return &http.Response{
		StatusCode: m.statusCode,
		Header:     m.header,
		Request:    req,
		Body:       io.NopCloser(bytes.NewBufferString(m.body)),
	}, nil
}

func TestNewClientWithGitHubApp(t *testing.T) {
	// Uses t.Setenv, so we can't run it in parallel.

	testPrivateKeyPath := "testdata/test_key.pem"
	testPrivateKeyBytes, err := os.ReadFile(testPrivateKeyPath)
	if err != nil {
		t.Fatalf("failed to read test private key: %v", err)
	}
	testPrivateKey := string(testPrivateKeyBytes)

	tests := []struct {
		name           string
		appId          string
		privateKey     string
		privateKeyFile string
		token          string
		envToken       string
		installationId string
		expectError    bool
	}{
		{
			name:           "github app with private key",
			appId:          "12345",
			privateKey:     testPrivateKey,
			installationId: "67890",
			expectError:    false,
		},
		{
			name:           "github app with private key file",
			appId:          "12345",
			privateKeyFile: "/tmp/test-private-key.pem",
			installationId: "67890",
			expectError:    false,
		},
		{
			name:        "invalid app id",
			appId:       "invalid",
			privateKey:  testPrivateKey,
			expectError: true,
		},
		{
			name:        "missing private key",
			appId:       "12345",
			expectError: true,
		},
		{
			name:        "token takes priority over app",
			token:       "token-priority",
			appId:       "12345",
			privateKey:  testPrivateKey,
			expectError: false,
		},
		{
			name:           "github app with installation",
			appId:          "12345",
			privateKey:     testPrivateKey,
			installationId: "67890",
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Uses t.Setenv, so we can't run it in parallel.
			t.Setenv(AppIdEnvVar, tt.appId)
			t.Setenv(PrivateKeyEnvVar, tt.privateKey)
			t.Setenv(TokenEnvVar, tt.envToken)
			t.Setenv(InstallationIdEnvVar, tt.installationId)

			// Create private key file for test cases that need it
			if tt.privateKeyFile != "" && tt.privateKey == "" {
				require.NoError(t, os.WriteFile(tt.privateKeyFile, []byte(testPrivateKey), 0600))
				defer os.Remove(tt.privateKeyFile)
				t.Setenv(PrivateKeyFileEnvVar, tt.privateKeyFile)
			}

			l := testhelpers.Logger(t)

			var opts []ClientOption
			if tt.token != "" {
				opts = append(opts, WithToken(tt.token))
			}

			client, err := NewClient(l, opts...)

			if tt.expectError {
				require.Error(t, err, "expected error")
				return
			}

			require.NoError(t, err, "expected no error")
			require.NotNil(t, client)
			require.NotNil(t, client.Rest)
			require.NotNil(t, client.GraphQL)

			// Verify token source is set for auth cases
			switch {
			case tt.token != "":
				assert.NotNil(t, client.tokenSource, "expected token source to be set")
				// Verify it's a static token source by getting a token
				token, err := client.tokenSource.Token()
				require.NoError(t, err)
				assert.Equal(t, tt.token, token.AccessToken, "expected token to match")
			case tt.appId != "" && (tt.privateKey != "" || tt.privateKeyFile != ""):
				assert.NotNil(t, client.tokenSource, "expected token source to be set for GitHub App")
			}
		})
	}
}
