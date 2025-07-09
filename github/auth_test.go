package github

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/config"
	"github.com/smartcontractkit/branch-out/internal/testhelpers"
)

func TestSetupAuth(t *testing.T) {
	t.Parallel()

	testPrivateKey := getTestPrivateKey(t)

	tests := []struct {
		name             string
		cfg              config.GitHub
		expectedToken    string
		expectedError    error
		expectedErrorMsg string
	}{
		{
			name: "simple token",
			cfg: config.GitHub{
				Token: "test-token",
			},
			expectedToken: "test-token",
		},
		{
			name: "valid github app",
			cfg: config.GitHub{
				AppID:          "12345",
				PrivateKey:     testPrivateKey,
				InstallationID: "67890",
			},
			expectedToken: "12345",
		},
		{
			name: "valid github app with private key file",
			cfg: config.GitHub{
				AppID:          "12345",
				PrivateKeyFile: "testdata/test_key.pem",
				InstallationID: "67890",
			},
			expectedToken: "12345",
		},
		{
			name: "simple token takes precedence over app",
			cfg: config.GitHub{
				Token:          "test-token",
				AppID:          "12345",
				PrivateKey:     testPrivateKey,
				InstallationID: "67890",
			},
			expectedToken: "test-token",
		},
		{
			name:          "no app id",
			cfg:           config.GitHub{},
			expectedError: ErrNoGitHubAppID,
		},
		{
			name:          "no private key",
			cfg:           config.GitHub{AppID: "12345"},
			expectedError: ErrNoGitHubPrivateKey,
		},
		{
			name:          "no installation id",
			cfg:           config.GitHub{AppID: "12345", PrivateKey: testPrivateKey},
			expectedError: ErrNoGitHubInstallationID,
		},
		{
			name: "invalid app id",
			cfg: config.GitHub{
				AppID:          "invalid",
				PrivateKey:     testPrivateKey,
				InstallationID: "12345",
			},
			expectedError: ErrInvalidGitHubAppID,
		},
		{
			name:             "invalid private key",
			cfg:              config.GitHub{AppID: "12345", PrivateKey: "invalid", InstallationID: "12345"},
			expectedErrorMsg: "invalid key",
		},
		{
			name:          "invalid private key file",
			cfg:           config.GitHub{AppID: "12345", PrivateKeyFile: "invalid", InstallationID: "12345"},
			expectedError: os.ErrNotExist,
		},
		{
			name:          "invalid installation id",
			cfg:           config.GitHub{AppID: "12345", PrivateKey: testPrivateKey, InstallationID: "invalid"},
			expectedError: ErrInvalidGitHubInstallationID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := testhelpers.Logger(t)

			tokenSource, err := setupAuth(l, tt.cfg)

			if tt.expectedErrorMsg != "" {
				require.Error(t, err, "expected an error with this config")
				assert.Contains(t, err.Error(), tt.expectedErrorMsg, "expected error message to contain specific text")
				return
			}

			if tt.expectedError != nil {
				require.Error(t, err, "expected an error with this config")
				assert.ErrorIs(t, err, tt.expectedError, "expected a specific error with this config")
				return
			}
			require.NoError(t, err)
			require.NotNil(t, tokenSource)
		})
	}
}
