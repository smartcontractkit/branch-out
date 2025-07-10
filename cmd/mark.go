package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/smartcontractkit/branch-out/server"
	"github.com/smartcontractkit/branch-out/trunk"
)

var (
	testPackage string
	testName    string
	repoURL     string
)

var markCmd = &cobra.Command{
	Use:   "mark <status>",
	Short: "Mark a test with a specific status",
	Long: `Mark a test with a specific status as if Trunk.io had sent us a webhook to react to.

This is useful for testing or for manually triggering a test status change that a running server wasn't able to receive.

The status can be one of: flaky, healthy, or broken.`,
	Example: `# Mark a test as flaky
branch-out mark flaky --package github.com/smartcontractkit/branch-out/package --name TestName

# Mark a test as healthy
branch-out mark healthy --package github.com/smartcontractkit/branch-out/package --name TestName`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{trunk.TestCaseStatusFlaky, trunk.TestCaseStatusHealthy, trunk.TestCaseStatusBroken},
	RunE: func(_ *cobra.Command, args []string) error {
		testStatus := args[0]

		l := logger.With().
			Str("command", "mark").
			Str("test_package", testPackage).
			Str("test_name", testName).
			Str("test_status", testStatus).
			Logger()

		statusChange := trunk.TestCaseStatusChange{
			TestCase: trunk.TestCase{
				TestSuite: testPackage,
				Name:      testName,
				Repository: trunk.Repository{
					HTMLURL: repoURL,
				},
			},
			StatusChange: trunk.StatusChange{
				CurrentStatus: trunk.Status{
					Value: testStatus,
				},
			},
		}

		jiraClient, trunkClient, githubClient, err := server.CreateClients(l, appConfig)
		if err != nil {
			return fmt.Errorf("failed to create clients: %w", err)
		}

		err = trunk.HandleTestCaseStatusChanged(l, statusChange, jiraClient, trunkClient, githubClient)
		if err != nil {
			return fmt.Errorf("failed to handle test status changed: %w", err)
		}

		l.Info().Msg("Test marked successfully")
		return nil
	},
}

func init() {
	root.AddCommand(markCmd)

	markCmd.Flags().
		StringVarP(&testPackage, "package", "p", "", "The test package (e.g. github.com/smartcontractkit/branch-out/package)")
	markCmd.Flags().StringVarP(&testName, "name", "n", "", "The test name (e.g. TestName)")
	markCmd.Flags().
		StringVarP(&repoURL, "repo", "r", "", "The repository URL (e.g. https://github.com/smartcontractkit/branch-out)")

	err := markCmd.MarkFlagRequired("package")
	if err != nil {
		panic(err)
	}
	err = markCmd.MarkFlagRequired("name")
	if err != nil {
		panic(err)
	}
	err = markCmd.MarkFlagRequired("repo")
	if err != nil {
		panic(err)
	}
}
