package testhelpers

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	exampleProjectPackage = "github.com/smartcontractkit/branch-out/golang/example_project"
	oddlyNamedPackage     = "github.com/smartcontractkit/branch-out/golang/example_project/oddly_named_package"
	testPackage           = "github.com/smartcontractkit/branch-out/golang/example_project/test_package"
)

var (
	exampleProjectTests = []string{
		"TestStandard1",
		"TestStandard2",
		"TestStandard3",
		"TestPassSubTestsTableStatic",
		"TestSubTestsTableDynamic",
		"TestDifferentParam",
		"TestPassSubTestsStatic",
		"TestPassSubTestsStatic/subtest_1",
		"TestPassSubTestsStatic/subtest_2",
		"TestSubTestsTableDynamic/subtest_1",
		"TestSubTestsTableDynamic/subtest_2",
		"TestPassSubTestsTableStatic/subtest_1",
		"TestPassSubTestsTableStatic/subtest_2",
		"FuzzExampleProject",
	}
	oddlyNamedPackageTests = []string{
		"TestOddlyNamedPackage",
	}
	testPackageTests = []string{
		"TestTestPackage",
	}
)

func TestParseTestOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		resultsFile string
		expected    map[string]*PackageTestResults
	}{
		{
			name:        "all failed",
			resultsFile: "testdata/all_failed.output",
			expected: map[string]*PackageTestResults{
				exampleProjectPackage: {
					Found:   exampleProjectTests,
					Skipped: []string{},
					Passed:  []string{},
					Failed:  exampleProjectTests,
				},
				oddlyNamedPackage: {
					Found:   oddlyNamedPackageTests,
					Skipped: []string{},
					Passed:  []string{},
					Failed:  oddlyNamedPackageTests,
				},
				testPackage: {
					Found:   testPackageTests,
					Skipped: []string{},
					Passed:  []string{},
					Failed:  testPackageTests,
				},
			},
		},
		{
			name:        "all passed",
			resultsFile: "testdata/all_passed.output",
			expected: map[string]*PackageTestResults{
				exampleProjectPackage: {
					Found:   exampleProjectTests,
					Skipped: []string{},
					Passed:  exampleProjectTests,
					Failed:  []string{},
				},
				oddlyNamedPackage: {
					Found:   oddlyNamedPackageTests,
					Skipped: []string{},
					Passed:  oddlyNamedPackageTests,
					Failed:  []string{},
				},
				testPackage: {
					Found:   testPackageTests,
					Skipped: []string{},
					Passed:  testPackageTests,
					Failed:  []string{},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output, err := os.ReadFile(test.resultsFile)
			require.NoError(t, err, "failed to read json results file")

			results, err := ParseTestOutput(output)
			require.NoError(t, err, "failed to parse test output")

			require.Len(t, results, len(test.expected), "number of packages should match")
			for pkg, expectedResults := range test.expected {
				actualResults, ok := results[pkg]
				require.True(t, ok, "package %s should be present in results", pkg)

				assert.ElementsMatch(
					t,
					expectedResults.Found,
					actualResults.Found,
					"Found tests should match (order doesn't matter)",
				)
				assert.ElementsMatch(
					t,
					expectedResults.Skipped,
					actualResults.Skipped,
					"Skipped tests should match (order doesn't matter)",
				)
				assert.ElementsMatch(
					t,
					expectedResults.Passed,
					actualResults.Passed,
					"Passed tests should match (order doesn't matter)",
				)
				assert.ElementsMatch(
					t,
					expectedResults.Failed,
					actualResults.Failed,
					"Failed tests should match (order doesn't matter)",
				)
				assert.Equal(t, expectedResults.Panic, actualResults.Panic, "Panic status should match")
				assert.Equal(t, expectedResults.Race, actualResults.Race, "Race status should match")
				assert.Equal(t, expectedResults.Timeout, actualResults.Timeout, "Timeout status should match")
			}
		})
	}
}
