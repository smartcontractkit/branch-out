package jira

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/config"
	"github.com/smartcontractkit/branch-out/internal/testhelpers"
)

var standardJiraConfig = config.Jira{
	ProjectKey: "TEST",
	Username:   "test",
	Token:      "test",
}

func TestNewClient_BasicAuth(t *testing.T) {
	t.Parallel()

	cfg := standardJiraConfig
	cfg.BaseDomain = "test.atlassian.net"

	client, err := NewClient(WithLogger(testhelpers.Logger(t)), WithConfig(config.Config{
		Jira: cfg,
	}))
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, "Basic", client.AuthType())
}

func TestNewClient_TokenAuth(t *testing.T) {
	t.Parallel()

	jiraConfig := config.Jira{
		ProjectKey:        "TEST",
		BaseDomain:        "test.atlassian.net",
		OAuthAccessToken:  "test",
		OAuthRefreshToken: "test",
		OAuthClientID:     "test",
		OAuthClientSecret: "test",
	}

	client, err := NewClient(WithLogger(testhelpers.Logger(t)), WithConfig(config.Config{
		Jira: jiraConfig,
	}))
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, "OAuth", client.AuthType())
}
