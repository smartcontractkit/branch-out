// Package golang_test contains integration tests for the golang package. Tests that require the example_project to be present.
package golang_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/golang"
	"github.com/smartcontractkit/branch-out/internal/testhelpers"
	"github.com/smartcontractkit/branch-out/quarantine"
)

var exampleProjectBuildFlags = []string{
	"-tags", "example_project",
}

func TestQuarantineTests_Integration_EnvVarGate(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration tests in short mode")
	}

	quarantineTargets := []golang.QuarantineTarget{
		{
			Package: baseProjectPackage,
			Tests:   standardTestNames,
		},
	}

	l := testhelpers.Logger(t)
	dir := setupDir(t)

	_, successfullyQuarantinedTests := quarantineTests(t, l, dir, quarantineTargets)

	testCases := []struct {
		runQuarantinedTests string
	}{
		{runQuarantinedTests: "false"},
		{runQuarantinedTests: "true"},
		{runQuarantinedTests: ""},
		{runQuarantinedTests: "not a boolean"},
	}

	for _, testCase := range testCases {
		t.Run(
			fmt.Sprintf("%s='%s'", quarantine.RunQuarantinedTestsEnvVar, testCase.runQuarantinedTests),
			func(t *testing.T) {
				t.Parallel()
				env := map[string]string{
					quarantine.RunQuarantinedTestsEnvVar: testCase.runQuarantinedTests,
				}

				baseTestOutput, _ := runExampleTests( //nolint:testifylint // If there's an error here, it's likely because the tests failed, which doesn't stop us from checking the results
					t,
					dir,
					env,
				)
				testResults, err := testhelpers.ParseTestOutputs(baseTestOutput)
				require.NoError(t, err, "failed to parse test output")

				// Check that the tests we marked as successfully quarantined were actually skipped after running the tests
				for _, successfullyQuarantinedTarget := range successfullyQuarantinedTests {
					pkgResults, ok := testResults[successfullyQuarantinedTarget.Package]
					require.True(t, ok, "package %s not found in test results", successfullyQuarantinedTarget.Package)
					for _, test := range successfullyQuarantinedTarget.Tests {
						require.Contains(
							t,
							pkgResults.Found,
							test,
							"'%s' in package '%s' was marked as successfully quarantined, but was NOT FOUND AT ALL when tests were run",
							test,
							successfullyQuarantinedTarget.Package,
						)
						if testCase.runQuarantinedTests == "true" {
							assert.Contains(
								t,
								pkgResults.Failed,
								test,
								"'%s' in package '%s' was marked as successfully quarantined, but when %s='%s' it should have been run and failed",
								test,
								successfullyQuarantinedTarget.Package,
								quarantine.RunQuarantinedTestsEnvVar,
								testCase.runQuarantinedTests,
							)
						} else {
							assert.Contains(
								t,
								pkgResults.Skipped,
								test,
								"'%s' in package '%s' was marked as successfully quarantined, but when %s='%s' it should have been skipped",
								test,
								successfullyQuarantinedTarget.Package,
								quarantine.RunQuarantinedTestsEnvVar,
								testCase.runQuarantinedTests,
							)
						}
					}
				}
			},
		)
	}
}

func TestQuarantineTests_Integration(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration tests in short mode")
	}

	testCases := []struct {
		name              string
		quarantineTargets []golang.QuarantineTarget
	}{
		{name: "standard base tests", quarantineTargets: []golang.QuarantineTarget{
			{
				Package: baseProjectPackage,
				Tests:   standardTestNames,
			},
		}},
		{name: "standard nested tests", quarantineTargets: []golang.QuarantineTarget{
			{
				Package: nestedProjectPackage,
				Tests:   standardTestNames,
			},
		}},
		{name: "sub tests", quarantineTargets: []golang.QuarantineTarget{
			{
				Package: baseProjectPackage,
				Tests:   subTestNames,
			},
		}},
		{name: "sub tests nested", quarantineTargets: []golang.QuarantineTarget{
			{
				Package: nestedProjectPackage,
				Tests:   subTestNames,
			},
		}},
		{name: "unusual tests", quarantineTargets: []golang.QuarantineTarget{
			{
				Package: baseProjectPackage,
				Tests:   unusualTestNames,
			},
		}},
		{name: "unusual tests nested", quarantineTargets: []golang.QuarantineTarget{
			{
				Package: nestedProjectPackage,
				Tests:   unusualTestNames,
			},
		}},
		{name: "test package tests", quarantineTargets: []golang.QuarantineTarget{
			{
				Package: baseProjectTestPackage,
				Tests:   testPackageTestNames,
			},
		}},
		{name: "test package tests nested", quarantineTargets: []golang.QuarantineTarget{
			{
				Package: nestedProjectTestPackage,
				Tests:   testPackageTestNames,
			},
		}},
		{name: "oddly named package tests", quarantineTargets: []golang.QuarantineTarget{
			{
				Package: baseProjectOddlyNamedPackage,
				Tests:   oddlyNamedPackageTestNames,
			},
		}},
		{name: "oddly named package tests nested", quarantineTargets: []golang.QuarantineTarget{
			{
				Package: nestedProjectOddlyNamedPackage,
				Tests:   oddlyNamedPackageTestNames,
			},
		}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			casesToSkip := []string{ // These test cases are knowingly broken for now
				"sub tests",
				"sub tests nested",
				"test package tests",
				"test package tests nested",
			}
			if slices.Contains(casesToSkip, testCase.name) {
				t.Skip("skipping known broken test case")
			}

			l := testhelpers.Logger(t)
			dir := setupDir(t)
			_, successfullyQuarantinedTests := quarantineTests(t, l, dir, testCase.quarantineTargets)

			// Run all tests
			baseTestOutput, _ := runExampleTests( //nolint:testifylint // If there's an error here, it's likely because the tests failed, which doesn't stop us from checking the results
				t,
				dir,
				map[string]string{},
			)
			nestedTestOutput, _ := runExampleTests( //nolint:testifylint // If there's an error here, it's likely because the tests failed, which doesn't stop us from checking the results
				t,
				filepath.Join(dir, "nested_project"),
				map[string]string{},
			)
			testResults, err := testhelpers.ParseTestOutputs(baseTestOutput, nestedTestOutput)
			require.NoError(t, err, "failed to parse test output")

			// Check that the tests we marked as successfully quarantined were actually skipped after running the tests
			for _, successfullyQuarantinedTarget := range successfullyQuarantinedTests {
				pkgResults, ok := testResults[successfullyQuarantinedTarget.Package]
				require.True(t, ok, "package %s not found in test results", successfullyQuarantinedTarget.Package)
				for _, test := range successfullyQuarantinedTarget.Tests {
					require.Contains(
						t,
						pkgResults.Found,
						test,
						"'%s' in package '%s' was marked as successfully quarantined, but was NOT FOUND AT ALL when tests were run",
						test,
						successfullyQuarantinedTarget.Package,
					)
					assert.Contains(
						t,
						pkgResults.Skipped,
						test,
						"'%s' in package '%s' was marked as successfully quarantined, but NOT SKIPPED when tests were run",
						test,
						successfullyQuarantinedTarget.Package,
					)
				}
			}
		})
	}

}

// runExampleTestsWithEnv runs go test for the example_project with additional env vars
// and returns the test results.
// It returns the test output and any error that occurred while running the tests.
// It can optionally run only a subset of tests by passing in the test names.
func runExampleTests(
	tb testing.TB,
	dir string,
	env map[string]string,
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

	// inherit the current environment and apply overrides/additions
	testCmd.Env = os.Environ()
	for k, v := range env {
		testCmd.Env = append(testCmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

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

// quarantineTests quarantines the tests in the given targets and returns the quarantine results and a list of the tests that were successfully quarantined for easy assertions
func quarantineTests(
	t *testing.T,
	l zerolog.Logger,
	dir string,
	targets []golang.QuarantineTarget,
) (golang.QuarantineResults, []golang.QuarantineTarget) {
	t.Helper()

	quarantineResults, err := golang.QuarantineTests(
		l,
		dir,
		targets,
		golang.WithBuildFlags(exampleProjectBuildFlags),
	)
	require.NoError(t, err, "failed to run quarantine function")
	err = golang.WriteQuarantineResultsToFiles(l, quarantineResults)
	require.NoError(t, err, "failed to write quarantine results to files")

	// Build a list of all the tests we successfully quarantined to check in our runs later
	// Don't bother checking the tests that we know weren't quarantined
	successfullyQuarantinedTests := []golang.QuarantineTarget{}
	for _, result := range quarantineResults {
		for _, success := range result.Successes {
			successfullyQuarantinedTests = append(successfullyQuarantinedTests, golang.QuarantineTarget{
				Package: success.Package,
				Tests:   success.TestNames(),
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

	return quarantineResults, successfullyQuarantinedTests
}

var (
	baseProjectPackage           = "github.com/smartcontractkit/branch-out-example-project"
	baseProjectTestPackage       = "github.com/smartcontractkit/branch-out-example-project/test_package"
	baseProjectOddlyNamedPackage = "github.com/smartcontractkit/branch-out-example-project/oddly_named_package"

	nestedProjectPackage           = "github.com/smartcontractkit/branch-out-example-project/nested_project"
	nestedProjectTestPackage       = "github.com/smartcontractkit/branch-out-example-project/nested_project/nested_test_package"
	nestedProjectOddlyNamedPackage = "github.com/smartcontractkit/branch-out-example-project/nested_project/nested_oddly_named_package"

	standardTestNames = []string{
		"TestStandard1",
		"TestStandard2",
		"TestStandard3",
	}
	subTestNames = []string{
		"TestSubTestsStatic/subtest_1",
		"TestSubTestsStatic/subtest_2",
		"TestSubTestsTableStatic/subtest_1",
		"TestSubTestsTableStatic/subtest_2",
		"TestSubTestsTableDynamic/subtest_1",
		"TestSubTestsTableDynamic/subtest_2",
	}
	unusualTestNames = []string{
		"FuzzExampleProject",
		"TestDifferentParam",
	}
	testPackageTestNames = []string{
		"TestTestPackage",
	}
	oddlyNamedPackageTestNames = []string{
		"TestOddlyNamedPackage",
	}
)
