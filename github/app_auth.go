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
	// AppIdEnvVar is the environment variable that contains the GitHub App ID.
	AppIdEnvVar = "GITHUB_APP_ID"
	// PrivateKeyEnvVar is the environment variable that contains the GitHub App private key.
	PrivateKeyEnvVar = "GITHUB_PRIVATE_KEY"
	// PrivateKeyFileEnvVar is the environment variable that contains the path to the GitHub App private key file.
	PrivateKeyFileEnvVar = "GITHUB_PRIVATE_KEY_FILE"
	// InstallationIdEnvVar is the environment variable that contains the GitHub App Installation ID.
	InstallationIdEnvVar = "GITHUB_INSTALLATION_ID"
)

// LoadInstallationTokenSource loads GitHub App configuration from environment variables
// and returns a token source for installation tokens.
// Returns nil if no GitHub App is configured (missing GITHUB_APP_ID).
func LoadInstallationTokenSource() (oauth2.TokenSource, error) {
	appIdStr := os.Getenv(AppIdEnvVar)
	if appIdStr == "" {
		return nil, nil // No GitHub App configured
	}

	appId, err := strconv.ParseInt(appIdStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid GitHub App ID in %s: %w", AppIdEnvVar, err)
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

	installationIdStr := os.Getenv(InstallationIdEnvVar)
	if installationIdStr == "" {
		return nil, fmt.Errorf("GitHub Installation ID is required: set %s", InstallationIdEnvVar)
	}
	installationId, err := strconv.ParseInt(installationIdStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid GitHub Installation ID in %s: %w", InstallationIdEnvVar, err)
	}

	appTokenSource, err := githubauth.NewApplicationTokenSource(appId, privateKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub App token source: %w", err)
	}

	return githubauth.NewInstallationTokenSource(installationId, appTokenSource), nil
}
