// Package quarantine provides a way to mark tests as flaky.
package quarantine

import (
	"fmt"
	"os"
	"testing"
)

// RunFlakyTestsEnvVar is the environment variable that controls whether to run flaky tests.
const RunFlakyTestsEnvVar = "RUN_FLAKY_TESTS"

// Flaky marks a test as flaky.
// To run tests marked as flaky, set the RUN_FLAKY_TESTS environment variable to true.
// To skip tests marked as flaky, set the RUN_FLAKY_TESTS environment variable to false (or don't set it at all).
//
// Example:
//
//	func TestFlaky(t *testing.T) {
//		quarantine.Flaky(t, "TEST-123")
//	}
func Flaky(tb testing.TB, ticket string) {
	tb.Helper()

	explanationStr := fmt.Sprintf(
		"Known flaky test. Ticket %s.\nClassified by branch-out (https://github.com/smartcontractkit/branch-out)",
		ticket,
	)
	// Add this when we can use Go 1.25.0.
	// tb.Attr("flaky", "true")
	//nolint:forbidigo // Config doesn't make sense here
	if os.Getenv(RunFlakyTestsEnvVar) != "true" {
		tb.Skipf(
			"Skipping %s. To run flaky tests, set the %s environment variable to true.",
			explanationStr,
			RunFlakyTestsEnvVar,
		)
	} else {
		tb.Logf("Running %s", explanationStr)
		tb.Cleanup(func() {
			tb.Logf(
				"Test is marked as flaky, but still ran. %s. To skip flaky tests, ensure the %s environment variable is set to false.",
				explanationStr,
				RunFlakyTestsEnvVar,
			)
		})
	}
}
