package github

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRepoURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		repoURL       string
		expectedOwner string
		expectedRepo  string
		expectedError string
	}{
		{
			name:          "https URL",
			repoURL:       "https://github.com/owner/repo",
			expectedOwner: "owner",
			expectedRepo:  "repo",
		},
		{
			name:          "https URL with .git suffix",
			repoURL:       "https://github.com/owner/repo.git",
			expectedOwner: "owner",
			expectedRepo:  "repo",
		},
		{
			name:          "URL with trailing slash",
			repoURL:       "https://github.com/owner/repo/",
			expectedOwner: "owner",
			expectedRepo:  "repo",
		},
		{
			name:          "complex owner and repo names",
			repoURL:       "https://github.com/my-org/my-complex-repo-name",
			expectedOwner: "my-org",
			expectedRepo:  "my-complex-repo-name",
		},
		{
			name:          "empty URL",
			repoURL:       "",
			expectedError: "repository URL is required",
		},
		{
			name:          "invalid URL",
			repoURL:       "not-a-url",
			expectedError: "invalid repository URL",
		},
		{
			name:          "URL with insufficient path",
			repoURL:       "https://github.com/owner",
			expectedError: "invalid repository URL format",
		},
		{
			name:          "URL with only domain",
			repoURL:       "https://github.com",
			expectedError: "invalid repository URL format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			owner, repo, err := ParseRepoURL(tt.repoURL)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedOwner, owner)
			assert.Equal(t, tt.expectedRepo, repo)
		})
	}
}
