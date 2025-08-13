package github

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/go-github/v73/github"
	"github.com/migueleliasweb/go-github-mock/src/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/golang"
	"github.com/smartcontractkit/branch-out/internal/testhelpers"
)

// createTestClient creates a GitHub client with mocked HTTP responses
func createTestClient(mockOptions ...mock.MockBackendOption) *Client {
	// Create the mocked HTTP client
	mockedHTTPClient := mock.NewMockedHTTPClient(mockOptions...)
	ghClient := github.NewClient(mockedHTTPClient)

	// Ignore GraphQL for now as requested
	return &Client{
		Rest:    ghClient,
		GraphQL: nil, // Ignoring GraphQL for now
	}
}

func TestGetBranchNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		owner         string
		repo          string
		defaultBranch string
		mockOptions   []mock.MockBackendOption
		expectedError string
	}{
		{
			name:          "successful branch names retrieval",
			owner:         "testowner",
			repo:          "testrepo",
			defaultBranch: "main",
			mockOptions: []mock.MockBackendOption{
				mock.WithRequestMatch(
					mock.GetReposByOwnerByRepo,
					github.Repository{
						DefaultBranch: github.Ptr("main"),
					},
				),
			},
		},
		{
			name:  "repository not found",
			owner: "testowner",
			repo:  "nonexistent",
			mockOptions: []mock.MockBackendOption{
				mock.WithRequestMatchHandler(
					mock.GetReposByOwnerByRepo,
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						mock.WriteError(
							w,
							http.StatusNotFound,
							"Not Found",
						)
					}),
				),
			},
			expectedError: "failed to get default branch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := createTestClient(tt.mockOptions...)

			ctx := context.Background()
			defaultBranch, prBranch, err := client.GetBranchNames(ctx, tt.owner, tt.repo)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.defaultBranch, defaultBranch)

			// Check that the PR branch follows the expected format
			expectedPRName := fmt.Sprintf("branch-out/quarantine-tests-%s", time.Now().Format("2006-01-02"))
			assert.Equal(t, expectedPRName, prBranch)
		})
	}
}

func TestGetOrCreateRemoteBranch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		owner         string
		repo          string
		branchName    string
		mockOptions   []mock.MockBackendOption
		expectedSHA   string
		expectedError string
	}{
		{
			name:       "existing branch",
			owner:      "testowner",
			repo:       "testrepo",
			branchName: "existing-branch",
			mockOptions: []mock.MockBackendOption{
				mock.WithRequestMatch(
					mock.GetReposGitRefByOwnerByRepoByRef,
					github.Reference{
						Object: &github.GitObject{
							SHA: github.Ptr("existing-sha"),
						},
					},
				),
			},
			expectedSHA: "existing-sha",
		},
		{
			name:       "create new branch",
			owner:      "testowner",
			repo:       "testrepo",
			branchName: "new-branch",
			mockOptions: []mock.MockBackendOption{
				// First call returns not found (branch doesn't exist)
				mock.WithRequestMatchHandler(
					mock.GetReposGitRefByOwnerByRepoByRef,
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						// Check if this is for the new branch or default branch
						if r.URL.Path == "/repos/testowner/testrepo/git/ref/refs/heads/new-branch" {
							mock.WriteError(w, http.StatusNotFound, "Not Found")
							return
						}

						// Return default branch SHA for branch creation
						w.Write(mock.MustMarshal(github.Reference{
							Object: &github.GitObject{
								SHA: github.Ptr("default-branch-sha"),
							},
						}))
					}),
				),
				// Create ref call
				mock.WithRequestMatch(
					mock.PostReposGitRefsByOwnerByRepo,
					github.Reference{
						Ref: github.Ptr("refs/heads/new-branch"),
						Object: &github.GitObject{
							SHA: github.Ptr("default-branch-sha"),
						},
					},
				),
			},
			expectedSHA: "default-branch-sha",
		},
		{
			name:       "git reference error",
			owner:      "testowner",
			repo:       "testrepo",
			branchName: "error-branch",
			mockOptions: []mock.MockBackendOption{
				mock.WithRequestMatchHandler(
					mock.GetReposGitRefByOwnerByRepoByRef,
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						mock.WriteError(w, http.StatusInternalServerError, "Internal Server Error")
					}),
				),
			},
			expectedError: "failed to get branch head SHA",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := createTestClient(tt.mockOptions...)

			ctx := context.Background()
			sha, err := client.GetOrCreateRemoteBranch(ctx, tt.owner, tt.repo, tt.branchName)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedSHA, sha)
		})
	}
}

func TestGenerateCommitAndPush(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		owner         string
		repo          string
		prBranch      string
		branchHeadSHA string
		results       *golang.QuarantineResults
		expectedError string
		skipTest      bool // Skip tests that require GraphQL
	}{
		{
			name:          "skip GraphQL test for now",
			owner:         "testowner",
			repo:          "testrepo",
			prBranch:      "test-branch",
			branchHeadSHA: "abc123",
			results: &golang.QuarantineResults{
				"pkg1": golang.QuarantinePackageResults{
					Package: "pkg1",
					Successes: []golang.QuarantinedFile{
						{
							File:               "test_file.go",
							ModifiedSourceCode: "package main\n// quarantined test",
							Tests: []golang.QuarantinedTest{
								{Name: "TestExample"},
							},
						},
					},
				},
			},
			skipTest: true, // Skipping GraphQL tests for now
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.skipTest {
				t.Skip("Skipping GraphQL test as requested - focusing on REST API only")
				return
			}

			// This test would require GraphQL mocking which we're ignoring for now
			// The actual implementation would go here once we decide to handle GraphQL
		})
	}
}

func TestCreateOrUpdatePullRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		owner         string
		repo          string
		prBranch      string
		defaultBranch string
		results       *golang.QuarantineResults
		mockOptions   []mock.MockBackendOption
		expectedURL   string
		expectedError string
	}{
		{
			name:          "create new pull request",
			owner:         "testowner",
			repo:          "testrepo",
			prBranch:      "test-branch",
			defaultBranch: "main",
			results: &golang.QuarantineResults{
				"pkg1": golang.QuarantinePackageResults{
					Package:   "pkg1",
					Successes: []golang.QuarantinedFile{},
				},
			},
			mockOptions: []mock.MockBackendOption{
				// No existing PR found
				mock.WithRequestMatch(
					mock.GetReposPullsByOwnerByRepo,
					[]*github.PullRequest{},
				),
				// Create new PR
				mock.WithRequestMatch(
					mock.PostReposPullsByOwnerByRepo,
					github.PullRequest{
						HTMLURL: github.Ptr("https://github.com/testowner/testrepo/pull/1"),
						Number:  github.Ptr(1),
					},
				),
				// Add label (might be called)
				mock.WithRequestMatch(
					mock.PostReposIssuesLabelsByOwnerByRepoByIssueNumber,
					[]*github.Label{
						{Name: github.Ptr("branch-out")},
					},
				),
			},
			expectedURL: "https://github.com/testowner/testrepo/pull/1",
		},
		{
			name:          "update existing pull request",
			owner:         "testowner",
			repo:          "testrepo",
			prBranch:      "test-branch",
			defaultBranch: "main",
			results: &golang.QuarantineResults{
				"pkg1": golang.QuarantinePackageResults{
					Package:   "pkg1",
					Successes: []golang.QuarantinedFile{},
				},
			},
			mockOptions: []mock.MockBackendOption{
				// Existing PR found
				mock.WithRequestMatch(
					mock.GetReposPullsByOwnerByRepo,
					[]*github.PullRequest{
						{
							Number:  github.Ptr(5),
							HTMLURL: github.Ptr("https://github.com/testowner/testrepo/pull/5"),
						},
					},
				),
				// Update existing PR
				mock.WithRequestMatch(
					mock.PatchReposPullsByOwnerByRepoByPullNumber,
					github.PullRequest{
						HTMLURL: github.Ptr("https://github.com/testowner/testrepo/pull/5"),
					},
				),
			},
			expectedURL: "https://github.com/testowner/testrepo/pull/5",
		},
		{
			name:          "error listing pull requests",
			owner:         "testowner",
			repo:          "testrepo",
			prBranch:      "test-branch",
			defaultBranch: "main",
			results: &golang.QuarantineResults{
				"pkg1": golang.QuarantinePackageResults{
					Package:   "pkg1",
					Successes: []golang.QuarantinedFile{},
				},
			},
			mockOptions: []mock.MockBackendOption{
				mock.WithRequestMatchHandler(
					mock.GetReposPullsByOwnerByRepo,
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						mock.WriteError(w, http.StatusInternalServerError, "Internal Server Error")
					}),
				),
			},
			expectedError: "failed to check for existing PR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := createTestClient(tt.mockOptions...)

			logger := testhelpers.Logger(t)
			ctx := context.Background()
			url, err := client.CreateOrUpdatePullRequest(ctx, logger, tt.owner, tt.repo, tt.prBranch, tt.defaultBranch, tt.results)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedURL, url)
		})
	}
}

func TestParseCommitMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		input            string
		expectedHeadline string
		expectedBody     string
	}{
		{
			name:             "single line message",
			input:            "Fix bug in handler",
			expectedHeadline: "Fix bug in handler",
			expectedBody:     "",
		},
		{
			name:             "multi-line message",
			input:            "Fix bug in handler\n\nThis fixes the issue with null pointer",
			expectedHeadline: "Fix bug in handler",
			expectedBody:     "\nThis fixes the issue with null pointer",
		},
		{
			name:             "empty message",
			input:            "",
			expectedHeadline: "",
			expectedBody:     "",
		},
		{
			name:             "message with only newline",
			input:            "Single line\n",
			expectedHeadline: "Single line",
			expectedBody:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			headline, body := parseCommitMessage(tt.input)
			assert.Equal(t, tt.expectedHeadline, headline)
			assert.Equal(t, tt.expectedBody, body)
		})
	}
}

func TestBase64EncodeFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple text",
			input:    "hello world",
			expected: "aGVsbG8gd29ybGQ=",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "go source code",
			input:    "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}",
			expected: "cGFja2FnZSBtYWluCgpmdW5jIG1haW4oKSB7CglwcmludGxuKCJoZWxsbyIpCn0=",
		},
		{
			name:     "unicode characters",
			input:    "Hello 世界",
			expected: "SGVsbG8g5LiW55WM",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := base64EncodeFile(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetDefaultBranch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		owner          string
		repo           string
		mockOptions    []mock.MockBackendOption
		expectedBranch string
		expectedError  string
	}{
		{
			name:  "successful get default branch",
			owner: "testowner",
			repo:  "testrepo",
			mockOptions: []mock.MockBackendOption{
				mock.WithRequestMatch(
					mock.GetReposByOwnerByRepo,
					github.Repository{
						DefaultBranch: github.Ptr("develop"),
					},
				),
			},
			expectedBranch: "develop",
		},
		{
			name:  "repository not found",
			owner: "testowner",
			repo:  "nonexistent",
			mockOptions: []mock.MockBackendOption{
				mock.WithRequestMatchHandler(
					mock.GetReposByOwnerByRepo,
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						mock.WriteError(w, http.StatusNotFound, "Not Found")
					}),
				),
			},
			expectedError: "failed to get repository",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := createTestClient(tt.mockOptions...)

			ctx := context.Background()
			branch, err := client.getDefaultBranch(ctx, tt.owner, tt.repo)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedBranch, branch)
		})
	}
}

func TestGetBranchHeadSHA(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		owner          string
		repo           string
		branchName     string
		mockOptions    []mock.MockBackendOption
		expectedSHA    string
		expectedExists bool
		expectedError  string
	}{
		{
			name:       "existing branch",
			owner:      "testowner",
			repo:       "testrepo",
			branchName: "main",
			mockOptions: []mock.MockBackendOption{
				mock.WithRequestMatch(
					mock.GetReposGitRefByOwnerByRepoByRef,
					github.Reference{
						Object: &github.GitObject{
							SHA: github.Ptr("abc123def"),
						},
					},
				),
			},
			expectedSHA:    "abc123def",
			expectedExists: true,
		},
		{
			name:       "non-existing branch",
			owner:      "testowner",
			repo:       "testrepo",
			branchName: "nonexistent",
			mockOptions: []mock.MockBackendOption{
				mock.WithRequestMatchHandler(
					mock.GetReposGitRefByOwnerByRepoByRef,
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						mock.WriteError(w, http.StatusNotFound, "Not Found")
					}),
				),
			},
			expectedSHA:    "",
			expectedExists: false,
		},
		{
			name:       "server error",
			owner:      "testowner",
			repo:       "testrepo",
			branchName: "main",
			mockOptions: []mock.MockBackendOption{
				mock.WithRequestMatchHandler(
					mock.GetReposGitRefByOwnerByRepoByRef,
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						mock.WriteError(w, http.StatusInternalServerError, "Internal Server Error")
					}),
				),
			},
			expectedError: "failed to get branch reference",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := createTestClient(tt.mockOptions...)

			ctx := context.Background()
			sha, exists, err := client.getBranchHeadSHA(ctx, tt.owner, tt.repo, tt.branchName)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedSHA, sha)
			assert.Equal(t, tt.expectedExists, exists)
		})
	}
}

func TestCreatePullRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		owner         string
		repo          string
		headBranch    string
		baseBranch    string
		title         string
		body          string
		mockOptions   []mock.MockBackendOption
		expectedURL   string
		expectedError string
	}{
		{
			name:       "successful pull request creation",
			owner:      "testowner",
			repo:       "testrepo",
			headBranch: "feature-branch",
			baseBranch: "main",
			title:      "Test PR",
			body:       "This is a test PR",
			mockOptions: []mock.MockBackendOption{
				mock.WithRequestMatch(
					mock.PostReposPullsByOwnerByRepo,
					github.PullRequest{
						HTMLURL: github.Ptr("https://github.com/testowner/testrepo/pull/42"),
						Number:  github.Ptr(42),
					},
				),
				// Mock the label addition (can succeed or fail, we ignore errors)
				mock.WithRequestMatch(
					mock.PostReposIssuesLabelsByOwnerByRepoByIssueNumber,
					[]*github.Label{
						{Name: github.Ptr("branch-out")},
					},
				),
			},
			expectedURL: "https://github.com/testowner/testrepo/pull/42",
		},
		{
			name:       "pull request creation error",
			owner:      "testowner",
			repo:       "testrepo",
			headBranch: "feature-branch",
			baseBranch: "main",
			title:      "Test PR",
			body:       "This is a test PR",
			mockOptions: []mock.MockBackendOption{
				mock.WithRequestMatchHandler(
					mock.PostReposPullsByOwnerByRepo,
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						mock.WriteError(w, http.StatusUnprocessableEntity, "Validation Failed")
					}),
				),
			},
			expectedError: "failed to create pull request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := createTestClient(tt.mockOptions...)

			ctx := context.Background()
			url, err := client.createPullRequest(ctx, tt.owner, tt.repo, tt.headBranch, tt.baseBranch, tt.title, tt.body)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedURL, url)
		})
	}
}

func TestFindExistingPR(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		owner         string
		repo          string
		headBranch    string
		baseBranch    string
		mockOptions   []mock.MockBackendOption
		expectedPR    *github.PullRequest
		expectedError string
	}{
		{
			name:       "existing PR found",
			owner:      "testowner",
			repo:       "testrepo",
			headBranch: "feature-branch",
			baseBranch: "main",
			mockOptions: []mock.MockBackendOption{
				mock.WithRequestMatch(
					mock.GetReposPullsByOwnerByRepo,
					[]*github.PullRequest{
						{
							Number:  github.Ptr(10),
							HTMLURL: github.Ptr("https://github.com/testowner/testrepo/pull/10"),
						},
					},
				),
			},
			expectedPR: &github.PullRequest{
				Number:  github.Ptr(10),
				HTMLURL: github.Ptr("https://github.com/testowner/testrepo/pull/10"),
			},
		},
		{
			name:       "no existing PR",
			owner:      "testowner",
			repo:       "testrepo",
			headBranch: "feature-branch",
			baseBranch: "main",
			mockOptions: []mock.MockBackendOption{
				mock.WithRequestMatch(
					mock.GetReposPullsByOwnerByRepo,
					[]*github.PullRequest{},
				),
			},
			expectedPR: nil,
		},
		{
			name:       "error listing PRs",
			owner:      "testowner",
			repo:       "testrepo",
			headBranch: "feature-branch",
			baseBranch: "main",
			mockOptions: []mock.MockBackendOption{
				mock.WithRequestMatchHandler(
					mock.GetReposPullsByOwnerByRepo,
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						mock.WriteError(w, http.StatusInternalServerError, "Internal Server Error")
					}),
				),
			},
			expectedError: "failed to list pull requests",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := createTestClient(tt.mockOptions...)

			ctx := context.Background()
			pr, err := client.findExistingPR(ctx, tt.owner, tt.repo, tt.headBranch, tt.baseBranch)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)
			if tt.expectedPR == nil {
				assert.Nil(t, pr)
			} else {
				require.NotNil(t, pr)
				assert.Equal(t, tt.expectedPR.GetNumber(), pr.GetNumber())
				assert.Equal(t, tt.expectedPR.GetHTMLURL(), pr.GetHTMLURL())
			}
		})
	}
}

func TestUpdatePullRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		owner         string
		repo          string
		prNumber      int
		title         string
		body          string
		mockOptions   []mock.MockBackendOption
		expectedURL   string
		expectedError string
	}{
		{
			name:     "successful pull request update",
			owner:    "testowner",
			repo:     "testrepo",
			prNumber: 15,
			title:    "Updated PR Title",
			body:     "Updated PR body",
			mockOptions: []mock.MockBackendOption{
				mock.WithRequestMatch(
					mock.PatchReposPullsByOwnerByRepoByPullNumber,
					github.PullRequest{
						HTMLURL: github.Ptr("https://github.com/testowner/testrepo/pull/15"),
					},
				),
			},
			expectedURL: "https://github.com/testowner/testrepo/pull/15",
		},
		{
			name:     "pull request update error",
			owner:    "testowner",
			repo:     "testrepo",
			prNumber: 15,
			title:    "Updated PR Title",
			body:     "Updated PR body",
			mockOptions: []mock.MockBackendOption{
				mock.WithRequestMatchHandler(
					mock.PatchReposPullsByOwnerByRepoByPullNumber,
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						mock.WriteError(w, http.StatusNotFound, "Not Found")
					}),
				),
			},
			expectedError: "failed to update pull request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := createTestClient(tt.mockOptions...)

			ctx := context.Background()
			url, err := client.updatePullRequest(ctx, tt.owner, tt.repo, tt.prNumber, tt.title, tt.body)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedURL, url)
		})
	}
}
