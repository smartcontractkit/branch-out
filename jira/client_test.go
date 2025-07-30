package jira

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	go_jira "github.com/andygrunwald/go-jira"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/config"
	"github.com/smartcontractkit/branch-out/internal/testhelpers"
	"github.com/smartcontractkit/branch-out/trunk"
)

// setupTestClient creates a test client with mocked services
func setupTestClient(t *testing.T, mockIssueService *mockIssueService, mockFieldService *mockFieldService) *Client {
	client, err := NewClient(
		WithLogger(testhelpers.Logger(t)),
		WithConfig(config.Config{
			Jira: config.Jira{
				BaseDomain: "test.atlassian.net",
				ProjectKey: "TEST",
				Username:   "testuser",
				Token:      "testtoken",
			},
		}),
		WithServices(mockIssueService, mockFieldService),
	)
	require.NoError(t, err)
	return client
}

func TestNewClient(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		authType   string
		jiraConfig config.Jira
		err        error
	}{
		{
			name:     "basic auth only",
			authType: "Basic",
			jiraConfig: config.Jira{
				ProjectKey: "TEST",
				BaseDomain: "test.atlassian.net",
				Username:   "test",
				Token:      "test",
			},
		},
		{
			name:     "oauth only",
			authType: "OAuth",
			jiraConfig: config.Jira{
				ProjectKey:        "TEST",
				BaseDomain:        "test.atlassian.net",
				OAuthAccessToken:  "test",
				OAuthRefreshToken: "test",
				OAuthClientID:     "test",
				OAuthClientSecret: "test",
			},
		},
		{
			name:     "oauth overrides basic auth",
			authType: "OAuth",
			jiraConfig: config.Jira{
				ProjectKey:        "TEST",
				BaseDomain:        "test.atlassian.net",
				Username:          "test",
				Token:             "test",
				OAuthAccessToken:  "test",
				OAuthRefreshToken: "test",
				OAuthClientID:     "test",
				OAuthClientSecret: "test",
			},
		},
		{
			name:     "no auth",
			authType: "No Auth",
			jiraConfig: config.Jira{
				ProjectKey: "TEST",
				BaseDomain: "test.atlassian.net",
			},
			err: ErrNoAuthCredentialsProvided,
		},
		{
			name: "no base domain",
			jiraConfig: config.Jira{
				ProjectKey: "TEST",
				Username:   "test",
				Token:      "test",
			},
			err: ErrBaseDomainRequired,
		},
		{
			name: "no project key",
			jiraConfig: config.Jira{
				BaseDomain: "test.atlassian.net",
				Username:   "test",
				Token:      "test",
			},
			err: ErrProjectKeyRequired,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client, err := NewClient(
				WithLogger(testhelpers.Logger(t)),
				WithConfig(config.Config{
					Jira: tc.jiraConfig,
				}),
			)

			if tc.err != nil {
				require.Error(t, err, "expected error")
				require.Nil(t, client, "expected nil client with error")
				require.ErrorIs(t, err, tc.err, "expected a specific error")
			} else {
				require.NoError(t, err, "expected no error")
				require.NotNil(t, client, "expected client")
				require.Equal(t, tc.authType, client.AuthType(), "expected specific auth type")
			}
		})
	}
}

func TestCreateFlakyTestIssue(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		jiraConfig    config.Jira
		returnIssue   *go_jira.Issue
		expectedError error
	}{
		{
			name: "custom fields",
			jiraConfig: config.Jira{
				ProjectKey:     "TEST",
				BaseDomain:     "test.atlassian.net",
				Username:       "test",
				Token:          "test",
				TestFieldID:    "customfield_1",
				PackageFieldID: "customfield_2",
				TrunkIDFieldID: "customfield_3",
			},
			returnIssue: &go_jira.Issue{
				Key: "TEST-123",
				Fields: &go_jira.IssueFields{
					Unknowns: map[string]any{
						"customfield_1": "TestCreateFlakyTestIssue",
						"customfield_2": "github.com/smartcontractkit/branch-out/jira",
						"customfield_3": "123",
					},
				},
			},
			expectedError: nil,
		},
		{
			name: "no custom fields",
			jiraConfig: config.Jira{
				ProjectKey: "TEST",
				BaseDomain: "test.atlassian.net",
				Username:   "test",
				Token:      "test",
			},
			returnIssue: &go_jira.Issue{
				Key: "TEST-123",
			},
		},
		{
			name: "error",
			jiraConfig: config.Jira{
				ProjectKey: "TEST",
				BaseDomain: "test.atlassian.net",
				Username:   "test",
				Token:      "test",
			},
			expectedError: errors.New("error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mockIssueService := newMockIssueService(t)
			mockFieldService := newMockFieldService(t)

			if tc.jiraConfig.TestFieldID != "" || tc.jiraConfig.PackageFieldID != "" ||
				tc.jiraConfig.TrunkIDFieldID != "" {
				mockFieldService.EXPECT().GetList().Return([]go_jira.Field{
					{ID: tc.jiraConfig.TestFieldID, Name: "Test Field"},
					{ID: tc.jiraConfig.PackageFieldID, Name: "Package Field"},
					{ID: tc.jiraConfig.TrunkIDFieldID, Name: "Trunk ID Field"},
				}, nil, nil)
				mockIssueService.EXPECT().Update(mock.Anything).Return(
					tc.returnIssue,
					nil,
					tc.expectedError,
				)
			}

			mockIssueService.EXPECT().Create(mock.Anything).Return(
				tc.returnIssue,
				nil,
				tc.expectedError,
			)

			client, err := NewClient(
				WithLogger(testhelpers.Logger(t)),
				WithConfig(config.Config{
					Jira: tc.jiraConfig,
				}),
				WithServices(mockIssueService, mockFieldService),
			)
			require.NoError(t, err, "error creating client")

			flakyTestIssue, err := client.CreateFlakyTestIssue(FlakyTestIssueRequest{
				ProjectKey: tc.jiraConfig.ProjectKey,
				RepoURL:    "https://github.com/smartcontractkit/branch-out",
				Package:    "github.com/smartcontractkit/branch-out/jira",
				Test:       "TestCreateFlakyTestIssue",
			})

			if tc.expectedError != nil {
				require.Error(t, err, "expected error")
				require.ErrorIs(t, err, tc.expectedError, "expected specific error")
			} else {
				require.NoError(t, err)
				require.NotNil(t, flakyTestIssue)
				require.Equal(t, tc.returnIssue.Key, flakyTestIssue.Key)
			}
		})
	}
}
func TestGetOpenFlakyTestIssues(t *testing.T) {
	t.Parallel()

	jiraConfig := config.Jira{
		ProjectKey: "TEST",
		BaseDomain: "test.atlassian.net",
		Username:   "test",
		Token:      "test",
	}

	testCases := []struct {
		name          string
		issues        []go_jira.Issue
		expectedError error
	}{
		{
			name: "many issues",
			issues: []go_jira.Issue{
				{Key: "TEST-123"},
				{Key: "TEST-456"},
				{Key: "TEST-789"},
			},
		},
		{
			name:   "no issues",
			issues: []go_jira.Issue{},
		},
		{
			name: "one issue",
			issues: []go_jira.Issue{
				{Key: "TEST-123"},
			},
		},
		{
			name: "error",
			issues: []go_jira.Issue{
				{Key: "TEST-123"},
			},
			expectedError: errors.New("error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mockIssueService := newMockIssueService(t)
			mockFieldService := newMockFieldService(t)

			mockIssueService.EXPECT().Search(mock.Anything, mock.Anything).Return(
				tc.issues,
				nil,
				tc.expectedError,
			)

			client, err := NewClient(
				WithLogger(testhelpers.Logger(t)),
				WithConfig(config.Config{
					Jira: jiraConfig,
				}),
				WithServices(mockIssueService, mockFieldService),
			)
			require.NoError(t, err, "error creating client")

			flakyTestIssues, err := client.GetOpenFlakyTestIssues()
			if tc.expectedError != nil {
				require.Error(t, err, "expected error")
				require.ErrorIs(t, err, tc.expectedError, "expected specific error")
			} else {
				require.NoError(t, err)
				require.NotNil(t, flakyTestIssues)
				require.Len(t, flakyTestIssues, len(tc.issues))
				for i, issue := range flakyTestIssues {
					require.Equal(t, tc.issues[i].Key, issue.Key)
				}
			}
		})
	}
}

func TestGetOpenFlakyTestIssue(t *testing.T) {
	t.Parallel()

	defaultJiraConfig := config.Jira{
		ProjectKey: "TEST",
		BaseDomain: "test.atlassian.net",
		Username:   "test",
		Token:      "test",
	}

	testCases := []struct {
		name          string
		issues        []go_jira.Issue
		jiraConfig    config.Jira
		expectedError error
	}{
		{
			name: "using custom fields",
			jiraConfig: config.Jira{
				ProjectKey:     "TEST",
				BaseDomain:     "test.atlassian.net",
				Username:       "test",
				Token:          "test",
				TestFieldID:    "customfield_1",
				PackageFieldID: "customfield_2",
				TrunkIDFieldID: "customfield_3",
			},
			issues: []go_jira.Issue{
				{
					Key: "TEST-123",
					Fields: &go_jira.IssueFields{
						Unknowns: map[string]any{
							"customfield_1": "TestExample",
							"customfield_2": "github.com/smartcontractkit/branch-out/jira",
							"customfield_3": "123",
						},
					},
				},
			},
		},
		{
			name:       "using summary fallback",
			jiraConfig: defaultJiraConfig,
			issues: []go_jira.Issue{
				{
					Key: "TEST-123",
					Fields: &go_jira.IssueFields{
						Summary: "Flaky Test: github.com/smartcontractkit/branch-out/jira.TestExample",
					},
				},
			},
		},
		{
			name:       "multiple issues",
			jiraConfig: defaultJiraConfig,
			issues: []go_jira.Issue{
				{Key: "TEST-123"},
				{Key: "TEST-456"},
			},
		},
		{
			name:       "error",
			jiraConfig: defaultJiraConfig,
			issues: []go_jira.Issue{
				{Key: "TEST-123"},
			},
			expectedError: errors.New("error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mockIssueService := newMockIssueService(t)
			mockFieldService := newMockFieldService(t)

			if tc.jiraConfig.TestFieldID != "" || tc.jiraConfig.PackageFieldID != "" ||
				tc.jiraConfig.TrunkIDFieldID != "" {
				mockFieldService.EXPECT().GetList().Return([]go_jira.Field{
					{ID: tc.jiraConfig.TestFieldID, Name: "Test Field"},
					{ID: tc.jiraConfig.PackageFieldID, Name: "Package Field"},
					{ID: tc.jiraConfig.TrunkIDFieldID, Name: "Trunk ID Field"},
				}, nil, nil)
			}

			mockIssueService.EXPECT().Search(mock.Anything, mock.Anything).Return(
				tc.issues,
				nil,
				tc.expectedError,
			)

			client, err := NewClient(
				WithLogger(testhelpers.Logger(t)),
				WithConfig(config.Config{
					Jira: tc.jiraConfig,
				}),
				WithServices(mockIssueService, mockFieldService),
			)
			require.NoError(t, err, "error creating client")

			flakyTestIssue, err := client.GetOpenFlakyTestIssue(
				"github.com/smartcontractkit/branch-out/jira",
				"TestExample",
			)
			if tc.expectedError != nil {
				require.Error(t, err, "expected error")
				require.ErrorIs(t, err, tc.expectedError, "expected specific error")
			} else {
				require.NoError(t, err)
				require.NotNil(t, flakyTestIssue)
				require.Equal(t, tc.issues[0].Key, flakyTestIssue.Key)
			}
		})
	}
}
func TestExtractFromSummary(t *testing.T) {
	t.Parallel()

	// Create a minimal client for testing the method
	client, err := NewClient(
		WithLogger(testhelpers.Logger(t)),
		WithConfig(config.Config{
			Jira: config.Jira{
				ProjectKey: "TEST",
				BaseDomain: "test.atlassian.net",
				Username:   "test",
				Token:      "test",
			},
		}),
		WithServices(newMockIssueService(t), newMockFieldService(t)),
	)
	require.NoError(t, err)

	testCases := []struct {
		name            string
		summary         string
		expectedTest    string
		expectedPackage string
	}{
		{
			name:            "valid flaky test summary",
			summary:         "Flaky Test: github.com/smartcontractkit/branch-out/jira.TestExample",
			expectedTest:    "TestExample",
			expectedPackage: "github.com/smartcontractkit/branch-out/jira",
		},
		{
			name:            "summary without flaky test prefix",
			summary:         "github.com/smartcontractkit/branch-out/jira.TestExample",
			expectedTest:    "TestExample",
			expectedPackage: "github.com/smartcontractkit/branch-out/jira",
		},
		{
			name:    "summary without dots",
			summary: "Flaky Test: InvalidFormat",
		},
		{
			name:    "empty summary",
			summary: "",
		},
		{
			name:            "summary with multiple dots",
			summary:         "Flaky Test: github.com/test/package/subpackage.TestName",
			expectedTest:    "TestName",
			expectedPackage: "github.com/test/package/subpackage",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			issue := &FlakyTestIssue{}
			client.extractFromSummary(issue, tc.summary)

			require.Equal(t, tc.expectedTest, issue.Test)
			require.Equal(t, tc.expectedPackage, issue.Package)
		})
	}
}
func TestCheckResponse(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		response      *go_jira.Response
		expectedError bool
		errorContains string
	}{
		{
			name:     "nil response",
			response: nil,
		},
		{
			name: "successful response",
			response: &go_jira.Response{
				Response: &http.Response{
					StatusCode: 200,
				},
			},
		},
		{
			name: "created response",
			response: &go_jira.Response{
				Response: &http.Response{
					StatusCode: 201,
				},
			},
		},
		{
			name: "client error response",
			response: &go_jira.Response{
				Response: &http.Response{
					StatusCode: 400,
					Body:       io.NopCloser(strings.NewReader(`{"error": "Bad Request"}`)),
				},
			},
			expectedError: true,
			errorContains: "jira API error (status 400)",
		},
		{
			name: "server error response",
			response: &go_jira.Response{
				Response: &http.Response{
					StatusCode: 500,
					Body:       io.NopCloser(strings.NewReader(`{"error": "Internal Server Error"}`)),
				},
			},
			expectedError: true,
			errorContains: "jira API error (status 500)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := checkResponse(tc.response)

			if tc.expectedError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.errorContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestClient_CloseIssue(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		issueKey      string
		closeComment  string
		setupMocks    func(*mockIssueService)
		expectedError bool
		errorContains string
	}{
		{
			name:         "successful close with comment",
			issueKey:     "TEST-123",
			closeComment: "Test is now healthy",
			setupMocks: func(mockService *mockIssueService) {
				// Mock AddComment call
				mockService.EXPECT().AddComment("TEST-123", mock.MatchedBy(func(comment *go_jira.Comment) bool {
					return comment.Body == "Test is now healthy"
				})).Return(&go_jira.Comment{}, &go_jira.Response{Response: &http.Response{StatusCode: 201}}, nil)

				// Mock GetTransitions call
				transitions := []go_jira.Transition{
					{
						ID:   "2",
						Name: "Close Issue",
						To: go_jira.Status{
							Name: "Closed",
						},
					},
				}
				mockService.EXPECT().GetTransitions("TEST-123").Return(transitions, &go_jira.Response{Response: &http.Response{StatusCode: 200}}, nil)

				// Mock DoTransition call
				mockService.EXPECT().DoTransition("TEST-123", "2").Return(&go_jira.Response{Response: &http.Response{StatusCode: 204}}, nil)
			},
			expectedError: false,
		},
		{
			name:         "successful close without comment",
			issueKey:     "TEST-123",
			closeComment: "",
			setupMocks: func(mockService *mockIssueService) {
				// Mock GetTransitions call
				transitions := []go_jira.Transition{
					{
						ID:   "3",
						Name: "Resolve",
						To: go_jira.Status{
							Name: "Resolved",
						},
					},
				}
				mockService.EXPECT().GetTransitions("TEST-123").Return(transitions, &go_jira.Response{Response: &http.Response{StatusCode: 200}}, nil)

				// Mock DoTransition call
				mockService.EXPECT().DoTransition("TEST-123", "3").Return(&go_jira.Response{Response: &http.Response{StatusCode: 204}}, nil)
			},
			expectedError: false,
		},
		{
			name:         "successful close with \"Done\" transition",
			issueKey:     "TEST-123",
			closeComment: "",
			setupMocks: func(mockService *mockIssueService) {
				// Mock GetTransitions call with "Done" transition
				transitions := []go_jira.Transition{
					{
						ID:   "4",
						Name: "Mark as Done",
						To: go_jira.Status{
							Name: "Done",
						},
					},
				}
				mockService.EXPECT().GetTransitions("TEST-123").Return(transitions, &go_jira.Response{Response: &http.Response{StatusCode: 200}}, nil)

				// Mock DoTransition call
				mockService.EXPECT().DoTransition("TEST-123", "4").Return(&go_jira.Response{Response: &http.Response{StatusCode: 204}}, nil)
			},
			expectedError: false,
		},
		{
			name:          "error - empty issue key",
			issueKey:      "",
			closeComment:  "Test comment",
			setupMocks:    func(mockService *mockIssueService) {},
			expectedError: true,
			errorContains: "issue key is required",
		},
		{
			name:         "comment fails but close succeeds (non-blocking comment)",
			issueKey:     "TEST-123",
			closeComment: "Test comment",
			setupMocks: func(mockService *mockIssueService) {
				// Mock AddComment call with error (should be non-blocking)
				mockService.EXPECT().AddComment("TEST-123", mock.Anything).Return(nil, nil, errors.New("API down"))

				// Mock GetTransitions call (should still be called)
				transitions := []go_jira.Transition{
					{
						ID:   "2",
						Name: "Close Issue",
						To: go_jira.Status{
							Name: "Closed",
						},
					},
				}
				mockService.EXPECT().GetTransitions("TEST-123").Return(transitions, &go_jira.Response{Response: &http.Response{StatusCode: 200}}, nil)

				// Mock DoTransition call (should still be called)
				mockService.EXPECT().DoTransition("TEST-123", "2").Return(&go_jira.Response{Response: &http.Response{StatusCode: 204}}, nil)
			},
			expectedError: false, // Comment failure should be non-blocking
		},
		{
			name:         "comment bad response but close succeeds (non-blocking comment)",
			issueKey:     "TEST-123",
			closeComment: "Test comment",
			setupMocks: func(mockService *mockIssueService) {
				// Mock AddComment call with bad status (should be non-blocking)
				mockService.EXPECT().AddComment("TEST-123", mock.Anything).Return(
					&go_jira.Comment{},
					&go_jira.Response{Response: &http.Response{
						StatusCode: 400,
						Body:       io.NopCloser(strings.NewReader(`{"error": "Bad Request"}`)),
					}},
					nil,
				)

				// Mock GetTransitions call (should still be called)
				transitions := []go_jira.Transition{
					{
						ID:   "2",
						Name: "Close Issue",
						To: go_jira.Status{
							Name: "Closed",
						},
					},
				}
				mockService.EXPECT().GetTransitions("TEST-123").Return(transitions, &go_jira.Response{Response: &http.Response{StatusCode: 200}}, nil)

				// Mock DoTransition call (should still be called)
				mockService.EXPECT().DoTransition("TEST-123", "2").Return(&go_jira.Response{Response: &http.Response{StatusCode: 204}}, nil)
			},
			expectedError: false, // Comment failure should be non-blocking
		},
		{
			name:         "error - get transitions fails",
			issueKey:     "TEST-123",
			closeComment: "Test comment",
			setupMocks: func(mockService *mockIssueService) {
				// Mock AddComment call
				mockService.EXPECT().AddComment("TEST-123", mock.Anything).Return(&go_jira.Comment{}, &go_jira.Response{Response: &http.Response{StatusCode: 201}}, nil)

				// Mock GetTransitions call with error
				mockService.EXPECT().GetTransitions("TEST-123").Return(nil, nil, errors.New("API error"))
			},
			expectedError: true,
			errorContains: "jira get_transitions operation failed for issue TEST-123",
		},
		{
			name:         "error - no close transition available",
			issueKey:     "TEST-123",
			closeComment: "Test comment",
			setupMocks: func(mockService *mockIssueService) {
				// Mock AddComment call
				mockService.EXPECT().AddComment("TEST-123", mock.Anything).Return(&go_jira.Comment{}, &go_jira.Response{Response: &http.Response{StatusCode: 201}}, nil)

				// Mock GetTransitions call with no close transitions
				transitions := []go_jira.Transition{
					{
						ID:   "1",
						Name: "Start Progress",
						To: go_jira.Status{
							Name: "In Progress",
						},
					},
				}
				mockService.EXPECT().GetTransitions("TEST-123").Return(transitions, &go_jira.Response{Response: &http.Response{StatusCode: 200}}, nil)
			},
			expectedError: true,
			errorContains: "no close transition available for issue TEST-123",
		},
		{
			name:         "error - do transition fails",
			issueKey:     "TEST-123",
			closeComment: "Test comment",
			setupMocks: func(mockService *mockIssueService) {
				// Mock AddComment call
				mockService.EXPECT().AddComment("TEST-123", mock.Anything).Return(&go_jira.Comment{}, &go_jira.Response{Response: &http.Response{StatusCode: 201}}, nil)

				// Mock GetTransitions call
				transitions := []go_jira.Transition{
					{
						ID:   "2",
						Name: "Close Issue",
						To: go_jira.Status{
							Name: "Closed",
						},
					},
				}
				mockService.EXPECT().GetTransitions("TEST-123").Return(transitions, &go_jira.Response{Response: &http.Response{StatusCode: 200}}, nil)

				// Mock DoTransition call with error
				mockService.EXPECT().DoTransition("TEST-123", "2").Return(nil, errors.New("transition failed"))
			},
			expectedError: true,
			errorContains: "jira do_transition operation failed for issue TEST-123",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mockIssueService := newMockIssueService(t)
			mockFieldService := newMockFieldService(t)

			tc.setupMocks(mockIssueService)

			client := setupTestClient(t, mockIssueService, mockFieldService)

			err := client.CloseIssue(tc.issueKey, tc.closeComment)

			if tc.expectedError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.errorContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestClient_AddCommentToIssue(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		issueKey      string
		commentBody   string
		setupMocks    func(*mockIssueService)
		expectedError bool
		errorContains string
	}{
		{
			name:        "successful comment addition",
			issueKey:    "TEST-123",
			commentBody: "This is a test comment",
			setupMocks: func(mockService *mockIssueService) {
				mockService.EXPECT().AddComment("TEST-123", mock.MatchedBy(func(comment *go_jira.Comment) bool {
					return comment.Body == "This is a test comment"
				})).Return(&go_jira.Comment{}, &go_jira.Response{Response: &http.Response{StatusCode: 201}}, nil)
			},
			expectedError: false,
		},
		{
			name:          "error - empty issue key",
			issueKey:      "",
			commentBody:   "Test comment",
			setupMocks:    func(mockService *mockIssueService) {},
			expectedError: true,
			errorContains: "issue key is required",
		},
		{
			name:          "error - empty comment body",
			issueKey:      "TEST-123",
			commentBody:   "",
			setupMocks:    func(mockService *mockIssueService) {},
			expectedError: true,
			errorContains: "comment body is required",
		},
		{
			name:        "error - API call fails",
			issueKey:    "TEST-123",
			commentBody: "Test comment",
			setupMocks: func(mockService *mockIssueService) {
				mockService.EXPECT().AddComment("TEST-123", mock.Anything).Return(nil, nil, errors.New("API error"))
			},
			expectedError: true,
			errorContains: "jira add_comment operation failed for issue TEST-123",
		},
		{
			name:        "error - bad response status",
			issueKey:    "TEST-123",
			commentBody: "Test comment",
			setupMocks: func(mockService *mockIssueService) {
				mockService.EXPECT().AddComment("TEST-123", mock.Anything).Return(
					&go_jira.Comment{},
					&go_jira.Response{Response: &http.Response{
						StatusCode: 400,
						Body:       io.NopCloser(strings.NewReader(`{"error": "Bad Request"}`)),
					}},
					nil,
				)
			},
			expectedError: true,
			errorContains: "jira add_comment operation failed for issue TEST-123",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mockIssueService := newMockIssueService(t)
			mockFieldService := newMockFieldService(t)

			tc.setupMocks(mockIssueService)

			client := setupTestClient(t, mockIssueService, mockFieldService)

			err := client.AddCommentToIssue(tc.issueKey, tc.commentBody)

			if tc.expectedError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.errorContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestClient_AddCommentToFlakyTestIssue(t *testing.T) {
	t.Parallel()

	// Create test data for status changes
	createStatusChange := func(currentStatus, previousStatus string) trunk.TestCaseStatusChange {
		return trunk.TestCaseStatusChange{
			TestCase: trunk.TestCase{
				ID:                         "test-123",
				Name:                       "TestExample",
				TestSuite:                  "github.com/smartcontractkit/branch-out/jira",
				FailureRateLast7D:          45.6,
				PullRequestsImpactedLast7D: 7,
				Variant:                    "linux-amd64",
				HTMLURL:                    "https://app.trunk.io/test-123",
			},
			StatusChange: trunk.StatusChange{
				CurrentStatus: trunk.Status{
					Value: currentStatus,
				},
				PreviousStatus: previousStatus,
			},
		}
	}

	testCases := []struct {
		name          string
		issue         FlakyTestIssue
		statusChange  trunk.TestCaseStatusChange
		setupMocks    func(*mockIssueService, *string) // Pass pointer to capture comment body
		expectedError bool
		errorContains string
	}{
		{
			name: "healthy status comment",
			issue: FlakyTestIssue{
				Issue: &go_jira.Issue{Key: "TEST-123"},
			},
			statusChange: createStatusChange(trunk.TestCaseStatusHealthy, trunk.TestCaseStatusFlaky),
			setupMocks: func(mockService *mockIssueService, capturedBody *string) {
				mockService.EXPECT().AddComment("TEST-123", mock.MatchedBy(func(comment *go_jira.Comment) bool {
					*capturedBody = comment.Body // Capture the comment body
					return true
				})).Return(&go_jira.Comment{}, &go_jira.Response{Response: &http.Response{StatusCode: 201}}, nil)
			},
			expectedError: false,
		},
		{
			name: "flaky status comment",
			issue: FlakyTestIssue{
				Issue: &go_jira.Issue{Key: "TEST-456"},
			},
			statusChange: createStatusChange(trunk.TestCaseStatusFlaky, trunk.TestCaseStatusHealthy),
			setupMocks: func(mockService *mockIssueService, capturedBody *string) {
				mockService.EXPECT().AddComment("TEST-456", mock.MatchedBy(func(comment *go_jira.Comment) bool {
					*capturedBody = comment.Body
					return true
				})).Return(&go_jira.Comment{}, &go_jira.Response{Response: &http.Response{StatusCode: 201}}, nil)
			},
			expectedError: false,
		},
		{
			name: "broken status comment",
			issue: FlakyTestIssue{
				Issue: &go_jira.Issue{Key: "TEST-789"},
			},
			statusChange: createStatusChange(trunk.TestCaseStatusBroken, trunk.TestCaseStatusFlaky),
			setupMocks: func(mockService *mockIssueService, capturedBody *string) {
				mockService.EXPECT().AddComment("TEST-789", mock.MatchedBy(func(comment *go_jira.Comment) bool {
					*capturedBody = comment.Body
					return true
				})).Return(&go_jira.Comment{}, &go_jira.Response{Response: &http.Response{StatusCode: 201}}, nil)
			},
			expectedError: false,
		},
		{
			name: "unknown status comment",
			issue: FlakyTestIssue{
				Issue: &go_jira.Issue{Key: "TEST-999"},
			},
			statusChange: createStatusChange("unknown", trunk.TestCaseStatusHealthy),
			setupMocks: func(mockService *mockIssueService, capturedBody *string) {
				mockService.EXPECT().AddComment("TEST-999", mock.MatchedBy(func(comment *go_jira.Comment) bool {
					*capturedBody = comment.Body
					return true
				})).Return(&go_jira.Comment{}, &go_jira.Response{Response: &http.Response{StatusCode: 201}}, nil)
			},
			expectedError: false,
		},
		{
			name: "error - add comment fails",
			issue: FlakyTestIssue{
				Issue: &go_jira.Issue{Key: "TEST-ERROR"},
			},
			statusChange: createStatusChange(trunk.TestCaseStatusHealthy, trunk.TestCaseStatusFlaky),
			setupMocks: func(mockService *mockIssueService, capturedBody *string) {
				mockService.EXPECT().AddComment("TEST-ERROR", mock.Anything).Return(nil, nil, errors.New("API error"))
			},
			expectedError: true,
			errorContains: "failed to add status comment to flaky test issue TEST-ERROR",
		},
		{
			name: "error - bad response status",
			issue: FlakyTestIssue{
				Issue: &go_jira.Issue{Key: "TEST-BAD"},
			},
			statusChange: createStatusChange(trunk.TestCaseStatusFlaky, trunk.TestCaseStatusHealthy),
			setupMocks: func(mockService *mockIssueService, capturedBody *string) {
				mockService.EXPECT().AddComment("TEST-BAD", mock.Anything).Return(
					&go_jira.Comment{},
					&go_jira.Response{Response: &http.Response{
						StatusCode: 500,
						Body:       io.NopCloser(strings.NewReader(`{"error": "Internal Server Error"}`)),
					}},
					nil,
				)
			},
			expectedError: true,
			errorContains: "failed to add status comment to flaky test issue TEST-BAD",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mockIssueService := newMockIssueService(t)
			mockFieldService := newMockFieldService(t)

			var capturedCommentBody string
			tc.setupMocks(mockIssueService, &capturedCommentBody)

			client := setupTestClient(t, mockIssueService, mockFieldService)

			err := client.AddCommentToFlakyTestIssue(tc.issue, tc.statusChange)

			if tc.expectedError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.errorContains)
			} else {
				require.NoError(t, err)

				// Validate the captured comment body based on the test case
				switch tc.statusChange.StatusChange.CurrentStatus.Value {
				case trunk.TestCaseStatusHealthy:
					require.Contains(t, capturedCommentBody, "*Test Status Update: HEALTHY* ✅")
					require.Contains(t, capturedCommentBody, "The test has recovered and is now healthy!")
					require.Contains(t, capturedCommentBody, "flaky → healthy")
					require.Contains(t, capturedCommentBody, "45.6%")
					require.Contains(t, capturedCommentBody, "7")
					require.Contains(t, capturedCommentBody, "https://app.trunk.io/test-123")
					require.Contains(t, capturedCommentBody, "This ticket should be closed")
				case trunk.TestCaseStatusFlaky:
					require.Contains(t, capturedCommentBody, "*Test Status Update: FLAKY* ⚠️")
					require.Contains(t, capturedCommentBody, "Another flaky occurrence has been detected")
					require.Contains(t, capturedCommentBody, "healthy → flaky")
					require.Contains(t, capturedCommentBody, "github.com/smartcontractkit/branch-out/jira")
					require.Contains(t, capturedCommentBody, "linux-amd64")
				case trunk.TestCaseStatusBroken:
					require.Contains(t, capturedCommentBody, "*Test Status Update: BROKEN* ❌")
					require.Contains(t, capturedCommentBody, "The test status has changed to broken")
					require.Contains(t, capturedCommentBody, "flaky → broken")
				default:
					require.Contains(t, capturedCommentBody, "*Test Status Update: UNKNOWN*")
					require.Contains(t, capturedCommentBody, "The test status has been updated")
					require.Contains(t, capturedCommentBody, "healthy → unknown")
				}
			}
		})
	}
}
