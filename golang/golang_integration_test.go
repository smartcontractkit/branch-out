package golang_test

import (
	"os/exec"
	"testing"
)

const exampleTestsDir = "example_tests"

func TestExampleTests(t *testing.T) {
	t.Parallel()

	runExampleTests(t, exampleTestsDir)
}

func runExampleTests(t *testing.T, dir string) {
	t.Helper()

	testCmd := exec.Command("go", "test", "-tags=example_tests", "./...", "-v", "-count=1")
	testCmd.Dir = dir

	output, err := testCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to run example tests: %s\n%s", err, output)
	}

	t.Logf("example tests passed: %s", output)
}
