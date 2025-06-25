// Package golang_test provides integration tests for the golang package, modifying and running test code.
package golang_test

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/internal/testhelpers"
)

func TestQuarantine(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		packageName string
		testName    string
	}{
		{testName: "TestStandard1"},
		{testName: "TestStandard2"},
		{testName: "TestStandard3"},
		{testName: "TestPassSubTestsStatic"},
		{testName: "TestPassSubTestsTableStatic"},
		{testName: "TestSubTestsTableDynamic"},
	}

	for _, pkg := range exampleProjectPackages {
		for _, tc := range testCases {
			t.Run(fmt.Sprintf("%s/%s", pkg, tc.testName), func(t *testing.T) {
				t.Parallel()

				dir := setupExampleTests(t)

				err := runExampleTest(t, dir, tc.testName)
				require.NoError(t, err, "if properly quarantined, no tests should fail")
			})
		}
	}
}

func runExampleTest(tb testing.TB, dir, testName string) error {
	tb.Helper()

	testCmd := exec.Command("go", "test", "-tags=example_project", "./...", "-run", testName, "-v", "-count=1")
	testCmd.Dir = dir

	output, err := testCmd.CombinedOutput()
	tb.Cleanup(func() {
		if tb.Failed() {
			tb.Logf("example test output:\n%s", output)
		}
	})

	return err
}

func setupExampleTests(tb testing.TB) string {
	tb.Helper()

	dirName := strings.ReplaceAll(tb.Name(), "/", "_")
	err := os.MkdirAll(dirName, 0750)
	require.NoError(tb, err, "failed to create test directory")

	tb.Cleanup(func() {
		if tb.Failed() {
			tb.Logf("leaving test directory '%s' for debugging", dirName)
			return
		}

		if err := os.RemoveAll(dirName); err != nil {
			tb.Logf("failed to remove test directory: %s", err) //nolint:gosec // we don't care if this fails
		}
	})

	err = testhelpers.CopyDir(tb, exampleProjectDir, dirName)
	require.NoError(tb, err, "failed to copy example tests")

	return dirName
}
