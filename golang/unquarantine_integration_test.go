// Package golang_test contains integration tests for the golang package.
package golang_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/golang"
	"github.com/smartcontractkit/branch-out/internal/testhelpers"
)

// Quarantine-related string constants for testing
const (
	quarantineConditionalCheck = `if os.Getenv("RUN_QUARANTINED_TESTS") != "true"`
	quarantineSkipStatement    = `t.Skip("Flaky test quarantined`
	quarantineEnvVarCheck      = `os.Getenv("RUN_QUARANTINED_TESTS")`
)

// TestQuarantineUnquarantineCycle tests the complete cycle using existing test infrastructure
func TestQuarantineUnquarantineCycle(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping integration tests in short mode")
	}

	l := testhelpers.Logger(t)
	dir := setupDir(t)

	packageName := baseProjectPackage
	testName := standardTestNames[0]

	// Create the quarantine.
	quarantineTargets := []golang.TestTarget{
		{
			Package: packageName,
			Tests:   []string{testName},
		},
	}

	quarantineResults, err := golang.QuarantineTests(
		l,
		dir,
		quarantineTargets,
		golang.WithBuildFlags(exampleProjectBuildFlags),
	)
	require.NoError(t, err, "failed to quarantine test")
	require.Contains(t, quarantineResults, packageName)
	require.Len(t, quarantineResults[packageName].Successes, 1)

	quarantinedFile := quarantineResults[packageName].Successes[0]

	// Quarantined code should contain these strings.
	require.Contains(t, quarantinedFile.ModifiedSourceCode, quarantineConditionalCheck,
		"Quarantined code should contain the full quarantine conditional")
	require.Contains(t, quarantinedFile.ModifiedSourceCode, quarantineSkipStatement,
		"Quarantined code should contain the skip statement")

	err = golang.WriteQuarantineResultsToFiles(l, quarantineResults)
	require.NoError(t, err, "failed to write quarantine results")

	// Unquarantine the test.
	unquarantineTargets := []golang.TestTarget{
		{
			Package: packageName,
			Tests:   []string{testName},
		},
	}

	unquarantineResults, err := golang.UnquarantineTests(
		l,
		dir,
		unquarantineTargets,
		golang.WithUnquarantineBuildFlags(exampleProjectBuildFlags),
	)
	require.NoError(t, err, "failed to unquarantine test")

	// Verify unquarantine worked
	require.Contains(t, unquarantineResults, packageName)
	unquarantinePackageResult := unquarantineResults[packageName]
	require.Len(t, unquarantinePackageResult.Successes, 1)

	unquarantinedFile := unquarantinePackageResult.Successes[0]

	// Critical check: quarantine logic must be completely removed
	assert.NotContains(t, unquarantinedFile.ModifiedSourceCode, quarantineEnvVarCheck,
		"Unquarantined code must not contain environment variable check")
	assert.NotContains(t, unquarantinedFile.ModifiedSourceCode, quarantineSkipStatement,
		"Unquarantined code must not contain skip logic")

	// Step 6: Write and verify unquarantine persisted
	err = golang.WriteUnquarantineResultsToFiles(l, unquarantineResults)
	require.NoError(t, err, "failed to write unquarantine results")

	// Step 7: Final verification - read from disk to ensure persistence
	if filepath.IsAbs(unquarantinedFile.FileAbs) {
		finalContent, err := os.ReadFile(unquarantinedFile.FileAbs)
		require.NoError(t, err, "should be able to read unquarantined file")

		finalSource := string(finalContent)
		assert.NotContains(t, finalSource, quarantineEnvVarCheck,
			"Final file must not contain quarantine logic")
		assert.Contains(t, finalSource, fmt.Sprintf("func %s(", testName),
			"Final file must still contain the test function")
	}
}

// TestUnquarantineNonQuarantinedTest verifies graceful handling of non-quarantined tests
func TestUnquarantineNonQuarantinedTest(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping integration tests in short mode")
	}

	l := testhelpers.Logger(t)
	dir := setupDir(t)

	packageName := baseProjectPackage
	testName := standardTestNames[1] // Use different test to avoid interference

	// Try to unquarantine a test that was never quarantined
	unquarantineTargets := []golang.TestTarget{
		{
			Package: packageName,
			Tests:   []string{testName},
		},
	}

	unquarantineResults, err := golang.UnquarantineTests(
		l,
		dir,
		unquarantineTargets,
		golang.WithUnquarantineBuildFlags(exampleProjectBuildFlags),
	)

	// Should not error - graceful handling is expected
	require.NoError(t, err, "unquarantine should handle non-quarantined tests gracefully")
	require.Contains(t, unquarantineResults, packageName)
}
