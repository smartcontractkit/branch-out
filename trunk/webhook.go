package trunk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	go_jira "github.com/andygrunwald/go-jira"
	"github.com/rs/zerolog"
	svix "github.com/svix/svix-webhooks/go"

	"github.com/smartcontractkit/branch-out/github"
	"github.com/smartcontractkit/branch-out/golang"
	"github.com/smartcontractkit/branch-out/jira"
)

const (
	// TestCaseStatusHealthy is the status of a test that is healthy.
	TestCaseStatusHealthy = "healthy"
	// TestCaseStatusFlaky is the status of a test that is flaky.
	TestCaseStatusFlaky = "flaky"
	// TestCaseStatusBroken is the status of a test that is broken.
	TestCaseStatusBroken = "broken"
)

// ReceiveWebhook processes a Trunk webhook, typically triggered by a test being marked/unmarked as flaky.
// It will create a
func ReceiveWebhook(
	l zerolog.Logger,
	req *http.Request,
	signingSecret string,
	jiraClient jira.IClient,
	trunkClient IClient,
	githubClient github.IClient,
) error {
	if err := verifyClients(jiraClient, trunkClient, githubClient); err != nil {
		return err
	}

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
	if err := VerifyWebhookRequest(l, req, signingSecret); err != nil {
		return fmt.Errorf("webhook call cannot be verified: %w", err)
	}

	var webhookData TestCaseStatusChange
	if err := json.Unmarshal(payload, &webhookData); err != nil {
		l.Error().Err(err).Str("payload", string(payload)).Msg("Failed to parse test_case.status_changed payload")
		return fmt.Errorf("failed to parse test_case.status_changed payload: %w", err)
	}

	return HandleTestCaseStatusChanged(l, webhookData, jiraClient, trunkClient, githubClient)
}

// HandleTestCaseStatusChanged processes when a test case's status changes.
// This can be one of: healthy, flaky, or broken.
func HandleTestCaseStatusChanged(
	l zerolog.Logger,
	statusChange TestCaseStatusChange,
	jiraClient jira.IClient,
	trunkClient IClient,
	githubClient github.IClient,
) error {
	if err := verifyClients(jiraClient, trunkClient, githubClient); err != nil {
		return err
	}

	l.Info().Msg("Processing test_case.status_changed event")

	testCase := statusChange.TestCase
	currentStatus := statusChange.StatusChange.CurrentStatus.Value
	previousStatus := statusChange.StatusChange.PreviousStatus

	l.Info().
		Str("test_id", testCase.ID).
		Str("test_name", testCase.Name).
		Str("file_path", testCase.FilePath).
		Str("current_status", currentStatus).
		Str("previous_status", previousStatus).
		Msg("Test status changed")

	switch currentStatus {
	case TestCaseStatusFlaky:
		return handleFlakyTest(l, statusChange, jiraClient, trunkClient, githubClient)
	case TestCaseStatusBroken:
		return handleBrokenTest(l, statusChange, jiraClient, trunkClient, githubClient)
	case TestCaseStatusHealthy:
		return handleHealthyTest(l, statusChange, jiraClient, trunkClient, githubClient)
	}

	return nil
}

// handleFlakyTest handles the case where a test is marked as flaky.
// This will create a Jira ticket, link it to the Trunk test case, and quarantine the test in GitHub.
func handleFlakyTest(
	l zerolog.Logger,
	statusChange TestCaseStatusChange,
	jiraClient jira.IClient,
	trunkClient IClient,
	githubClient github.IClient,
) error {
	if err := verifyClients(jiraClient, trunkClient, githubClient); err != nil {
		return err
	}
	testCase := statusChange.TestCase
	currentStatus := statusChange.StatusChange.CurrentStatus.Value
	previousStatus := statusChange.StatusChange.PreviousStatus

	l = l.With().
		Str("test_id", testCase.ID).
		Str("name", testCase.Name).
		Str("repo_url", testCase.Repository.HTMLURL).
		Str("package", testCase.TestSuite).
		Str("current_status", currentStatus).
		Str("previous_status", previousStatus).
		Logger()

	l.Info().
		Msg("Quarantining flaky test")

	// Create a Jira ticket for the flaky test
	_, err := createJiraTicketForFlakyTest(l, statusChange, jiraClient, trunkClient)
	if err != nil {
		return fmt.Errorf("failed to create Jira ticket: %w", err)
	}

	// Quarantine the test in GitHub
	err = githubClient.QuarantineTests(
		context.Background(),
		l,
		testCase.Repository.HTMLURL,
		[]golang.QuarantineTarget{
			{
				Package: testCase.TestSuite,
				Tests:   []string{testCase.Name},
			},
		},
	)
	if err != nil {
		return fmt.Errorf("failed to quarantine test: %w", err)
	}

	return nil
}

// handleBrokenTest handles the case where a test is marked as broken.
// Right now this is the same as handling a flaky test.
func handleBrokenTest(
	l zerolog.Logger,
	statusChange TestCaseStatusChange,
	jiraClient jira.IClient,
	trunkClient IClient,
	githubClient github.IClient,
) error {
	return handleFlakyTest(l, statusChange, jiraClient, trunkClient, githubClient)
}

// handleHealthyTest handles the case where a test is marked as healthy.
// This is a no-op for now.
func handleHealthyTest(
	_ zerolog.Logger,
	_ TestCaseStatusChange,
	jiraClient jira.IClient,
	trunkClient IClient,
	githubClient github.IClient,
) error {
	if err := verifyClients(jiraClient, trunkClient, githubClient); err != nil {
		return err
	}

	return fmt.Errorf("healthy test handling not implemented")
}

// verifyClients verifies that all the clients are not nil.
func verifyClients(jiraClient jira.IClient, trunkClient IClient, githubClient github.IClient) error {
	if jiraClient == nil {
		return fmt.Errorf("jira client is nil")
	}
	if trunkClient == nil {
		return fmt.Errorf("trunk client is nil")
	}
	if githubClient == nil {
		return fmt.Errorf("github client is nil")
	}
	return nil
}

// createJiraTicketForFlakyTest creates a Jira ticket for a flaky test
func createJiraTicketForFlakyTest(
	l zerolog.Logger,
	statusChange TestCaseStatusChange,
	jiraClient jira.IClient,
	trunkClient IClient,
) (*go_jira.Issue, error) {
	if jiraClient == nil {
		return nil, fmt.Errorf("jira client is nil")
	}
	if trunkClient == nil {
		return nil, fmt.Errorf("trunk client is nil")
	}

	testCase := statusChange.TestCase

	// Create details JSON from test case data
	details, err := json.Marshal(map[string]any{
		"failure_rate_last_7d":           testCase.FailureRateLast7D,
		"pull_requests_impacted_last_7d": testCase.PullRequestsImpactedLast7D,
		"codeowners":                     testCase.Codeowners,
		"html_url":                       testCase.HTMLURL,
		"repository_url":                 testCase.Repository.HTMLURL,
		"current_status":                 statusChange.StatusChange.CurrentStatus,
		"previous_status":                statusChange.StatusChange.PreviousStatus,
		"test_suite":                     testCase.TestSuite,
		"variant":                        testCase.Variant,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal test case details: %w", err)
	}

	req := jira.FlakyTestTicketRequest{
		RepoURL:           testCase.Repository.HTMLURL,
		Package:           testCase.TestSuite,
		Test:              testCase.Name,
		FilePath:          testCase.FilePath,
		TrunkID:           testCase.ID,
		AdditionalDetails: string(details),
	}

	ticket, err := jiraClient.CreateFlakyTestTicket(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create Jira ticket: %w", err)
	}

	// Link the Jira ticket back to the Trunk test case
	if err := trunkClient.LinkTicketToTestCase(testCase.ID, ticket, testCase.Repository.HTMLURL); err != nil {
		l.Warn().Err(err).Msg("Failed to link Jira ticket to Trunk test case (non-blocking)")
		// Don't return error as the ticket was created successfully
	}

	return ticket, nil
}

// Unused for now, but keeping for reference.
// // handleExistingTicketForFlakyTest handles the case where a test already has a linked Jira ticket
// func handleExistingTicketForFlakyTest(
// 	l zerolog.Logger,
// 	webhookData TestCaseStatusChange,
// 	jiraClient jira.IClient,
// 	trunkClient IClient,
// ) error {
// 	testCase := webhookData.TestCase
// 	ticketURL := testCase.Ticket.HTMLURL

// 	// Extract ticket key from the URL
// 	ticketKey := extractTicketKeyFromJiraURL(ticketURL)

// 	l.Info().
// 		Str("existing_ticket_url", ticketURL).
// 		Str("ticket_key", ticketKey).
// 		Msg("Test has existing ticket, checking status")

// 	// Get the current ticket status
// 	ticketStatus, err := jiraClient.GetTicketStatus(ticketKey)
// 	if err != nil {
// 		l.Warn().Err(err).
// 			Str("ticket_key", ticketKey).
// 			Msg("Failed to get ticket status, will skip ticket handling")
// 		return nil // Non-blocking - continue processing
// 	}

// 	if ticketStatus.IsResolved() {
// 		l.Info().
// 			Str("ticket_key", ticketKey).
// 			Str("status", ticketStatus.Fields.Status.Name).
// 			Msg("Existing ticket is resolved, creating new ticket for re-flaking test")

// 		// Create a new ticket since the old one is closed
// 		_, err := createJiraTicketForFlakyTest(l, webhookData, jiraClient, trunkClient)
// 		if err != nil {
// 			return fmt.Errorf("failed to create Jira ticket: %w", err)
// 		}
// 		return nil

// 	}
// 	l.Info().
// 		Str("ticket_key", ticketKey).
// 		Str("status", ticketStatus.Fields.Status.Name).
// 		Msg("Existing ticket is still open, adding comment with latest flaky test info")

// 	// Add a comment to the existing ticket with new information
// 	return addFlakyTestUpdateComment(l, webhookData, jiraClient, ticketKey)
// }

// Unused for now, but keeping for reference.
// addFlakyTestUpdateComment adds a comment to an existing Jira ticket with updated flaky test information
// func addFlakyTestUpdateComment(
// 	l zerolog.Logger,
// 	webhookData TestCaseStatusChange,
// 	jiraClient jira.IClient,
// 	ticketKey string,
// ) error {
// 	testCase := webhookData.TestCase

// 	// Create a comment with the latest information
// 	comment := fmt.Sprintf(`Test case has become flaky again.

// *Latest Status Update:*
// • Current Status: %s
// • Previous Status: %s
// • Failure Rate (last 7 days): %.2f%%
// • Pull Requests Impacted (last 7 days): %d
// • Repository: %s
// • Test File: %s
// • Test Suite: %s

// *Links:*
// • [View Test in Trunk.io](%s)
// • [Repository](%s)

// _This comment was automatically generated by [branch-out](https://github.com/smartcontractkit/branch-out)._`,
// 		webhookData.StatusChange.CurrentStatus.Value,
// 		webhookData.StatusChange.PreviousStatus,
// 		testCase.FailureRateLast7D,
// 		testCase.PullRequestsImpactedLast7D,
// 		testCase.Repository.HTMLURL,
// 		testCase.FilePath,
// 		testCase.TestSuite,
// 		testCase.HTMLURL,
// 		testCase.Repository.HTMLURL,
// 	)

// 	err := jiraClient.AddCommentToTicket(ticketKey, comment)
// 	if err != nil {
// 		l.Error().Err(err).
// 			Str("ticket_key", ticketKey).
// 			Msg("Failed to add comment to existing Jira ticket")
// 		return fmt.Errorf("failed to add comment to ticket %s: %w", ticketKey, err)
// 	}

// 	l.Info().
// 		Str("ticket_key", ticketKey).
// 		Str("test_name", testCase.Name).
// 		Msg("Successfully added flaky test update comment to existing ticket")

// 	return nil
// }

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
// Unused for now, but keeping for reference.
// func extractTicketKeyFromJiraURL(url string) string {
// 	// Expected format: https://your-company.atlassian.net/browse/TICKET-123
// 	parts := strings.Split(url, "/")
// 	if len(parts) >= 1 {
// 		lastPart := parts[len(parts)-1]
// 		// Handle potential query parameters (e.g., TICKET-123?someParam=value)
// 		if queryIndex := strings.Index(lastPart, "?"); queryIndex != -1 {
// 			return lastPart[:queryIndex]
// 		}
// 		return lastPart
// 	}
// 	return url // Return the full URL if parsing fails
// }

// extractDomainFromJiraURL extracts the domain from a Jira self URL
func extractDomainFromJiraURL(selfURL string) string {
	// Example: https://your-company.atlassian.net/rest/api/2/issue/123
	url, err := url.Parse(selfURL)
	if err != nil {
		return ""
	}
	return url.Host
}

// SelfSignWebhookRequest self-signs a request to create a valid svix webhook call.
// This is useful for testing and for the webhook command.
func SelfSignWebhookRequest(l zerolog.Logger, req *http.Request, signingSecret string) (*http.Request, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}

	if req.Body == nil {
		return nil, fmt.Errorf("request body is nil")
	}

	// Create svix webhook for signing
	wh, err := svix.NewWebhook(signingSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to create svix webhook: %w", err)
	}

	if req.Header == nil {
		req.Header = make(http.Header)
	}

	// Generate headers (svix will add the signature)
	req.Header.Set("webhook-id", "self_signed_webhook_id")
	req.Header.Set("webhook-timestamp", fmt.Sprintf("%d", time.Now().Unix()))

	payload, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}
	defer func() {
		if err := req.Body.Close(); err != nil {
			l.Error().Err(err).Msg("Failed to close request body")
		}
	}()
	req.Body = io.NopCloser(bytes.NewBuffer(payload))

	// Sign the payload
	signature, err := wh.Sign(req.Header.Get("webhook-id"), time.Now(), payload)
	if err != nil {
		return nil, fmt.Errorf("failed to sign webhook payload: %w", err)
	}
	req.Header.Set("webhook-signature", signature)

	return req, nil
}

// VerifyWebhookRequest verifies a request as a valid svix webhook call.
// https://docs.svix.com/receiving/verifying-payloads/how
func VerifyWebhookRequest(l zerolog.Logger, req *http.Request, signingSecret string) error {
	wh, err := svix.NewWebhook(signingSecret)
	if err != nil {
		return fmt.Errorf("failed to create svix webhook: %w", err)
	}

	payload, err := io.ReadAll(req.Body)
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}
	defer func() {
		if err := req.Body.Close(); err != nil {
			l.Error().Err(err).Msg("Failed to close request body")
		}
	}()
	req.Body = io.NopCloser(bytes.NewBuffer(payload))

	return wh.Verify(payload, req.Header)
}
