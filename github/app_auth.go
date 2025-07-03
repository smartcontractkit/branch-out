// Package github provides GitHub App authentication via oauth2.TokenSource.
// It always returns installation tokens using a mandatory installation ID.
package github

import (
	"fmt"
	"os"
	"strconv"

	"github.com/jferrl/go-githubauth"
	"golang.org/x/oauth2"
)

const (
	// AppIDEnvVar is the environment variable that contains the GitHub App ID.
	AppIDEnvVar = "GITHUB_APP_ID"
	// PrivateKeyEnvVar is the environment variable that contains the GitHub App private key.
	PrivateKeyEnvVar = "GITHUB_PRIVATE_KEY"
	// PrivateKeyFileEnvVar is the environment variable that contains the path to the GitHub App private key file.
	PrivateKeyFileEnvVar = "GITHUB_PRIVATE_KEY_FILE"
	// InstallationIDEnvVar is the environment variable that contains the GitHub App Installation ID.
	InstallationIDEnvVar = "GITHUB_INSTALLATION_ID"
)

// LoadInstallationTokenSource loads GitHub App configuration from environment variables
// and returns a token source for installation tokens.
// Returns nil if no GitHub App is configured (missing GITHUB_APP_ID).
func LoadInstallationTokenSource() (oauth2.TokenSource, error) {
	appIDStr := os.Getenv(AppIDEnvVar)
	if appIDStr == "" {
		return nil, nil // No GitHub App configured
	}

	appID, err := strconv.ParseInt(appIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid GitHub App ID in %s: %w", AppIDEnvVar, err)
	}

	var privateKeyBytes []byte

	// Try to get private key from environment variable first
	privateKeyEnv := os.Getenv(PrivateKeyEnvVar)
	if privateKeyEnv != "" {
		privateKeyBytes = []byte(privateKeyEnv)
	} else {
		// Try to get private key from file
		privateKeyFile := os.Getenv(PrivateKeyFileEnvVar)
		if privateKeyFile != "" {
			privateKeyBytes, err = os.ReadFile(privateKeyFile)
			if err != nil {
				return nil, fmt.Errorf("failed to read GitHub App private key file %s: %w", privateKeyFile, err)
			}
		}
	}

	if len(privateKeyBytes) == 0 {
		return nil, fmt.Errorf("GitHub App private key not found: set %s or %s", PrivateKeyEnvVar, PrivateKeyFileEnvVar)
	}

	installationIDStr := os.Getenv(InstallationIDEnvVar)
	if installationIDStr == "" {
		return nil, fmt.Errorf("GitHub Installation ID is required: set %s", InstallationIDEnvVar)
	}
	installationID, err := strconv.ParseInt(installationIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid GitHub Installation ID in %s: %w", InstallationIDEnvVar, err)
	}

	appTokenSource, err := githubauth.NewApplicationTokenSource(appID, privateKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub App token source: %w", err)
	}

	return githubauth.NewInstallationTokenSource(installationID, appTokenSource), nil
}
