// Package golang provides utilities for the Go programming language.
package golang

import (
	"fmt"
	"os"
	"strings"

	"github.com/rs/zerolog"
)

// TestResultsInterface defines the common interface for quarantine and unquarantine results.
type TestResultsInterface interface {
	QuarantineResults | UnquarantineResults
}

// PackageResultsInterface defines the common interface for package-level results.
type PackageResultsInterface interface {
	QuarantinePackageResults | UnquarantinePackageResults
}

// FileResultsInterface defines the common interface for file-level results.
type FileResultsInterface interface {
	QuarantinedFile | UnquarantinedFile
}

// TestInterface defines the common interface for individual test results.
type TestInterface interface {
	QuarantinedTest | UnquarantinedTest
}

// forEachResult applies a function to each package result in the collection.
// This eliminates duplication between QuarantineResults and UnquarantineResults processing.
func forEachResult[T TestResultsInterface](results T, fn func(any) error) error {
	switch r := any(results).(type) {
	case QuarantineResults:
		for _, result := range r {
			if err := fn(result); err != nil {
				return err
			}
		}
	case UnquarantineResults:
		for _, result := range r {
			if err := fn(result); err != nil {
				return err
			}
		}
	}
	return nil
}

// forEachResultVoid applies a function to each package result in the collection (no error return).
// This eliminates duplication between QuarantineResults and UnquarantineResults processing.
func forEachResultVoid[T TestResultsInterface](results T, fn func(any)) {
	switch r := any(results).(type) {
	case QuarantineResults:
		for _, result := range r {
			fn(result)
		}
	case UnquarantineResults:
		for _, result := range r {
			fn(result)
		}
	}
}

// writeFileResults is a helper function that writes files for any result type.
func writeFileResults(result any, l zerolog.Logger, operationType string) error {
	var successes []any

	// Extract successes based on type
	switch r := result.(type) {
	case QuarantinePackageResults:
		for _, s := range r.Successes {
			successes = append(successes, s)
		}
	case UnquarantinePackageResults:
		for _, s := range r.Successes {
			successes = append(successes, s)
		}
	}

	// Process each success file
	for _, success := range successes {
		var fileAbs, packageName string
		var modifiedSourceCode string
		var testNames []string

		switch s := success.(type) {
		case QuarantinedFile:
			fileAbs = s.FileAbs
			packageName = s.Package
			modifiedSourceCode = s.ModifiedSourceCode
			testNames = s.TestNames()
		case UnquarantinedFile:
			fileAbs = s.FileAbs
			packageName = s.Package
			modifiedSourceCode = s.ModifiedSourceCode
			testNames = s.TestNames()
		}

		if err := os.WriteFile(fileAbs, []byte(modifiedSourceCode), 0600); err != nil {
			return fmt.Errorf("failed to write %s results to %s: %w", operationType, fileAbs, err)
		}
		l.Trace().
			Str("file", fileAbs).
			Str("package", packageName).
			Strs(operationType+"d_tests", testNames).
			Msgf("Wrote %s results", operationType)
	}
	return nil
}

// generateResultsMarkdown creates a Markdown representation of test results.
// It generates a formatted table for successfully processed tests and lists any failures.
func generateResultsMarkdown[T TestResultsInterface](
	results T,
	operationType, owner, repo, branch string,
) string {
	var md strings.Builder

	// Write header based on operation type
	if operationType == "quarantined" {
		md.WriteString("# Quarantined Flaky Tests using branch-out\n\n")
	} else {
		md.WriteString("# Unquarantined Recovered Tests using branch-out\n\n")
	}

	// Use helper to iterate over results
	forEachResultVoid(results, func(result any) {
		switch r := result.(type) {
		case QuarantinePackageResults:
			generatePackageMarkdown(&md, r, operationType, owner, repo, branch)
		case UnquarantinePackageResults:
			generatePackageMarkdown(&md, r, operationType, owner, repo, branch)
		}
	})

	md.WriteString("\n\n---\n\n")
	md.WriteString("Created automatically by [branch-out](https://github.com/smartcontractkit/branch-out).")
	return md.String()
}

// generatePackageMarkdown generates markdown for a single package result.
func generatePackageMarkdown[T PackageResultsInterface](
	md *strings.Builder,
	result T,
	operationType, owner, repo, branch string,
) {
	var packageName string
	var successCount int
	var failures []string
	var successes []any

	// Extract data based on type
	switch r := any(result).(type) {
	case QuarantinePackageResults:
		packageName = r.Package
		successCount = r.SuccessfulTestsCount()
		failures = r.Failures
		for _, s := range r.Successes {
			successes = append(successes, s)
		}
	case UnquarantinePackageResults:
		packageName = r.Package
		successCount = r.SuccessfulTestsCount()
		failures = r.Failures
		for _, s := range r.Successes {
			successes = append(successes, s)
		}
	}

	// Package header with emoji
	emoji := "🟢"
	if len(failures) > 0 {
		emoji = "🔴"
	}
	fmt.Fprintf(md, "## `%s` %s\n\n", packageName, emoji)

	// Process successes
	if len(successes) > 0 {
		fmt.Fprintf(md, "### Successfully %s %d tests\n\n", operationType, successCount)
		md.WriteString("| File | Tests |\n")
		md.WriteString("|------|-------|\n")

		for _, success := range successes {
			generateFileMarkdown(md, success, owner, repo, branch)
		}
		md.WriteString("\n")
	}

	// Process failures
	if len(failures) > 0 {
		fmt.Fprintf(md, "### Failed to %s %d tests. Need manual intervention!\n\n", operationType, len(failures))
		for _, test := range failures {
			fmt.Fprintf(md, "- %s\n", test)
		}
		md.WriteString("\n")
	}
}

// generateFileMarkdown generates markdown for a single file result.
func generateFileMarkdown(md *strings.Builder, file any, owner, repo, branch string) {
	var fileName string
	var testLinks []string

	switch f := file.(type) {
	case QuarantinedFile:
		fileName = f.File
		githubBlobURL := fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s", owner, repo, branch, f.File)
		for _, test := range f.Tests {
			testLink := fmt.Sprintf("[%s](%s#L%d)", test.Name, githubBlobURL, test.OriginalLine)
			testLinks = append(testLinks, testLink)
		}
	case UnquarantinedFile:
		fileName = f.File
		githubBlobURL := fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s", owner, repo, branch, f.File)
		for _, test := range f.Tests {
			testLink := fmt.Sprintf("[%s](%s#L%d)", test.Name, githubBlobURL, test.OriginalLine)
			testLinks = append(testLinks, testLink)
		}
	}

	githubBlobURL := fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s", owner, repo, branch, fileName)
	fmt.Fprintf(md, "| [%s](%s) | %s |\n", fileName, githubBlobURL, strings.Join(testLinks, ", "))
}

// generateResultsString creates a string representation of test results.
// It handles both quarantine and unquarantine results using generics for type safety.
func generateResultsString[T TestResultsInterface](
	results T,
	actionPast string, // "quarantined" | "unquarantined"
) string {
	var b strings.Builder

	// Use helper to iterate over results
	forEachResultVoid(results, func(result any) {
		switch r := result.(type) {
		case QuarantinePackageResults:
			generatePackageString(&b, r, actionPast)
		case UnquarantinePackageResults:
			generatePackageString(&b, r, actionPast)
		}
	})
	return b.String()
}

// generatePackageString generates string representation for a single package result.
func generatePackageString[T PackageResultsInterface](
	b *strings.Builder,
	result T,
	actionPast string,
) {
	var packageName string
	var failures []string
	var successes []any

	// Extract data based on type
	switch r := any(result).(type) {
	case QuarantinePackageResults:
		packageName = r.Package
		failures = r.Failures
		for _, s := range r.Successes {
			successes = append(successes, s)
		}
	case UnquarantinePackageResults:
		packageName = r.Package
		failures = r.Failures
		for _, s := range r.Successes {
			successes = append(successes, s)
		}
	}

	b.WriteString(packageName)
	b.WriteString("\n")
	b.WriteString("--------------------------------\n")

	if len(successes) > 0 {
		b.WriteString("Successes\n\n")
		for _, success := range successes {
			generateFileString(b, success, actionPast)
		}
	} else {
		b.WriteString("\nNo successes!\n")
	}

	if len(failures) > 0 {
		b.WriteString("\nFailures\n\n")
		for _, failure := range failures {
			fmt.Fprintf(b, "%s\n", failure)
		}
	} else {
		b.WriteString("\nNo failures!\n")
	}
}

// generateFileString generates string representation for a single file result.
func generateFileString(b *strings.Builder, file any, actionPast string) {
	var fileName string
	var testNames []string

	switch f := file.(type) {
	case QuarantinedFile:
		fileName = f.File
		testNames = f.TestNames()
	case UnquarantinedFile:
		fileName = f.File
		testNames = f.TestNames()
	}

	if len(testNames) > 0 {
		fmt.Fprintf(b, "%s: %s\n", fileName, strings.Join(testNames, ", "))
	} else {
		fmt.Fprintf(b, "%s: No tests %s\n", fileName, actionPast)
	}
}

// writeResultsToFiles writes test results to the file system.
// It handles both quarantine and unquarantine results using generics for type safety.
func writeResultsToFiles[T TestResultsInterface](
	l zerolog.Logger,
	results T,
	operationType string, // "quarantine" | "unquarantine" - for logging
) error {
	// Use helper to iterate over results and write files
	return forEachResult(results, func(result any) error {
		return writeFileResults(result, l, operationType)
	})
}
