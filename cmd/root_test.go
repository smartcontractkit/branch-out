package cmd

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/config"
)

func TestRoot_Config(t *testing.T) {
	var (
		defaultLogLevel      string
		defaultPort          int
		defaultGitHubBaseURL string
		err                  error
	)
	defaultLogLevel, err = config.GetDefault[string]("log-level")
	require.NoError(t, err)
	defaultPort, err = config.GetDefault[int]("port")
	require.NoError(t, err)
	defaultGitHubBaseURL, err = config.GetDefault[string]("github-base-url")
	require.NoError(t, err)

	testCases := []struct {
		name    string
		envVars map[string]string
		flags   []string

		expectedConfig config.Config
	}{
		{
			name: "default config",
			envVars: map[string]string{
				"GITHUB_TOKEN": "",
			},
			expectedConfig: config.Config{
				LogLevel: defaultLogLevel,
				Port:     defaultPort,
				GitHub: config.GitHub{
					BaseURL: defaultGitHubBaseURL,
				},
			},
		},
		{
			name: "env vars override default config",
			envVars: map[string]string{
				"LOG_LEVEL":       "error",
				"GITHUB_TOKEN":    "env-token",
				"GITHUB_BASE_URL": "https://api.github.com/test",
			},
			expectedConfig: config.Config{
				LogLevel: "error",
				GitHub: config.GitHub{
					BaseURL: "https://api.github.com/test",
					Token:   "env-token",
				},
			},
		},
		{
			name: "just flags",
			flags: []string{
				"--log-level", "error",
				"--github-token", "test-github-token",
				"--trunk-token", "test-trunk-token",
			},
			expectedConfig: config.Config{
				LogLevel: "error",
				GitHub: config.GitHub{
					BaseURL: defaultGitHubBaseURL,
					Token:   "test-github-token",
				},
				Trunk: config.Trunk{
					Token: "test-trunk-token",
				},
			},
		},
		{
			name: "flags override env vars",
			envVars: map[string]string{
				"LOG_LEVEL":    "error",
				"GITHUB_TOKEN": "env-token",
				"TRUNK_TOKEN":  "env-trunk-token",
			},
			flags: []string{
				"--log-level", "debug",
				"--github-token", "test-github-token",
				"--trunk-token", "test-trunk-token",
			},
			expectedConfig: config.Config{
				LogLevel: "debug",
				GitHub: config.GitHub{
					BaseURL: defaultGitHubBaseURL,
					Token:   "test-github-token",
				},
				Trunk: config.Trunk{
					Token: "test-trunk-token",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for key, value := range tc.envVars {
				t.Setenv(key, value)
			}

			// Set port to 0 to allow the server to start on a random port
			tc.flags = append(tc.flags, "--port", "0")
			// Set flags, which should override env vars
			root.SetArgs(tc.flags)

			// Create a context with timeout to prevent the test from hanging
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			t.Cleanup(cancel)

			// Execute will return when the context times out
			err := root.ExecuteContext(ctx)
			require.NoError(t, err, "error executing root command")

			assert.Equal(
				t,
				tc.expectedConfig,
				*appConfig,
				"config should be properly set with flags > env vars > .env file > default values",
			)
		})
	}
}
