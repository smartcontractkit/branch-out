package golang

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/internal/testhelpers"
)

func TestQuarantineTests_Integration_All(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration tests in short mode")
	}

	l := testhelpers.Logger(t)
	dir := setupDir(t)

	quarantineResults, err := QuarantineTests(l, dir, allQuarantineTargets, exampleProjectBuildFlags...)
	require.NoError(t, err)

	for _, result := range quarantineResults {
		assert.NoError(t, result.Error, "unexpected error from quarantining test")
		assert.True(t, result.Quarantined, "all tests are expected to be successfully quarantined")

		if result.Quarantined {
			err := os.WriteFile(filepath.Join(dir, result.File), []byte(result.ModifiedSourceCode), 0644)
			require.NoError(t, err, "failed to write modified source code to file")
		}
	}

	testResults := runExampleTests(t, dir)

	for _, target := range allQuarantineTargets {
		pkgResults, ok := testResults[target.PackageName]
		require.True(t, ok, "package %s not found in test results", target.PackageName)
		require.Contains(
			t,
			pkgResults.Found,
			target.TestName,
			fmt.Sprintf("test should be found in package %s", target.PackageName),
		)
		assert.Contains(
			t,
			pkgResults.Skipped,
			target.TestName,
			fmt.Sprintf("test should be skipped in package %s", target.PackageName),
		)
	}
}

// runExampleTests runs go test for the example_project and returns the test results.
// It can optionally run only a subset of tests by passing in the test names.
func runExampleTests(
	tb testing.TB,
	dir string,
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
	require.NoError(tb, err, "failed to run example tests")

	testResults, err := testhelpers.ParseTestOutput(combinedOutput)
	require.NoError(tb, err, "failed to parse test output")

	return testResults
}

func setupDir(tb testing.TB) string {
	tb.Helper()

	targetDir := tb.TempDir()
	err := testhelpers.CopyDir(tb, "example_project", targetDir)
	require.NoError(tb, err, "failed to copy example project to temp dir for testing")

	return targetDir
}

var (
	baseProjectQuarantineTargets = []QuarantineTarget{
		{PackageName: "github.com/smartcontractkit/branch-out/golang/example_project", TestName: "TestStandard1"},
		{PackageName: "github.com/smartcontractkit/branch-out/golang/example_project", TestName: "TestStandard2"},
		{PackageName: "github.com/smartcontractkit/branch-out/golang/example_project", TestName: "TestStandard3"},
		{
			PackageName: "github.com/smartcontractkit/branch-out/golang/example_project",
			TestName:    "TestPassSubTestsStatic/subtest_1",
		},
		{
			PackageName: "github.com/smartcontractkit/branch-out/golang/example_project",
			TestName:    "TestPassSubTestsStatic/subtest_2",
		},
		{
			PackageName: "github.com/smartcontractkit/branch-out/golang/example_project",
			TestName:    "TestPassSubTestsTableStatic/subtest_1",
		},
		{
			PackageName: "github.com/smartcontractkit/branch-out/golang/example_project",
			TestName:    "TestPassSubTestsTableStatic/subtest_2",
		},
		{
			PackageName: "github.com/smartcontractkit/branch-out/golang/example_project",
			TestName:    "TestSubTestsTableDynamic/subtest_1",
		},
		{
			PackageName: "github.com/smartcontractkit/branch-out/golang/example_project",
			TestName:    "TestSubTestsTableDynamic/subtest_2",
		},
		{
			PackageName: "github.com/smartcontractkit/branch-out/golang/example_project",
			TestName:    "BenchmarkExampleProject",
		},
		{
			PackageName: "github.com/smartcontractkit/branch-out/golang/example_project",
			TestName:    "FuzzExampleProject",
		},
		{
			PackageName: "github.com/smartcontractkit/branch-out/golang/example_project",
			TestName:    "TestDifferentParam",
		},
		{
			PackageName: "github.com/smartcontractkit/branch-out/golang/example_project/test_package",
			TestName:    "TestTestPackage",
		},
		{
			PackageName: "github.com/smartcontractkit/branch-out/golang/example_project/package_name_doesnt_match_dir_name",
			TestName:    "TestOddlyNamedPackage",
		},
	}

	nestedProjectQuarantineTargets = []QuarantineTarget{
		{
			PackageName: "github.com/smartcontractkit/branch-out/golang/example_project/nested_project",
			TestName:    "TestStandard1",
		},
		{
			PackageName: "github.com/smartcontractkit/branch-out/golang/example_project/nested_project",
			TestName:    "TestStandard2",
		},
		{
			PackageName: "github.com/smartcontractkit/branch-out/golang/example_project/nested_project",
			TestName:    "TestStandard3",
		},
		{
			PackageName: "github.com/smartcontractkit/branch-out/golang/example_project/nested_project",
			TestName:    "TestPassSubTestsStatic/subtest_1",
		},
		{
			PackageName: "github.com/smartcontractkit/branch-out/golang/example_project/nested_project",
			TestName:    "TestPassSubTestsStatic/subtest_2",
		},
		{
			PackageName: "github.com/smartcontractkit/branch-out/golang/example_project/nested_project",
			TestName:    "TestPassSubTestsTableStatic/subtest_1",
		},
		{
			PackageName: "github.com/smartcontractkit/branch-out/golang/example_project/nested_project",
			TestName:    "TestPassSubTestsTableStatic/subtest_2",
		},
		{
			PackageName: "github.com/smartcontractkit/branch-out/golang/example_project/nested_project",
			TestName:    "TestSubTestsTableDynamic/subtest_1",
		},
		{
			PackageName: "github.com/smartcontractkit/branch-out/golang/example_project/nested_project",
			TestName:    "TestSubTestsTableDynamic/subtest_2",
		},
		{
			PackageName: "github.com/smartcontractkit/branch-out/golang/example_project/nested_project",
			TestName:    "BenchmarkExampleProject",
		},
		{
			PackageName: "github.com/smartcontractkit/branch-out/golang/example_project/nested_project",
			TestName:    "FuzzExampleProject",
		},
		{
			PackageName: "github.com/smartcontractkit/branch-out/golang/example_project/nested_project",
			TestName:    "TestDifferentParam",
		},
		{
			PackageName: "github.com/smartcontractkit/branch-out/golang/example_project/nested_project/test_package",
			TestName:    "TestTestPackage",
		},
		{
			PackageName: "github.com/smartcontractkit/branch-out/golang/example_project/nested_project/package_name_doesnt_match_dir_name",
			TestName:    "TestOddlyNamedPackage",
		},
	}

	allQuarantineTargets = append(baseProjectQuarantineTargets, nestedProjectQuarantineTargets...)
)
