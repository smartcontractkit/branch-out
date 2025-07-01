package github

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			originalAppId := os.Getenv(AppIdEnvVar)
			defer os.Setenv(AppIdEnvVar, originalAppId)
			os.Unsetenv(AppIdEnvVar)

			tokenSource, err := LoadInstallationTokenSource()
			require.NoError(t, err)
			assert.Nil(t, tokenSource) // Should return nil when no app configured
		})

		t.Run("invalid_app_id", func(t *testing.T) {
			originalAppId := os.Getenv(AppIdEnvVar)
			defer os.Setenv(AppIdEnvVar, originalAppId)

			os.Setenv(AppIdEnvVar, "invalid")

			tokenSource, err := LoadInstallationTokenSource()
			assert.Error(t, err)
			assert.Nil(t, tokenSource)
			assert.Contains(t, err.Error(), "invalid GitHub App ID")
		})

		t.Run("missing_private_key", func(t *testing.T) {
			originalAppId := os.Getenv(AppIdEnvVar)
			originalPrivateKey := os.Getenv(PrivateKeyEnvVar)
			originalPrivateKeyFile := os.Getenv(PrivateKeyFileEnvVar)
			defer func() {
				os.Setenv(AppIdEnvVar, originalAppId)
				os.Setenv(PrivateKeyEnvVar, originalPrivateKey)
				os.Setenv(PrivateKeyFileEnvVar, originalPrivateKeyFile)
			}()

			os.Setenv(AppIdEnvVar, "12345")
			os.Unsetenv(PrivateKeyEnvVar)
			os.Unsetenv(PrivateKeyFileEnvVar)

			tokenSource, err := LoadInstallationTokenSource()
			assert.Error(t, err)
			assert.Nil(t, tokenSource)
			assert.Contains(t, err.Error(), "GitHub App private key not found")
		})

		t.Run("private_key_from_env", func(t *testing.T) {
			originalAppId := os.Getenv(AppIdEnvVar)
			originalPrivateKey := os.Getenv(PrivateKeyEnvVar)
			originalInstallationId := os.Getenv(InstallationIdEnvVar)
			defer func() {
				os.Setenv(AppIdEnvVar, originalAppId)
				os.Setenv(PrivateKeyEnvVar, originalPrivateKey)
				os.Setenv(InstallationIdEnvVar, originalInstallationId)
			}()

			os.Setenv(AppIdEnvVar, "12345")
			os.Setenv(PrivateKeyEnvVar, testPrivateKey)
			os.Setenv(InstallationIdEnvVar, "67890")

			tokenSource, err := LoadInstallationTokenSource()
			require.NoError(t, err)
			require.NotNil(t, tokenSource)
		})

		t.Run("private_key_from_file", func(t *testing.T) {
			originalAppId := os.Getenv(AppIdEnvVar)
			originalPrivateKey := os.Getenv(PrivateKeyEnvVar)
			originalPrivateKeyFile := os.Getenv(PrivateKeyFileEnvVar)
			originalInstallationId := os.Getenv(InstallationIdEnvVar)
			defer func() {
				os.Setenv(AppIdEnvVar, originalAppId)
				os.Setenv(PrivateKeyEnvVar, originalPrivateKey)
				os.Setenv(PrivateKeyFileEnvVar, originalPrivateKeyFile)
				os.Setenv(InstallationIdEnvVar, originalInstallationId)
			}()

			os.Setenv(AppIdEnvVar, "12345")
			os.Unsetenv(PrivateKeyEnvVar) // Ensure env var takes precedence
			os.Setenv(PrivateKeyFileEnvVar, testPrivateKeyPath)
			os.Setenv(InstallationIdEnvVar, "67890")

			tokenSource, err := LoadInstallationTokenSource()
			require.NoError(t, err)
			require.NotNil(t, tokenSource)
		})

		t.Run("missing_installation_id", func(t *testing.T) {
			originalAppId := os.Getenv(AppIdEnvVar)
			originalPrivateKey := os.Getenv(PrivateKeyEnvVar)
			originalInstallationId := os.Getenv(InstallationIdEnvVar)
			defer func() {
				os.Setenv(AppIdEnvVar, originalAppId)
				os.Setenv(PrivateKeyEnvVar, originalPrivateKey)
				os.Setenv(InstallationIdEnvVar, originalInstallationId)
			}()

			os.Setenv(AppIdEnvVar, "12345")
			os.Setenv(PrivateKeyEnvVar, testPrivateKey)
			os.Unsetenv(InstallationIdEnvVar)

			tokenSource, err := LoadInstallationTokenSource()
			assert.Error(t, err)
			assert.Nil(t, tokenSource)
			assert.Contains(t, err.Error(), "GitHub Installation ID is required")
		})

		t.Run("invalid_installation_id", func(t *testing.T) {
			originalAppId := os.Getenv(AppIdEnvVar)
			originalPrivateKey := os.Getenv(PrivateKeyEnvVar)
			originalInstallationId := os.Getenv(InstallationIdEnvVar)
			defer func() {
				os.Setenv(AppIdEnvVar, originalAppId)
				os.Setenv(PrivateKeyEnvVar, originalPrivateKey)
				os.Setenv(InstallationIdEnvVar, originalInstallationId)
			}()

			os.Setenv(AppIdEnvVar, "12345")
			os.Setenv(PrivateKeyEnvVar, testPrivateKey)
			os.Setenv(InstallationIdEnvVar, "invalid")

			tokenSource, err := LoadInstallationTokenSource()
			assert.Error(t, err)
			assert.Nil(t, tokenSource)
			assert.Contains(t, err.Error(), "invalid GitHub Installation ID")
		})
	})
}
