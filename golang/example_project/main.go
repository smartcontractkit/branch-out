package example_project

import (
	"fmt"
	"math/rand"
	"os"
	"testing"
)

const (
	TestResultsEnvVar = "EXAMPLE_PROJECT_TESTS_MODE"

	TestResultsModeFail  = "fail"
	TestResultsModeSkip  = "skip"
	TestResultsModePass  = "pass"
	TestResultsModeMixed = "mixed"
)

// Helper is a helper function that will fail, skip, or pass the test based on the environment variable.
func Helper(tb testing.TB, msg string) {
	tb.Helper()

	modes := []string{
		TestResultsModeFail,
		TestResultsModeSkip,
		TestResultsModePass,
	}
	mode := os.Getenv(TestResultsEnvVar)
	if mode == TestResultsModeMixed {
		mode = modes[rand.Intn(len(modes))]
	}

	switch mode {
	case TestResultsModeFail:
		msg = fmt.Sprintf("%s (fail)", msg)
		tb.Fail()
	case TestResultsModeSkip:
		msg = fmt.Sprintf("%s (skip)", msg)
		tb.Skip(msg)
	case TestResultsModePass:
		msg = fmt.Sprintf("%s (pass)", msg)
	default:
		msg = fmt.Sprintf("%s (fail)", msg)
		tb.Fail()
	}

	tb.Log(msg)
}
