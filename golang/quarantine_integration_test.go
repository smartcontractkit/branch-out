package golang

import (
	"fmt"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIntegrationQuarantine(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration quarantine test in short mode")
	}

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

				err := runExampleTest(t, exampleProjectDir, tc.testName)
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
