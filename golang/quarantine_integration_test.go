package golang

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

	quarantineResults, err := QuarantineTests(l, dir, allQuarantineTargets)
	require.NoError(t, err, "failed to quarantine tests")

	var (
		ableToQuarantine   []string
		unableToQuarantine []string
	)
	for _, result := range quarantineResults {
		for _, success := range result.Successes {
			ableToQuarantine = append(ableToQuarantine, success.Tests...)
			err := os.WriteFile(success.File, []byte(success.ModifiedSourceCode), 0600)
			require.NoError(t, err, "failed to write modified source code to file")
		}
		unableToQuarantine = append(unableToQuarantine, result.Failures...)
	}
	t.Logf("able to quarantine %d tests\n%s\n", len(ableToQuarantine), strings.Join(ableToQuarantine, "\n"))
	t.Logf("unable to quarantine %d tests\n%s\n", len(unableToQuarantine), strings.Join(unableToQuarantine, "\n"))

	testResults := runExampleTests(t, dir, false)

	for _, target := range allQuarantineTargets {
		pkgResults, ok := testResults[target.Package]
		require.True(t, ok, "package %s not found in test results", target.Package)
		for _, test := range target.Tests {
			require.Contains(
				t,
				pkgResults.Found,
				test,
				"test should be found in package %s", target.Package,
			)
			assert.Contains(
				t,
				pkgResults.Skipped,
				test,
				"test should be skipped in package %s", target.Package,
			)
		}
	}
}

// runExampleTests runs go test for the example_project and returns the test results.
// It can optionally run only a subset of tests by passing in the test names.
func runExampleTests(
	tb testing.TB,
	dir string,
	expectRunError bool,
	specificTests ...string,
) map[string]*testhelpers.PackageTestResults {
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

	combinedOutput, err := testCmd.CombinedOutput()
	if expectRunError {
		assert.Error(tb, err, "expected error while running example tests")
	} else {
		assert.NoError(tb, err, "expected no error while running example tests")
	}

	testResults, err := testhelpers.ParseTestOutput(combinedOutput)
	require.NoError(tb, err, "failed to parse test output")

	return testResults
}

func setupDir(tb testing.TB) string {
	tb.Helper()

	targetDir := fmt.Sprintf("%s-copied-code", strings.ReplaceAll(tb.Name(), "/", "_"))
	err := os.MkdirAll(targetDir, 0750)
	require.NoError(tb, err, "failed to create copied code dir")
	tb.Cleanup(func() {
		if tb.Failed() {
			tb.Logf("leaving copied code dir %s for debugging", targetDir)
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
	baseProjectQuarantineTargets = []QuarantineTarget{
		{
			Package: "github.com/smartcontractkit/branch-out/golang/example_project",
			Tests: []string{
				"TestStandard1",
				"TestStandard2",
				"TestStandard3",
				"TestPassSubTestsStatic/subtest_1",
				"TestPassSubTestsStatic/subtest_2",
				"TestPassSubTestsTableStatic/subtest_1",
				"TestPassSubTestsTableStatic/subtest_2",
				"TestSubTestsTableDynamic/subtest_1",
				"TestSubTestsTableDynamic/subtest_2",
				"BenchmarkExampleProject",
				"FuzzExampleProject",
				"TestDifferentParam",
			},
		},
	}

	nestedProjectQuarantineTargets = []QuarantineTarget{
		{
			Package: "github.com/smartcontractkit/branch-out/golang/example_project/nested_project",
			Tests: []string{
				"TestStandard1",
				"TestStandard2",
				"TestStandard3",
				"TestPassSubTestsStatic/subtest_1",
				"TestPassSubTestsStatic/subtest_2",
				"TestPassSubTestsTableStatic/subtest_1",
			},
		},
		{
			Package: "github.com/smartcontractkit/branch-out/golang/example_project/nested_project",
			Tests: []string{
				"TestDifferentParam",
				"TestTestPackage",
				"TestOddlyNamedPackage",
			},
		},
	}

	allQuarantineTargets = append(baseProjectQuarantineTargets, nestedProjectQuarantineTargets...)
)
