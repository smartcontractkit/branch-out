// Package golang_test contains integration tests for the golang package. Tests that require the example_project to be present.
package golang_test

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/golang"
	"github.com/smartcontractkit/branch-out/internal/testhelpers"
)

var exampleProjectBuildFlags = []string{
	"-tags", "example_project",
}

func TestQuarantineTests_Integration_All(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration tests in short mode")
	}

	l := testhelpers.Logger(t)
	dir := setupDir(t)

	quarantineResults, err := golang.QuarantineTests(l, dir, allQuarantineTargets, exampleProjectBuildFlags)
	require.NoError(t, err, "failed to quarantine tests")

	// Build a list of all the tests we successfully quarantined to check in our runs later
	// Don't bother checking the tests that we know weren't quarantined
	successfullyQuarantinedTests := []golang.QuarantineTarget{}
	for _, result := range quarantineResults {
		for _, success := range result.Successes {
			successfullyQuarantinedTests = append(successfullyQuarantinedTests, golang.QuarantineTarget{
				Package: success.Package,
				Tests:   success.Tests,
			})
		}
		assert.Empty(
			t,
			result.Failures,
			"failed to quarantine these tests in package '%s'\n%s",
			result.Package,
			strings.Join(result.Failures, "\n"),
		)
	}

	testOutput, _ := runExampleTests( //nolint:testifylint // If there's an error here, it's likely because the tests failed, which doesn't stop us from checking the results
		t,
		dir,
	)
	t.Cleanup(func() {
		if t.Failed() {
			sanitizedName := strings.ReplaceAll(t.Name(), "/", "_")
			testOutputFile := fmt.Sprintf("%s_test_output.log.json", sanitizedName)
			l.Error().Str("test_output_file", testOutputFile).Msg("Leaving test output for debugging")
			if err := os.WriteFile(testOutputFile, testOutput, 0600); err != nil {
				t.Logf("failed to write test output to file: %s", err)
			}
		}
	})

	testResults, err := testhelpers.ParseTestOutput(testOutput)
	require.NoError(t, err, "failed to parse test output")

	for _, successfullyQuarantinedTarget := range successfullyQuarantinedTests {
		pkgResults, ok := testResults[successfullyQuarantinedTarget.Package]
		require.True(t, ok, "package %s not found in test results", successfullyQuarantinedTarget.Package)
		for _, test := range successfullyQuarantinedTarget.Tests {
			require.Contains(
				t,
				pkgResults.Found,
				test,
				"'%s' in package '%s' wasn't run", test, successfullyQuarantinedTarget.Package,
			)
			assert.Contains(
				t,
				pkgResults.Skipped,
				test,
				"'%s' in package '%s' was marked as successfully quarantined but was not skipped",
				test,
				successfullyQuarantinedTarget.Package,
			)
		}
	}
}

// runExampleTests runs go test for the example_project and returns the test results.
// It returns the test output and any error that occurred while running the tests.
// It can optionally run only a subset of tests by passing in the test names.
func runExampleTests(
	tb testing.TB,
	dir string,
	specificTests ...string,
) (testOutput []byte, runError error) {
	tb.Helper()

	command := []string{"test"}
	command = append(command, exampleProjectBuildFlags...)
	command = append(command, "./...")
	if len(specificTests) > 0 {
		runRegex := strings.Join(specificTests, "|")
		command = append(command, "-run", runRegex)
	}
	command = append(command, "-v", "-count=1", "-json")

	testCmd := exec.Command("go", command...)
	testCmd.Dir = dir

	return testCmd.CombinedOutput()
}

func setupDir(tb testing.TB) string {
	tb.Helper()

	targetDir := fmt.Sprintf("%s-copied-code", strings.ReplaceAll(tb.Name(), "/", "_"))
	err := os.MkdirAll(targetDir, 0750)
	require.NoError(tb, err, "failed to create copied code dir")
	tb.Cleanup(func() {
		if tb.Failed() {
			tb.Logf("leaving dir %s for debugging", targetDir)
		} else {
			err := os.RemoveAll(targetDir)
			require.NoError(tb, err, "failed to remove copied code")
		}
	})

	err = testhelpers.CopyDir(tb, "example_project", targetDir)
	require.NoError(tb, err, "failed to copy example project to temp dir for testing")

	return targetDir
}

var (
	baseTests = []string{
		"TestStandard1",
		"TestStandard2",
		"TestStandard3",
		"TestSubTestsStatic/subtest_1",
		"TestSubTestsStatic/subtest_2",
		"TestSubTestsTableStatic/subtest_1",
		"TestSubTestsTableStatic/subtest_2",
		"TestSubTestsTableDynamic/subtest_1",
		"TestSubTestsTableDynamic/subtest_2",
		"TestSubSubTestsStatic/parent_subtest/sub-subtest_1",
		"TestSubSubTestsStatic/parent_subtest/sub-subtest_2",
		// "BenchmarkExampleProject",
		"FuzzExampleProject",
		"TestDifferentParam",
	}
	testPackageTests = []string{
		"TestTestPackage",
	}
	oddlyNamedPackageTests = []string{
		"TestOddlyNamedPackage",
	}

	baseProjectQuarantineTargets = []golang.QuarantineTarget{
		{
			Package: "github.com/smartcontractkit/branch-out/golang/example_project",
			Tests:   baseTests,
		},
		{
			Package: "github.com/smartcontractkit/branch-out/golang/example_project/test_package",
			Tests:   testPackageTests,
		},
		{
			Package: "github.com/smartcontractkit/branch-out/golang/example_project/oddly_named_package",
			Tests:   oddlyNamedPackageTests,
		},
	}

	nestedProjectQuarantineTargets = []golang.QuarantineTarget{
		{
			Package: "github.com/smartcontractkit/branch-out/golang/example_project/nested_project",
			Tests:   baseTests,
		},
		{
			Package: "github.com/smartcontractkit/branch-out/golang/example_project/nested_project/nested_test_package",
			Tests:   testPackageTests,
		},
		{
			Package: "github.com/smartcontractkit/branch-out/golang/example_project/nested_project/nested_oddly_named_package",
			Tests:   oddlyNamedPackageTests,
		},
	}

	allQuarantineTargets = append(baseProjectQuarantineTargets, nestedProjectQuarantineTargets...)
)
