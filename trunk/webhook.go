package trunk

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/rs/zerolog"
	svix "github.com/svix/svix-webhooks/go"

	"github.com/smartcontractkit/branch-out/jira"
)

// ReceiveWebhook processes a Trunk webhook, typically triggered by a test being marked/unmarked as flaky.
// It will create a
func ReceiveWebhook(
	l zerolog.Logger,
	req *http.Request,
	signingSecret string,
	jiraClient jira.IClient,
	trunkClient IClient,
) error {
	l.Info().Msg("Received trunk webhook")

	payload, err := io.ReadAll(req.Body)
	if err != nil {
		l.Error().Err(err).Msg("Failed to read request body")
		return fmt.Errorf("failed to read request body: %w", err)
	}
	defer func() {
		if err := req.Body.Close(); err != nil {
			l.Error().Err(err).Msg("Failed to close request body")
		}
	}()

	// Verify the webhook signature
	// https://docs.svix.com/receiving/verifying-payloads/how
	wh, err := svix.NewWebhook(signingSecret)
	if err != nil {
		return fmt.Errorf("failed to create svix webhook: %w", err)
	}

	if err := wh.Verify(payload, req.Header); err != nil {
		return fmt.Errorf("failed to verify svix webhook: %w", err)
	}

	var webhookData TestCaseStatusChangedPayload
	if err := json.Unmarshal(payload, &webhookData); err != nil {
		l.Error().Err(err).Str("payload", string(payload)).Msg("Failed to parse test_case.status_changed payload")
		return fmt.Errorf("failed to parse test_case.status_changed payload: %w", err)
	}

	return handleTestCaseStatusChanged(l, webhookData, jiraClient, trunkClient)
}

// handleTestCaseStatusChanged processes test_case.status_changed events
func handleTestCaseStatusChanged(
	l zerolog.Logger,
	webhookData TestCaseStatusChangedPayload,
	jiraClient jira.IClient,
	trunkClient IClient,
) error {
	l.Info().Msg("Processing test_case.status_changed event")

	testCase := webhookData.TestCase
	currentStatus := webhookData.StatusChange.CurrentStatus.Value
	previousStatus := webhookData.StatusChange.PreviousStatus

	l.Info().
		Str("test_id", testCase.ID).
		Str("test_name", testCase.Name).
		Str("file_path", testCase.FilePath).
		Str("current_status", currentStatus).
		Str("previous_status", previousStatus).
		Msg("Test case status changed")

	// If test status changed to "flaky" and we have a Jira client, create a ticket
	if currentStatus == "flaky" && jiraClient != nil {
		if testCase.Ticket.HTMLURL == "" {
			// No existing ticket, create a new one
			return createJiraTicketForFlakyTest(l, webhookData, jiraClient, trunkClient)
		}
		// Existing ticket found, check its status
		return handleExistingTicketForFlakyTest(l, webhookData, jiraClient, trunkClient)
	}

	return nil
}

// createJiraTicketForFlakyTest creates a Jira ticket for a flaky test
func createJiraTicketForFlakyTest(
	l zerolog.Logger,
	webhookData TestCaseStatusChangedPayload,
	jiraClient jira.IClient,
	trunkClient IClient,
) error {
	testCase := webhookData.TestCase

	// Extract repo name from the HTML URL
	repoName := extractRepoNameFromURL(testCase.Repository.HTMLURL)

	// Create details JSON from test case data
	details, err := json.Marshal(map[string]interface{}{
		"failure_rate_last_7d":           testCase.FailureRateLast7D,
		"pull_requests_impacted_last_7d": testCase.PullRequestsImpactedLast7D,
		"codeowners":                     testCase.Codeowners,
		"html_url":                       testCase.HTMLURL,
		"repository_url":                 testCase.Repository.HTMLURL,
		"current_status":                 webhookData.StatusChange.CurrentStatus,
		"previous_status":                webhookData.StatusChange.PreviousStatus,
		"test_suite":                     testCase.TestSuite,
		"variant":                        testCase.Variant,
	})
	if err != nil {
		l.Error().Err(err).Msg("Failed to marshal test case details to JSON")
		return fmt.Errorf("failed to marshal test case details: %w", err)
	}

	req := jira.FlakyTestTicketRequest{
		RepoName:        repoName,
		TestPackageName: testCase.Name,
		FilePath:        testCase.FilePath,
		TrunkID:         testCase.ID,
		Details:         string(details),
	}

	l.Info().
		Str("repo_name", req.RepoName).
		Str("test_name", req.TestPackageName).
		Str("file_path", req.FilePath).
		Str("trunk_id", req.TrunkID).
		Msg("Creating Jira ticket for flaky test")

	ticket, err := jiraClient.CreateFlakyTestTicket(req)
	if err != nil {
		l.Error().Err(err).Msg("Failed to create Jira ticket for flaky test")
		return fmt.Errorf("failed to create Jira ticket: %w", err)
	}

	l.Info().
		Str("ticket_key", ticket.Key).
		Str("ticket_id", ticket.ID).
		Str("ticket_url", fmt.Sprintf("https://%s/browse/%s", extractDomainFromJiraURL(ticket.Self), ticket.Key)).
		Msg("Successfully created Jira ticket for flaky test")

	// Link the Jira ticket back to the Trunk test case
	if trunkClient != nil {
		if err := trunkClient.LinkTicketToTestCase(testCase.ID, ticket, testCase.Repository.HTMLURL); err != nil {
			l.Warn().Err(err).Msg("Failed to link Jira ticket to Trunk test case (non-blocking)")
			// Don't return error as the ticket was created successfully
		}
	}

	return nil
}

// handleExistingTicketForFlakyTest handles the case where a test already has a linked Jira ticket
func handleExistingTicketForFlakyTest(
	l zerolog.Logger,
	webhookData TestCaseStatusChangedPayload,
	jiraClient jira.IClient,
	trunkClient IClient,
) error {
	testCase := webhookData.TestCase
	ticketURL := testCase.Ticket.HTMLURL

	// Extract ticket key from the URL
	ticketKey := extractTicketKeyFromJiraURL(ticketURL)

	l.Info().
		Str("existing_ticket_url", ticketURL).
		Str("ticket_key", ticketKey).
		Msg("Test has existing ticket, checking status")

	// Get the current ticket status
	ticketStatus, err := jiraClient.GetTicketStatus(ticketKey)
	if err != nil {
		l.Warn().Err(err).
			Str("ticket_key", ticketKey).
			Msg("Failed to get ticket status, will skip ticket handling")
		return nil // Non-blocking - continue processing
	}

	if ticketStatus.IsResolved() {
		l.Info().
			Str("ticket_key", ticketKey).
			Str("status", ticketStatus.Fields.Status.Name).
			Msg("Existing ticket is resolved, creating new ticket for re-flaking test")

		// Create a new ticket since the old one is closed
		return createJiraTicketForFlakyTest(l, webhookData, jiraClient, trunkClient)

	}
	l.Info().
		Str("ticket_key", ticketKey).
		Str("status", ticketStatus.Fields.Status.Name).
		Msg("Existing ticket is still open, adding comment with latest flaky test info")

	// Add a comment to the existing ticket with new information
	return addFlakyTestUpdateComment(l, webhookData, jiraClient, ticketKey)
}

// addFlakyTestUpdateComment adds a comment to an existing Jira ticket with updated flaky test information
func addFlakyTestUpdateComment(
	l zerolog.Logger,
	webhookData TestCaseStatusChangedPayload,
	jiraClient jira.IClient,
	ticketKey string,
) error {
	testCase := webhookData.TestCase

	// Create a comment with the latest information
	comment := fmt.Sprintf(`Test case has become flaky again.

*Latest Status Update:*
• Current Status: %s
• Previous Status: %s
• Failure Rate (last 7 days): %.2f%%
• Pull Requests Impacted (last 7 days): %d
• Repository: %s
• Test File: %s
• Test Suite: %s

*Links:*
• [View Test in Trunk.io](%s)
• [Repository](%s)

_This comment was automatically generated by the flaky test webhook handler._`,
		webhookData.StatusChange.CurrentStatus.Value,
		webhookData.StatusChange.PreviousStatus,
		testCase.FailureRateLast7D,
		testCase.PullRequestsImpactedLast7D,
		testCase.Repository.HTMLURL,
		testCase.FilePath,
		testCase.TestSuite,
		testCase.HTMLURL,
		testCase.Repository.HTMLURL,
	)

	err := jiraClient.AddCommentToTicket(ticketKey, comment)
	if err != nil {
		l.Error().Err(err).
			Str("ticket_key", ticketKey).
			Msg("Failed to add comment to existing Jira ticket")
		return fmt.Errorf("failed to add comment to ticket %s: %w", ticketKey, err)
	}

	l.Info().
		Str("ticket_key", ticketKey).
		Str("test_name", testCase.Name).
		Msg("Successfully added flaky test update comment to existing ticket")

	return nil
}

// extractRepoNameFromURL extracts the repository name from a GitHub URL
func extractRepoNameFromURL(url string) string {
	// Expected format: https://github.com/owner/repo
	parts := strings.Split(url, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-1] // Return the repo name (last part)
	}
	return url // Return the full URL if parsing fails
}

// extractTicketKeyFromJiraURL extracts the ticket key from a Jira URL
func extractTicketKeyFromJiraURL(url string) string {
	// Expected format: https://your-company.atlassian.net/browse/TICKET-123
	parts := strings.Split(url, "/")
	if len(parts) >= 1 {
		lastPart := parts[len(parts)-1]
		// Handle potential query parameters (e.g., TICKET-123?someParam=value)
		if queryIndex := strings.Index(lastPart, "?"); queryIndex != -1 {
			return lastPart[:queryIndex]
		}
		return lastPart
	}
	return url // Return the full URL if parsing fails
}

// extractDomainFromJiraURL extracts the domain from a Jira self URL
func extractDomainFromJiraURL(selfURL string) string {
	// Example: https://your-company.atlassian.net/rest/api/2/issue/123
	parts := strings.Split(selfURL, "/")
	if len(parts) >= 3 {
		return parts[2] // Return the domain part
	}
	return "unknown-domain.atlassian.net" // Fallback
}

// extractRepoInfoFromURL extracts owner and repository name from a GitHub URL
func extractRepoInfoFromURL(url string) (owner, repoName string) {
	// Expected format: https://github.com/owner/repo
	parts := strings.Split(url, "/")
	if len(parts) >= 5 && parts[2] == "github.com" {
		return parts[3], parts[4] // Return owner and repo name
	}
	return "unknown", "unknown" // Fallback if parsing fails
}
