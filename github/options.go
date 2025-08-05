// Package github provides utilities for manipulating GitHub branches and PRs.
package github

import (
	"fmt"
	"net/url"
	"strings"
)

// FlakyTestOptions describes the options for flaky test operations (quarantine/unquarantine).
type FlakyTestOptions struct {
	buildFlags []string // Any build flags to pass to the go command (e.g. ["-tags", "integration"])
}

// FlakyTestOption is a function type that can be used to configure flaky test operations.
type FlakyTestOption func(*FlakyTestOptions)

// WithBuildFlags sets the build flags to use when loading packages.
func WithBuildFlags(buildFlags []string) FlakyTestOption {
	return func(options *FlakyTestOptions) {
		options.buildFlags = buildFlags
	}
}

// ParseRepoURL parses a GitHub repository URL and returns owner and repo name
func ParseRepoURL(repoURL string) (host, owner, repo string, err error) {
	if repoURL == "" {
		return "", "", "", fmt.Errorf("repository URL is required")
	}

	// Parse the URL
	u, err := url.Parse(repoURL)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid repository URL: %w", err)
	}

	host = u.Host

	// Extract owner and repo from path
	path := strings.Trim(u.Path, "/")
	parts := strings.Split(path, "/")

	if len(parts) < 2 {
		return "", "", "", fmt.Errorf("invalid repository URL format, expected: https://github.com/owner/repo")
	}

	owner = parts[0]
	repo = parts[1]

	// Remove .git suffix if present
	repo = strings.TrimSuffix(repo, ".git")

	return host, owner, repo, nil
}
