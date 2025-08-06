package golang

import (
	"fmt"
	"os"
	"strings"

	"github.com/rs/zerolog"
)

// OperationType represents the type of operation performed on tests.
type OperationType string

const (
	// OperationQuarantine indicates tests were quarantined.
	OperationQuarantine OperationType = "quarantine"
	// OperationUnquarantine indicates tests were unquarantined.
	OperationUnquarantine OperationType = "unquarantine"
)

// ------------- core domain types ------------------------------------------------

// Test describes a test function that was processed.
type Test struct {
	Name         string // Name of the test function
	OriginalLine int    // Line number of the test function
	ModifiedLine int    // Line number after modification (only used for quarantine)
}

// File describes the outputs of successfully processing tests in a single file.
type File struct {
	Package            string // Import path of the Go package
	File               string // Relative path to the file where the tests were found
	FileAbs            string // Absolute path to the file on the local filesystem
	Tests              []Test // All the test functions successfully processed in this file
	ModifiedSourceCode string // Modified source code after processing the tests
}

// TestNames returns the names of the test functions that were processed in this file.
func (f File) TestNames() []string {
	names := make([]string, len(f.Tests))
	for i, test := range f.Tests {
		names[i] = test.Name
	}
	return names
}

// PackageResults describes the results of processing tests in a package.
type PackageResults struct {
	Package   string   // Import path of the Go package
	Successes []File   // Every file where we found and processed tests
	Failures  []string // Names of the test functions that were not able to be processed
}

// SuccessfulTestsCount returns the number of tests that were successfully processed.
func (p PackageResults) SuccessfulTestsCount() int {
	count := 0
	for _, success := range p.Successes {
		count += len(success.TestNames())
	}
	return count
}

// Results represents the results of processing tests across multiple packages.
type Results map[string]PackageResults

// ForEach iterates over all package results in the collection.
func (r Results) ForEach(fn func(PackageResults)) {
	for _, packageResult := range r {
		fn(packageResult)
	}
}

// String returns a **generic** string representation of the results.
//
// NOTE: For quarantine/unquarantine specific output use ResultsView.
func (r Results) String() string {
	return generateResultsString(r, "processed")
}

// Markdown returns a **generic** Markdown representation of the results.
//
// NOTE: For quarantine/unquarantine specific output use ResultsView.
func (r Results) Markdown(owner, repo, branch string) string {
	return generateResultsMarkdown(r, "processed", owner, repo, branch)
}

// ------------- interfaces -------------------------------------------------------

// MarkdownGenerator defines the interface for types that can generate markdown.
type MarkdownGenerator interface {
	Markdown(owner, repo, branch string) string
}

// StringGenerator defines the interface for types that can generate string representations.
type StringGenerator interface {
	String() string
}

// ResultsInterface combines all common methods for result types.
type ResultsInterface interface {
	MarkdownGenerator
	StringGenerator
}

// ------------- generic, interface-based adapter ---------------------------------

// ResultsView is a lightweight adapter that attaches an OperationType
// to a set of Results and implements String()/Markdown() accordingly.
type ResultsView struct {
	Results       Results
	OperationType OperationType
}

// NewResultsView creates a new adapter for the given Results and operation.
func NewResultsView(r Results, op OperationType) ResultsView {
	return ResultsView{Results: r, OperationType: op}
}

// String implements StringGenerator for ResultsView.
func (v ResultsView) String() string {
	return generateResultsString(v.Results, pastTense(v.OperationType))
}

// Markdown implements MarkdownGenerator for ResultsView.
func (v ResultsView) Markdown(owner, repo, branch string) string {
	return generateResultsMarkdown(v.Results, pastTense(v.OperationType), owner, repo, branch)
}

// Helper to convert an OperationType into the past-tense token used in output.
func pastTense(op OperationType) string {
	switch op {
	case OperationQuarantine:
		return "quarantined"
	case OperationUnquarantine:
		return "unquarantined"
	default:
		return string(op)
	}
}

// ------------- high-level helpers ----------------------------------------------

// GetMarkdown extracts markdown from any MarkdownGenerator implementation.
func GetMarkdown(gen MarkdownGenerator, owner, repo, branch string) string {
	return gen.Markdown(owner, repo, branch)
}

// GetCommitInfo extracts commit information from Results type.
// Returns the full commit message and a map of file paths to their modified content.
func GetCommitInfo(results Results, operationType OperationType) (string, map[string]string) {
	allFileUpdates := make(map[string]string)
	var commitMessage strings.Builder

	commitMessage.WriteString(fmt.Sprintf("branch-out %s tests\n", operationType))

	// Helper function to process a single package result
	processPackageResult := func(packageResult PackageResults) {
		for _, success := range packageResult.Successes {
			commitMessage.WriteString(fmt.Sprintf("%s: %s\n",
				success.File, strings.Join(success.TestNames(), ", ")))
			allFileUpdates[success.File] = success.ModifiedSourceCode
		}
	}

	results.ForEach(processPackageResult)
	return commitMessage.String(), allFileUpdates
}

// ------------- internal helpers -------------------------------------------------

// forEachPackageResult applies a function to each package result in the collection.
func forEachPackageResult(results Results, fn func(PackageResults) error) error {
	var firstError error
	results.ForEach(func(result PackageResults) {
		if firstError == nil {
			if err := fn(result); err != nil {
				firstError = err
			}
		}
	})
	return firstError
}

// writeFileResults is a helper function that writes files for any result type.
func writeFileResults(packageResult PackageResults, l zerolog.Logger, operationType string) error {
	for _, success := range packageResult.Successes {
		if err := os.WriteFile(success.FileAbs, []byte(success.ModifiedSourceCode), 0o600); err != nil {
			return fmt.Errorf("failed to write %s results to %s: %w",
				operationType, success.FileAbs, err)
		}
		l.Trace().
			Str("file", success.FileAbs).
			Str("package", success.Package).
			Strs(operationType+"d_tests", success.TestNames()).
			Msgf("Wrote %s results", operationType)
	}
	return nil
}

// generateResultsMarkdown creates a Markdown representation of test results.
func generateResultsMarkdown(
	results Results,
	operationPast, // "quarantined" | "unquarantined" | "processed"
	owner, repo, branch string,
) string {
	var md strings.Builder

	switch operationPast {
	case "quarantined":
		md.WriteString("# Quarantined Flaky Tests using branch-out\n\n")
	case "unquarantined":
		md.WriteString("# Unquarantined Recovered Tests using branch-out\n\n")
	default:
		md.WriteString("# Processed Tests using branch-out\n\n")
	}

	_ = forEachPackageResult(results, func(result PackageResults) error {
		generatePackageMarkdown(&md, result, operationPast, owner, repo, branch)
		return nil
	})

	md.WriteString("\n\n---\n\n")
	md.WriteString("Created automatically by [branch-out](https://github.com/smartcontractkit/branch-out).")
	return md.String()
}

// generatePackageMarkdown generates markdown for a single package result.
func generatePackageMarkdown(
	md *strings.Builder,
	result PackageResults,
	operationPast, owner, repo, branch string,
) {
	emoji := "🟢"
	if len(result.Failures) > 0 {
		emoji = "🔴"
	}
	fmt.Fprintf(md, "## `%s` %s\n\n", result.Package, emoji)

	// Successes
	if len(result.Successes) > 0 {
		fmt.Fprintf(md, "### Successfully %s %d tests\n\n",
			operationPast, result.SuccessfulTestsCount())
		md.WriteString("| File | Tests |\n")
		md.WriteString("|------|-------|\n")

		for _, success := range result.Successes {
			generateFileMarkdown(md, success, owner, repo, branch)
		}
		md.WriteString("\n")
	}

	// Failures
	if len(result.Failures) > 0 {
		// Use the infinitive form for "Failed to X" (quarantine/unquarantine)
		var infinitiveForm string
		switch operationPast {
		case "quarantined":
			infinitiveForm = "quarantine"
		case "unquarantined":
			infinitiveForm = "unquarantine"
		default:
			infinitiveForm = "process"
		}
		fmt.Fprintf(md, "### Failed to %s %d tests. Need manual intervention!\n\n",
			infinitiveForm, len(result.Failures))
		for _, test := range result.Failures {
			fmt.Fprintf(md, "- %s\n", test)
		}
		md.WriteString("\n")
	}
}

// generateFileMarkdown generates markdown for a single file result.
func generateFileMarkdown(md *strings.Builder, file File, owner, repo, branch string) {
	githubBlobURL := fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s",
		owner, repo, branch, file.File)

	var testLinks []string
	for _, test := range file.Tests {
		link := fmt.Sprintf("[%s](%s#L%d)", test.Name, githubBlobURL, test.OriginalLine)
		testLinks = append(testLinks, link)
	}

	fmt.Fprintf(md, "| [%s](%s) | %s |\n",
		file.File, githubBlobURL, strings.Join(testLinks, ", "))
}

// generateResultsString creates a string representation of test results.
func generateResultsString(
	results Results,
	operationPast string, // "quarantined" | "unquarantined" | "processed"
) string {
	var b strings.Builder
	_ = forEachPackageResult(results, func(result PackageResults) error {
		generatePackageString(&b, result, operationPast)
		return nil
	})
	return b.String()
}

// generatePackageString generates string representation for a single package result.
func generatePackageString(
	b *strings.Builder,
	result PackageResults,
	operationPast string,
) {
	b.WriteString(result.Package + "\n")
	b.WriteString("--------------------------------\n")

	if len(result.Successes) > 0 {
		b.WriteString("Successes\n\n")
		for _, success := range result.Successes {
			generateFileString(b, success, operationPast)
		}
	} else {
		b.WriteString("\nNo successes!\n")
	}

	if len(result.Failures) > 0 {
		b.WriteString("\nFailures\n\n")
		for _, failure := range result.Failures {
			fmt.Fprintln(b, failure)
		}
	} else {
		b.WriteString("\nNo failures!\n")
	}
}

// generateFileString generates string representation for a single file result.
func generateFileString(b *strings.Builder, file File, operationPast string) {
	if len(file.TestNames()) > 0 {
		fmt.Fprintf(b, "%s: %s\n",
			file.File, strings.Join(file.TestNames(), ", "))
	} else {
		fmt.Fprintf(b, "%s: No tests %s\n", file.File, operationPast)
	}
}

// writeResultsToFiles writes test results to the file system.
func writeResultsToFiles(
	l zerolog.Logger,
	results Results,
	operationType string, // "quarantine" | "unquarantine" – for logging only
) error {
	return forEachPackageResult(results, func(result PackageResults) error {
		return writeFileResults(result, l, operationType)
	})
}
