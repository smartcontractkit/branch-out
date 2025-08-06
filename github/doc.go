// Package github provides utilities for manipulating GitHub branches and PRs.
//
// This package includes:
//   - GitHub App authentication via oauth2.TokenSource with installation tokens
//   - Branch management operations (creation, deletion, comparison)
//   - Pull request operations (creation, merging, commenting)
//   - Test quarantine and unquarantine functionality for GitHub workflows
//   - GitHub API client with rate limiting and error handling
//
// The package supports GitHub App authentication and always returns installation
// tokens using a mandatory installation ID for secure API access.
package github
