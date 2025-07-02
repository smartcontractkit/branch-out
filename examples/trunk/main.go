// Package main demonstrates how to integrate with Trunk.io API for linking
// Jira tickets to test cases and updating test case status.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/rs/zerolog"

	"github.com/smartcontractkit/branch-out/jira"
	"github.com/smartcontractkit/branch-out/trunk"
)

func main() {
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	// Trunk.io API integration example
	// Set this environment variable:
	// export TRUNK_API_TOKEN="your-trunk-api-token"

	// Check if required token is set
	if os.Getenv("TRUNK_API_TOKEN") == "" {
		log.Fatal(`TRUNK_API_TOKEN environment variable is required.

Please set up your environment variable:
export TRUNK_API_TOKEN="your-trunk-api-token"`)
	}

	logger.Info().Msg("Starting Trunk.io API example")

	// Create a Trunk client
	trunkClient, err := trunk.NewClient(logger)
	if err != nil {
		log.Fatalf("Failed to create Trunk client: %v", err)
	}

	logger.Info().Msg("Successfully created Trunk client")

	// Example: Link a ticket to a test case
	// This would typically happen after creating a Jira ticket
	exampleTicket := &jira.TicketResponse{
		ID:   "12345",
		Key:  "EXAMPLE-123",
		Self: "https://your-company.atlassian.net/rest/api/2/issue/12345",
	}

	testCaseID := "example-test-case-id-uuid"
	repoURL := "https://github.com/your-org/your-repo"

	logger.Info().
		Str("test_case_id", testCaseID).
		Str("ticket_key", exampleTicket.Key).
		Str("repo_url", repoURL).
		Msg("Linking Jira ticket to Trunk test case")

	err = trunkClient.LinkTicketToTestCase(testCaseID, exampleTicket, repoURL)
	if err != nil {
		log.Fatalf("Failed to link ticket to test case: %v", err)
	}

	logger.Info().
		Str("ticket_key", exampleTicket.Key).
		Str("test_case_id", testCaseID).
		Msg("Successfully linked Jira ticket to Trunk test case")

	fmt.Println("\n✅ Trunk.io API example completed successfully!")
	fmt.Println("🔗 The ticket has been linked to the test case in Trunk.io")
	fmt.Println("\nNext steps:")
	fmt.Println("- Check the Trunk.io dashboard to see the linked ticket")
	fmt.Println("- The webhook handler will automatically use this client for real flaky test events")
}
