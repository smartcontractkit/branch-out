package jira

import (
	"testing"

	go_jira "github.com/andygrunwald/go-jira"
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

func TestCreateFlakyTestIssue_Success(t *testing.T) {
	t.Parallel()

	jiraConfig := config.Jira{
		ProjectKey: "TEST",
		BaseDomain: "test.atlassian.net",
		Username:   "test",
		Token:      "test",
	}
	mockIssueService := newMockIssueService(t)
	mockFieldService := newMockFieldService(t)

	client, err := NewClient(
		WithLogger(testhelpers.Logger(t)),
		WithConfig(config.Config{
			Jira: jiraConfig,
		}),
		WithServices(
			mockIssueService,
			mockFieldService,
		),
	)
	require.NoError(t, err, "error creating client")

	flakyTestIssueRequest := FlakyTestIssueRequest{
		ProjectKey:        jiraConfig.ProjectKey,
		RepoURL:           "https://github.com/smartcontractkit/branch-out",
		Package:           "github.com/smartcontractkit/branch-out/jira",
		Test:              "TestCreateFlakyTestIssue",
		FilePath:          "test.go",
		TrunkID:           "123",
		AdditionalDetails: "test",
	}

	issue := &go_jira.Issue{
		Key: "TEST-123",
	}

	mockIssueService.EXPECT().Create(flakyTestIssueRequest.toJiraIssue()).Return(
		issue,
		nil,
		nil,
	)

	flakyTestIssue, err := client.CreateFlakyTestIssue(flakyTestIssueRequest)
	require.NoError(t, err)
	require.NotNil(t, flakyTestIssue)
	require.Equal(t, issue.Key, flakyTestIssue.Key)
}
