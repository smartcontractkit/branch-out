package main

import (
	"log"
	"os"

	"github.com/rs/zerolog"
	"github.com/smartcontractkit/branch-out/jira"
)

func main() {
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	// Smart authentication detection - automatically uses OAuth if available, falls back to Basic Auth
	// Set these environment variables based on your preferred auth method:
	//
	// For OAuth (recommended):
	// export JIRA_BASE_DOMAIN="your-company.atlassian.net"
	// export JIRA_OAUTH_CLIENT_ID="your-oauth-client-id"
	// export JIRA_OAUTH_CLIENT_SECRET="your-oauth-client-secret"
	// export JIRA_OAUTH_ACCESS_TOKEN="your-oauth-access-token"
	// export JIRA_OAUTH_REFRESH_TOKEN="your-oauth-refresh-token"
	//
	// For Basic Auth (easier setup):
	// export JIRA_BASE_DOMAIN="your-company.atlassian.net"
	// export JIRA_USERNAME="your-email@company.com"
	// export JIRA_API_TOKEN="your-api-token"

	// Check if required base domain is set
	if os.Getenv("JIRA_BASE_DOMAIN") == "" {
		log.Fatal(`JIRA_BASE_DOMAIN environment variable is required.

Please set up your environment variables first:

For Basic Auth (easiest):
export JIRA_BASE_DOMAIN="your-company.atlassian.net"
export JIRA_USERNAME="your-email@company.com"
export JIRA_API_TOKEN="your-api-token"

For OAuth:
export JIRA_BASE_DOMAIN="your-company.atlassian.net"
export JIRA_OAUTH_CLIENT_ID="your-client-id"
export JIRA_OAUTH_CLIENT_SECRET="your-client-secret"
export JIRA_OAUTH_ACCESS_TOKEN="your-access-token"
export JIRA_OAUTH_REFRESH_TOKEN="your-refresh-token"`)
	}

	// Detect which credentials are available
	hasOAuth := os.Getenv("JIRA_OAUTH_CLIENT_ID") != "" &&
		os.Getenv("JIRA_OAUTH_CLIENT_SECRET") != "" &&
		os.Getenv("JIRA_OAUTH_ACCESS_TOKEN") != ""
	hasBasicAuth := os.Getenv("JIRA_USERNAME") != "" && os.Getenv("JIRA_API_TOKEN") != ""

	if !hasOAuth && !hasBasicAuth {
		log.Fatal(`No authentication credentials found. Please set either:
OAuth: JIRA_OAUTH_CLIENT_ID, JIRA_OAUTH_CLIENT_SECRET, JIRA_OAUTH_ACCESS_TOKEN
OR
Basic Auth: JIRA_USERNAME, JIRA_API_TOKEN`)
	}

	// Show which auth method will be used
	if hasOAuth {
		logger.Info().Msg("OAuth credentials detected - will use OAuth authentication")
	} else {
		logger.Info().Msg("Basic Auth credentials detected - will use Basic authentication")
	}

	// Create Jira client (it will automatically choose the right auth method)
	projectKey := os.Getenv("JIRA_PROJECT_KEY")
	if projectKey == "" {
		projectKey = "DX" // Default project key
		logger.Info().Str("project_key", projectKey).Msg("Using default project key. Set JIRA_PROJECT_KEY to override.")
	}

	jiraClient, err := jira.NewClient(logger, projectKey)
	if err != nil {
		log.Fatalf("Failed to create Jira client: %v", err)
	}

	logger.Info().
		Str("auth_type", jiraClient.AuthType()).
		Bool("oauth_enabled", jiraClient.IsOAuthEnabled()).
		Str("project_key", projectKey).
		Msg("Jira client created successfully")

	// Example flaky test data
	req := jira.FlakyTestTicketRequest{
		RepoName:        "smartcontractkit/chainlink",
		TestPackageName: "TestFlakyConnection",
		FilePath:        "core/services/chainlink/test_helpers_test.go",
		TrunkID:         "trunk-abc-123",
		Details:         `{"failure_rate_last_7d": 0.15, "pull_requests_impacted_last_7d": 5, "most_common_failure": "connection timeout", "codeowners": ["@chainlink/qa"], "last_occurrence": "2025-06-30T14:30:00Z"}`,
	}

	logger.Info().
		Str("repo_name", req.RepoName).
		Str("test_name", req.TestPackageName).
		Str("file_path", req.FilePath).
		Msg("Creating Jira ticket with flaky test details")

	// Create the ticket
	ticket, err := jiraClient.CreateFlakyTestTicket(req)
	if err != nil {
		log.Fatalf("Failed to create Jira ticket: %v", err)
	}

	// Success! Show the ticket details
	logger.Info().
		Str("ticket_key", ticket.Key).
		Str("ticket_id", ticket.ID).
		Str("ticket_url", ticket.Self).
		Msg("Successfully created Jira ticket")

	// Print a nice summary
	baseDomain := os.Getenv("JIRA_BASE_DOMAIN")
	logger.Info().
		Str("view_ticket", "https://"+baseDomain+"/browse/"+ticket.Key).
		Msg("You can view the ticket in your browser")
}
