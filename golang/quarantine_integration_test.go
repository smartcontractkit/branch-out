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

	for _, result := range quarantineResults {
		require.Empty(t, result.Failures, "failed to quarantine these tests in package %s", result.Package)
	}

	testOutput, err := runExampleTests(t, dir)
	assert.NoError( //nolint:testifylint // If there's an error here, it's likely because the tests failed, which doesn't stop us from checking the results
		t,
		err,
		"error while running example tests",
	)

	testResults, err := testhelpers.ParseTestOutput(testOutput)
	require.NoError(t, err, "failed to parse test output")

	for _, target := range allQuarantineTargets {
		pkgResults, ok := testResults[target.Package]
		require.True(t, ok, "package %s not found in test results", target.Package)
		for _, test := range target.Tests {
			require.Contains(
				t,
				pkgResults.Found,
				test,
				"%s wasn't run in package %s", test, target.Package,
			)
			assert.Contains(
				t,
				pkgResults.Skipped,
				test,
				"%s was not skipped in package %s", test, target.Package,
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
		"BenchmarkExampleProject",
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
			Package: "github.com/smartcontractkit/branch-out/golang/example_project/nested_project/nested_oddly_named_package",
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
