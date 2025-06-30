package testhelpers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
)

var (
	// ErrTestBuildFailed is returned when the build failed during the go test run.
	ErrTestBuildFailed = errors.New("test build failed")

	timeoutRe = regexp.MustCompile(`^panic: test timed out after (.*)`)
	panicRe   = regexp.MustCompile(`^panic:`)
	raceRe    = regexp.MustCompile(`^WARNING: DATA RACE`)
)

// PackageTestResults is a summary of the test results for a given package.
type PackageTestResults struct {
	Found   []string `json:"found"`   // All tests that were found in the run
	Skipped []string `json:"skipped"` // All tests that were skipped in the run
	Passed  []string `json:"passed"`  // All tests that passed in the run
	Failed  []string `json:"failed"`  // All tests that failed in the run
	Panic   bool     `json:"panic"`   // Whether a panic occurred in the run
	Race    bool     `json:"race"`    // Whether a race condition occurred in the run
	Timeout bool     `json:"timeout"` // Whether a timeout occurred in the run
}

// testOutputLine is a single line of output from go test -json.
type testOutputLine struct {
	Time    string  `json:"Time,omitempty"`
	Action  string  `json:"Action"`
	Package string  `json:"Package,omitempty"`
	Test    string  `json:"Test,omitempty"`
	Elapsed float64 `json:"Elapsed,omitempty"`
	Output  string  `json:"Output,omitempty"`
}

// ParseTestOutputs parses the output of go test -json and returns basic test result summary.
// It is not a highly rigorous test result parser, but it is enough to get the basic test results.
// There will probably be some odd behavior around races, panics, and other edge cases.
// Meant only for use in integration tests where we're reasonably sure about what we're expecting.
func ParseTestOutputs(jsonOutputs ...[]byte) (map[string]*PackageTestResults, error) {
	allOutputs := append([][]byte{}, jsonOutputs...)

	lines := make([]testOutputLine, 0, len(allOutputs))
	for _, jsonOutput := range allOutputs {
		decoder := json.NewDecoder(bytes.NewReader(jsonOutput))

		for decoder.More() {
			var line testOutputLine
			if err := decoder.Decode(&line); err != nil {
				return nil, fmt.Errorf("error unmarshalling go test -json output: %w", err)
			}
			lines = append(lines, line)
		}
	}

	return analyzeLines(lines)
}

// analyzeLines analyzes the lines of output from go test -json to build results
func analyzeLines(lines []testOutputLine) (map[string]*PackageTestResults, error) {
	allResults := make(map[string]*PackageTestResults)

	for _, line := range lines {
		if line.Action == "build-fail" {
			return nil, ErrTestBuildFailed
		}
		if line.Package != "" {
			if _, ok := allResults[line.Package]; !ok {
				allResults[line.Package] = &PackageTestResults{
					Found:   []string{},
					Skipped: []string{},
					Passed:  []string{},
					Failed:  []string{},
					Panic:   false,
					Race:    false,
					Timeout: false,
				}
			}
		}

		// Only add a test to the found list if it hasn't already been added.
		if line.Test != "" && !slices.Contains(allResults[line.Package].Found, line.Test) {
			allResults[line.Package].Found = append(allResults[line.Package].Found, line.Test)
		}

		// Panics and races will often lie in JSON output when running in parallel, so that the attached line.Test is possibly not the actual test that panicked.
		// We'll just use the line.Test as a best guess.
		if timeoutRe.MatchString(line.Output) { // Timeouts are a special kind of panic
			allResults[line.Package].Timeout = true
			continue
		}

		if panicRe.MatchString(line.Output) {
			allResults[line.Package].Panic = true
			continue
		}

		if raceRe.MatchString(line.Output) {
			allResults[line.Package].Race = true
			continue
		}

		switch line.Action {
		case "build-fail":
			return nil, ErrTestBuildFailed
		case "pass":
			if line.Test != "" && !slices.Contains(allResults[line.Package].Passed, line.Test) {
				allResults[line.Package].Passed = append(
					allResults[line.Package].Passed,
					line.Test,
				)
			}
		case "fail":
			if line.Test != "" && !slices.Contains(allResults[line.Package].Failed, line.Test) {
				allResults[line.Package].Failed = append(
					allResults[line.Package].Failed,
					line.Test,
				)
			}
		case "skip":
			if line.Test != "" && !slices.Contains(allResults[line.Package].Skipped, line.Test) {
				allResults[line.Package].Skipped = append(
					allResults[line.Package].Skipped,
					line.Test,
				)
			}
		}
	}

	for _, results := range allResults {
		slices.Sort(results.Found)
		slices.Sort(results.Skipped)
		slices.Sort(results.Passed)
		slices.Sort(results.Failed)
	}

	return allResults, nil
}
