// Package github provides GitHub App authentication via oauth2.TokenSource.
// It always returns installation tokens using a mandatory installation ID.
package github

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/jferrl/go-githubauth"
	"github.com/rs/zerolog"
	"golang.org/x/oauth2"

	"github.com/smartcontractkit/branch-out/config"
)

var (
	// ErrNoGitHubAppID is returned when no GitHub App ID is provided.
	ErrNoGitHubAppID = errors.New("no GitHub App ID provided")
	// ErrNoGitHubPrivateKey is returned when no GitHub private key is provided.
	ErrNoGitHubPrivateKey = errors.New("no GitHub private key provided")
	// ErrNoGitHubInstallationID is returned when no GitHub installation ID is provided.
	ErrNoGitHubInstallationID = errors.New("no GitHub installation ID provided")
	// ErrInvalidGitHubAppID is returned when the GitHub App ID is invalid.
	ErrInvalidGitHubAppID = errors.New("invalid GitHub App ID")
	// ErrInvalidGitHubInstallationID is returned when the GitHub installation ID is invalid.
	ErrInvalidGitHubInstallationID = errors.New("invalid GitHub installation ID")
)

// setupAppAuth enables authentication via a GitHub App if it is installed
// and returns a token source for installation tokens.
// Returns nil if no GitHub App is configured (missing GITHUB_APP_ID).
func setupAuth(l zerolog.Logger, cfg config.GitHub) (oauth2.TokenSource, error) {
	if cfg.Token != "" {
		l.Debug().Msg("Using simple GitHub token for authentication")
		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: cfg.Token}), nil
	}

	l.Debug().Msg("Using GitHub App authentication")
	if cfg.AppID == "" {
		return nil, ErrNoGitHubAppID
	}

	appID, err := strconv.ParseInt(cfg.AppID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidGitHubAppID, err)
	}

	var privateKeyBytes []byte

	if cfg.PrivateKey != "" {
		privateKeyBytes = []byte(cfg.PrivateKey)
	} else if cfg.PrivateKeyFile != "" {
		privateKeyBytes, err = os.ReadFile(cfg.PrivateKeyFile)
		if err != nil {
			return nil, err
		}
	}

	if len(privateKeyBytes) == 0 {
		return nil, ErrNoGitHubPrivateKey
	}

	if cfg.InstallationID == "" {
		return nil, ErrNoGitHubInstallationID
	}
	installationID, err := strconv.ParseInt(cfg.InstallationID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidGitHubInstallationID, err)
	}

	appTokenSource, err := githubauth.NewApplicationTokenSource(appID, privateKeyBytes)
	if err != nil {
		return nil, err
	}

	return githubauth.NewInstallationTokenSource(
		installationID,
		appTokenSource,
		githubauth.WithEnterpriseURLs(cfg.BaseURL, cfg.BaseURL),
	), nil
}
