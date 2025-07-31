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
)

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
