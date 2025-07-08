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
				LogLevel: config.DefaultLogLevel,
				Port:     config.DefaultPort,
				GitHub: config.GitHub{
					BaseURL: config.DefaultGitHubBaseURL,
				},
			},
		},
		{
			name: "env vars override default config",
			envVars: map[string]string{
				"LOG_LEVEL":       "error",
				"PORT":            "8888",
				"GITHUB_TOKEN":    "env-token",
				"GITHUB_BASE_URL": "https://api.github.com/test",
			},
			expectedConfig: config.Config{
				LogLevel: "error",
				Port:     8888,
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
				"--port", "8888",
				"--github-token", "test-github-token",
			},
			expectedConfig: config.Config{
				LogLevel: "error",
				Port:     8888,
				GitHub: config.GitHub{
					BaseURL: config.DefaultGitHubBaseURL,
					Token:   "test-github-token",
				},
			},
		},
		{
			name: "flags override env vars",
			envVars: map[string]string{
				"LOG_LEVEL":    "error",
				"PORT":         "8888",
				"GITHUB_TOKEN": "env-token",
			},
			flags: []string{
				"--log-level", "debug",
				"--port", "9999",
				"--github-token", "test-github-token",
			},
			expectedConfig: config.Config{
				LogLevel: "debug",
				Port:     9999,
				GitHub: config.GitHub{
					BaseURL: config.DefaultGitHubBaseURL,
					Token:   "test-github-token",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for key, value := range tc.envVars {
				t.Setenv(key, value)
			}

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
				"config should be properly set with flags overriding env vars",
			)
		})
	}
}
