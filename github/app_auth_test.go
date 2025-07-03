package github

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:paralleltest // Tests use environment variables and cannot be parallelized
func TestGithubAppTokenSource(t *testing.T) {
	// Load test private key
	testPrivateKeyPath := "testdata/test_key.pem"
	testPrivateKeyBytes, err := os.ReadFile(testPrivateKeyPath)
	if err != nil {
		t.Fatalf("failed to read test private key: %v", err)
	}
	testPrivateKey := string(testPrivateKeyBytes)

	t.Run("LoadGithubAppTokenSource", func(t *testing.T) {
		t.Run("no_app_id", func(t *testing.T) {
			// Clear environment
			originalAppID := os.Getenv(AppIDEnvVar)
			defer func() {
				require.NoError(t, os.Setenv(AppIDEnvVar, originalAppID))
			}()
			require.NoError(t, os.Unsetenv(AppIDEnvVar))

			tokenSource, err := LoadInstallationTokenSource()
			require.NoError(t, err)
			assert.Nil(t, tokenSource) // Should return nil when no app configured
		})

		t.Run("invalid_app_id", func(t *testing.T) {
			originalAppID := os.Getenv(AppIDEnvVar)
			defer func() {
				require.NoError(t, os.Setenv(AppIDEnvVar, originalAppID))
			}()

			require.NoError(t, os.Setenv(AppIDEnvVar, "invalid"))

			tokenSource, err := LoadInstallationTokenSource()
			require.Error(t, err)
			assert.Nil(t, tokenSource)
			assert.Contains(t, err.Error(), "invalid GitHub App ID")
		})

		t.Run("missing_private_key", func(t *testing.T) {
			originalAppID := os.Getenv(AppIDEnvVar)
			originalPrivateKey := os.Getenv(PrivateKeyEnvVar)
			originalPrivateKeyFile := os.Getenv(PrivateKeyFileEnvVar)
			defer func() {
				require.NoError(t, os.Setenv(AppIDEnvVar, originalAppID))
				require.NoError(t, os.Setenv(PrivateKeyEnvVar, originalPrivateKey))
				require.NoError(t, os.Setenv(PrivateKeyFileEnvVar, originalPrivateKeyFile))
			}()

			require.NoError(t, os.Setenv(AppIDEnvVar, "12345"))
			require.NoError(t, os.Unsetenv(PrivateKeyEnvVar))
			require.NoError(t, os.Unsetenv(PrivateKeyFileEnvVar))

			tokenSource, err := LoadInstallationTokenSource()
			require.Error(t, err)
			assert.Nil(t, tokenSource)
			assert.Contains(t, err.Error(), "GitHub App private key not found")
		})

		t.Run("private_key_from_env", func(t *testing.T) {
			originalAppID := os.Getenv(AppIDEnvVar)
			originalPrivateKey := os.Getenv(PrivateKeyEnvVar)
			originalInstallationID := os.Getenv(InstallationIDEnvVar)
			defer func() {
				require.NoError(t, os.Setenv(AppIDEnvVar, originalAppID))
				require.NoError(t, os.Setenv(PrivateKeyEnvVar, originalPrivateKey))
				require.NoError(t, os.Setenv(InstallationIDEnvVar, originalInstallationID))
			}()

			require.NoError(t, os.Setenv(AppIDEnvVar, "12345"))
			require.NoError(t, os.Setenv(PrivateKeyEnvVar, testPrivateKey))
			require.NoError(t, os.Setenv(InstallationIDEnvVar, "67890"))

			tokenSource, err := LoadInstallationTokenSource()
			require.NoError(t, err)
			require.NotNil(t, tokenSource)
		})

		t.Run("private_key_from_file", func(t *testing.T) {
			originalAppID := os.Getenv(AppIDEnvVar)
			originalPrivateKey := os.Getenv(PrivateKeyEnvVar)
			originalPrivateKeyFile := os.Getenv(PrivateKeyFileEnvVar)
			originalInstallationID := os.Getenv(InstallationIDEnvVar)
			defer func() {
				require.NoError(t, os.Setenv(AppIDEnvVar, originalAppID))
				require.NoError(t, os.Setenv(PrivateKeyEnvVar, originalPrivateKey))
				require.NoError(t, os.Setenv(PrivateKeyFileEnvVar, originalPrivateKeyFile))
				require.NoError(t, os.Setenv(InstallationIDEnvVar, originalInstallationID))
			}()

			require.NoError(t, os.Setenv(AppIDEnvVar, "12345"))
			require.NoError(t, os.Unsetenv(PrivateKeyEnvVar)) // Ensure env var takes precedence
			require.NoError(t, os.Setenv(PrivateKeyFileEnvVar, testPrivateKeyPath))
			require.NoError(t, os.Setenv(InstallationIDEnvVar, "67890"))

			tokenSource, err := LoadInstallationTokenSource()
			require.NoError(t, err)
			require.NotNil(t, tokenSource)
		})

		t.Run("missing_installation_id", func(t *testing.T) {
			originalAppID := os.Getenv(AppIDEnvVar)
			originalPrivateKey := os.Getenv(PrivateKeyEnvVar)
			originalInstallationID := os.Getenv(InstallationIDEnvVar)
			defer func() {
				require.NoError(t, os.Setenv(AppIDEnvVar, originalAppID))
				require.NoError(t, os.Setenv(PrivateKeyEnvVar, originalPrivateKey))
				require.NoError(t, os.Setenv(InstallationIDEnvVar, originalInstallationID))
			}()

			require.NoError(t, os.Setenv(AppIDEnvVar, "12345"))
			require.NoError(t, os.Setenv(PrivateKeyEnvVar, testPrivateKey))
			require.NoError(t, os.Unsetenv(InstallationIDEnvVar))

			tokenSource, err := LoadInstallationTokenSource()
			require.Error(t, err)
			assert.Nil(t, tokenSource)
			assert.Contains(t, err.Error(), "GitHub Installation ID is required")
		})

		t.Run("invalid_installation_id", func(t *testing.T) {
			originalAppID := os.Getenv(AppIDEnvVar)
			originalPrivateKey := os.Getenv(PrivateKeyEnvVar)
			originalInstallationID := os.Getenv(InstallationIDEnvVar)
			defer func() {
				require.NoError(t, os.Setenv(AppIDEnvVar, originalAppID))
				require.NoError(t, os.Setenv(PrivateKeyEnvVar, originalPrivateKey))
				require.NoError(t, os.Setenv(InstallationIDEnvVar, originalInstallationID))
			}()

			require.NoError(t, os.Setenv(AppIDEnvVar, "12345"))
			require.NoError(t, os.Setenv(PrivateKeyEnvVar, testPrivateKey))
			require.NoError(t, os.Setenv(InstallationIDEnvVar, "invalid"))

			tokenSource, err := LoadInstallationTokenSource()
			require.Error(t, err)
			assert.Nil(t, tokenSource)
			assert.Contains(t, err.Error(), "invalid GitHub Installation ID")
		})
	})
}
