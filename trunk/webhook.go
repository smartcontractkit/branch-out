package trunk

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"

	"github.com/smartcontractkit/branch-out/github"
)

// ReceiveWebhook processes a Trunk webhook and returns an error if the webhook is invalid.
func ReceiveWebhook(l zerolog.Logger, payload []byte) error {
	l.Info().Msg("Received trunk webhook")
	// First, quickly determine the webhook type without full parsing
	webhookType, err := GetWebhookType(payload)
	if err != nil {
		l.Error().Err(err).Str("payload", string(payload)).Msg("Failed to determine webhook type")
		return err
	}

	l = l.With().Str("webhook_type", string(webhookType)).Logger()

	// Parse the full event
	event, err := ParseWebhookEvent(payload)
	if err != nil {
		l.Error().Err(err).Msg("Failed to parse webhook event")
		return err
	}

	// Process different event types
	switch webhookType {
	case WebhookTypeQuarantiningSettingChanged:
		return handleQuarantiningSettingChanged(l, event)

	case WebhookTypeStatusChanged:
		return handleStatusChanged(l, event)
	}
	return fmt.Errorf("unknown webhook type '%s'", webhookType)
}

// handleQuarantiningSettingChanged processes quarantining setting change events
func handleQuarantiningSettingChanged(l zerolog.Logger, event WebhookEvent) error {
	l.Info().Msg("Processing quarantining setting changed event")

	testCase := event.GetTestCase()
	l.Info().
		Str("test_id", testCase.ID).
		Str("test_name", testCase.Name).
		Str("file_path", testCase.FilePath).
		Bool("quarantined", testCase.Quarantined).
		Msg("Test case quarantine setting changed")

	// Only process if test is being quarantined (not un-quarantined)
	if !testCase.Quarantined {
		l.Info().Msg("Test is being un-quarantined, skipping PR creation")
		return nil
	}

	// Parse repository URL to get owner and repo
	owner, repo, err := github.ParseRepoURL(testCase.Repository.HTMLURL)
	if err != nil {
		l.Error().Err(err).Str("repo_url", testCase.Repository.HTMLURL).Msg("Failed to parse repository URL")
		return fmt.Errorf("failed to parse repository URL: %w", err)
	}

	l.Debug().
		Str("owner", owner).
		Str("repo", repo).
		Msg("Parsed repository information")

	// Create GitHub client
	githubClient, err := github.NewClient(l, "", nil)
	if err != nil {
		l.Error().Err(err).Msg("Failed to create GitHub client")
		return fmt.Errorf("failed to create GitHub client: %w", err)
	}

	// Create quarantine request
	quarantineReq := github.QuarantineTestRequest{
		Owner:            owner,
		Repo:             repo,
		FilePath:         testCase.FilePath,
		TestFunctionName: testCase.Name,
		// Let the system generate default branch name, commit message, PR title, and body
	}

	// Execute quarantine process
	ctx := context.Background()
	response, err := githubClient.QuarantineTest(ctx, l, quarantineReq)
	if err != nil {
		l.Error().Err(err).Msg("Failed to quarantine test")
		return fmt.Errorf("failed to quarantine test: %w", err)
	}

	if response.PRURL != "" {
		l.Info().
			Str("branch_name", response.BranchName).
			Str("commit_sha", response.CommitSHA).
			Str("pr_url", response.PRURL).
			Msg("Successfully created quarantine PR")
	} else {
		l.Info().Msg("Test was already quarantined, no PR created")
	}

	return nil
}

// handleStatusChanged processes status change events
func handleStatusChanged(l zerolog.Logger, event WebhookEvent) error {
	l.Info().Msg("Processing status changed event")

	testCase := event.GetTestCase()
	l.Info().
		Str("test_id", testCase.ID).
		Str("test_name", testCase.Name).
		Str("file_path", testCase.FilePath).
		Str("status", testCase.Status.Value).
		Str("reason", testCase.Status.Reason).
		Msg("Test case status changed")

	// For now, we don't automatically quarantine based on status changes
	// This could be extended in the future to automatically quarantine tests
	// that become flaky or broken
	l.Debug().Msg("Status change processed (no action taken)")

	return nil
}
