// Package golang provides utilities for the Go programming language.
package golang

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

// UnquarantineResults describes the result of unquarantining multiple packages.
// The key is the import path of the package, and the value is the result of unquarantining that package.
type UnquarantineResults map[string]UnquarantinePackageResults

// String returns a string representation of the unquarantine results.
// Good for debugging and logging.
func (u UnquarantineResults) String() string {
	return generateResultsString(u, "unquarantined")
}

// Markdown returns a Markdown representation of the unquarantine results.
// Good for a PR description.
func (u UnquarantineResults) Markdown(owner, repo, branch string) string {
	return generateResultsMarkdown(u, "unquarantined", owner, repo, branch)
}

// UnquarantinePackageResults describes the result of unquarantining a list of tests in a package.
type UnquarantinePackageResults struct {
	Package   string              // Import path of the Go package (redundant, but kept for handy access)
	Successes []UnquarantinedFile // Every file where we found and unquarantined tests
	Failures  []string            // Names of the test functions that were not able to be unquarantined
}

// SuccessfulTestsCount returns the number of tests that were successfully unquarantined.
func (u UnquarantinePackageResults) SuccessfulTestsCount() int {
	count := 0
	for _, success := range u.Successes {
		count += len(success.TestNames())
	}
	return count
}

// UnquarantinedFile describes the outputs of successfully unquarantining a list of tests in a package in a single file.
type UnquarantinedFile struct {
	Package            string              // Import path of the Go package (redundant, but kept for handy access)
	File               string              // Relative path to the file where the tests were found and unquarantined (if any)
	FileAbs            string              // Absolute path to the file where the tests were found and unquarantined on the local filesystem (if any)
	Tests              []UnquarantinedTest // All the test functions successfully unquarantined in this file
	ModifiedSourceCode string              // Modified source code to unquarantine the tests (if any)
}

// TestNames returns the names of the test functions that were unquarantined in this file.
func (u UnquarantinedFile) TestNames() []string {
	names := make([]string, len(u.Tests))
	for i, test := range u.Tests {
		names[i] = test.Name
	}
	return names
}

// UnquarantinedTest describes a test function that was unquarantined.
type UnquarantinedTest struct {
	Name         string // Name of the test function that was unquarantined
	OriginalLine int    // Line number of the test function that was unquarantined
}

// UnquarantineOption is a function that can be used to configure the unquarantine process.
type UnquarantineOption func(*unquarantineOptions)

// unquarantineOptions describes the options for the unquarantine process.
type unquarantineOptions struct {
	buildFlags []string
}

// WithUnquarantineBuildFlags sets the build flags to use when loading packages.
func WithUnquarantineBuildFlags(buildFlags []string) UnquarantineOption {
	return func(options *unquarantineOptions) {
		options.buildFlags = buildFlags
	}
}

// UnquarantineTests looks through a Go project to find and unquarantine any tests that match the given targets.
// It returns a list of results for each target, including whether it was able to be unquarantined, and the modified source code to unquarantine the test.
// The modified source code is returned so that it can be committed to the repository.
// You must do something with it, as the code is not edited or committed by this function.
//
// Tests unquarantined by this process will have their skip logic completely removed.
func UnquarantineTests(
	l zerolog.Logger,
	repoPath string,
	unquarantineTargets []TestTarget,
	options ...UnquarantineOption,
) (UnquarantineResults, error) {
	unquarantineOptions := &unquarantineOptions{
		buildFlags: []string{},
	}
	for _, option := range options {
		option(unquarantineOptions)
	}

	packages, err := Packages(l, repoPath, unquarantineOptions.buildFlags)
	if err != nil {
		return nil, err
	}

	var (
		sanitizedTargets    = sanitizeTestTargets(unquarantineTargets)
		testsToUnquarantine int
	)
	for _, target := range sanitizedTargets { // Calculate the largest possible amount of results by how many tests we have to unquarantine
		testsToUnquarantine += len(target.Tests)
	}

	start := time.Now()
	l.Info().Msg("Unquarantining tests")
	var (
		packageResultsChan = make(chan UnquarantinePackageResults, len(sanitizedTargets))
		eg                 = errgroup.Group{}
	)

	for _, target := range sanitizedTargets {
		eg.Go(func() error {
			pkg, err := packages.Get(target.Package)
			if err != nil {
				return fmt.Errorf("failed to get package %s: %w", target.Package, err)
			}
			results, err := unquarantinePackage(l, repoPath, pkg, target.Tests)
			packageResultsChan <- results
			return err
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	close(packageResultsChan)

	var (
		successfullyUnquarantined = make([]string, 0, testsToUnquarantine)
		failedToUnquarantine      = make([]string, 0, testsToUnquarantine)
		results                   = make(UnquarantineResults, len(sanitizedTargets))
	)
	for result := range packageResultsChan {
		results[result.Package] = result
		for _, success := range result.Successes {
			for _, test := range success.TestNames() {
				successfullyUnquarantined = append(
					successfullyUnquarantined,
					fmt.Sprintf("%s.%s", success.Package, test),
				)
			}
		}
		for _, failure := range result.Failures {
			failedToUnquarantine = append(failedToUnquarantine, fmt.Sprintf("%s/%s", result.Package, failure))
		}
	}

	l.Info().
		Strs("successfully_unquarantined", successfullyUnquarantined).
		Strs("failed_to_unquarantine", failedToUnquarantine).
		Str("duration", time.Since(start).String()).
		Msg("Unquarantine results")

	return results, nil
}

// WriteUnquarantineResultsToFiles writes successfully unquarantined tests to the file system.
func WriteUnquarantineResultsToFiles(l zerolog.Logger, results UnquarantineResults) error {
	return writeResultsToFiles(l, results, "unquarantine")
}

// unquarantinePackage looks for test functions in all test files in a package and unquarantines them.
func unquarantinePackage(
	l zerolog.Logger,
	repoPath string,
	pkg PackageInfo,
	testsToUnquarantine []string,
) (UnquarantinePackageResults, error) {
	l = l.With().
		Str("package", pkg.ImportPath).
		Strs("tests_to_unquarantine", testsToUnquarantine).
		Strs("test_files", pkg.TestGoFiles).
		Logger()
	l.Debug().Msg("Unquarantining tests in package")

	var (
		haveUnquarantined = make(map[string]bool) // Track which tests we've already unquarantined for quick lookup
		results           = UnquarantinePackageResults{
			Package:   pkg.ImportPath,
			Successes: make([]UnquarantinedFile, 0, len(testsToUnquarantine)),
			Failures:  make([]string, 0, len(testsToUnquarantine)),
		}
	)

	// Look through each test file in the package to see if we can find any of our target tests to unquarantine.
	for _, testFile := range pkg.TestGoFiles {
		l := l.With().Str("test_file", testFile).Logger()

		// Parse the Go file using AST
		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, testFile, nil, parser.ParseComments)
		if err != nil {
			return results, fmt.Errorf(
				"failed to parse %s while unquarantining tests in %s: %w",
				testFile,
				pkg.ImportPath,
				err,
			)
		}

		foundTests := testsInFile(node, testsToUnquarantine)
		if len(foundTests) == 0 {
			l.Debug().Msg("No target tests found in file")
			continue
		}

		foundTestNames := make([]string, 0, len(foundTests))
		for _, test := range foundTests {
			foundTestNames = append(foundTestNames, test.Name.Name)
		}

		modifiedSource, unquarantinedTests, err := unskipTests(fset, node, foundTests)
		if err != nil {
			return results, fmt.Errorf("failed to unquarantine tests in file %s: %w", testFile, err)
		}

		// Log validation results
		actuallyUnquarantinedNames := make([]string, 0, len(unquarantinedTests))
		for _, test := range unquarantinedTests {
			actuallyUnquarantinedNames = append(actuallyUnquarantinedNames, test.Name)
			haveUnquarantined[test.Name] = true
		}

		// Log warnings for tests that were not actually quarantined
		for _, requestedTest := range foundTestNames {
			wasActuallyUnquarantined := false
			for _, actualTest := range actuallyUnquarantinedNames {
				if requestedTest == actualTest {
					wasActuallyUnquarantined = true
					break
				}
			}
			if !wasActuallyUnquarantined {
				l.Warn().
					Str("test_name", requestedTest).
					Str("file", testFile).
					Msg("Test was not quarantined, skipping unquarantine operation")
			}
		}

		if len(actuallyUnquarantinedNames) > 0 {
			l.Debug().
				Strs("actually_unquarantined_tests", actuallyUnquarantinedNames).
				Msg("Successfully unquarantined tests in file")
		} else {
			l.Debug().Msg("No tests were actually unquarantined in this file")
			continue // Skip adding to results if no tests were actually unquarantined
		}

		absRepoPath, err := filepath.Abs(repoPath)
		if err != nil {
			return results, fmt.Errorf("failed to get absolute path of repo %s: %w", repoPath, err)
		}
		relativeFilePath := strings.TrimPrefix(testFile, absRepoPath)
		relativeFilePath = strings.TrimPrefix(relativeFilePath, string(filepath.Separator))
		results.Successes = append(results.Successes, UnquarantinedFile{
			Package:            pkg.ImportPath,
			File:               relativeFilePath,
			FileAbs:            testFile,
			Tests:              unquarantinedTests,
			ModifiedSourceCode: modifiedSource,
		})
	}

	// Add any tests that were not found to the failures
	for _, test := range testsToUnquarantine {
		if !haveUnquarantined[test] {
			results.Failures = append(results.Failures, test)
		}
	}

	return results, nil
}

// unskipTests removes quarantine logic from the beginning of test functions.
// It looks for and removes the entire conditional block that was added during quarantining:
//
//	if os.Getenv("RUN_QUARANTINED_TESTS") != "true" {
//	    t.Skip("Flaky test quarantined. Ticket <Jira ticket>. Done automatically by branch-out (https://github.com/smartcontractkit/branch-out)")
//	} else {
//	    t.Logf("'RUN_QUARANTINED_TESTS' set to '%s', running quarantined test", os.Getenv("RUN_QUARANTINED_TESTS"))
//	}
//
// Returns an error if validation is enabled and tests are not actually quarantined.
func unskipTests(
	fset *token.FileSet,
	node *ast.File,
	testFuncs []*ast.FuncDecl,
) (string, []UnquarantinedTest, error) {
	// Store original line numbers and test names
	originalPositions := make(map[string]int)
	actuallyQuarantinedTests := make(map[string]bool)

	for _, testFunc := range testFuncs {
		originalPositions[testFunc.Name.Name] = fset.Position(testFunc.Pos()).Line

		// Validate that the test is actually quarantined
		isQuarantined := false
		if testFunc.Body != nil && len(testFunc.Body.List) > 0 {
			if ifStmt, ok := testFunc.Body.List[0].(*ast.IfStmt); ok {
				isQuarantined = isQuarantineConditional(ifStmt)
			}
		}
		actuallyQuarantinedTests[testFunc.Name.Name] = isQuarantined
	}

	// Remove quarantine logic from each test function
	actuallyUnquarantinedTests := make([]UnquarantinedTest, 0, len(testFuncs))
	for _, testFunc := range testFuncs {
		if testFunc.Body != nil && len(testFunc.Body.List) > 0 {
			// Check if the first statement is our quarantine conditional
			if ifStmt, ok := testFunc.Body.List[0].(*ast.IfStmt); ok {
				if isQuarantineConditional(ifStmt) {
					// Remove the first statement (the quarantine conditional)
					testFunc.Body.List = testFunc.Body.List[1:]

					// Record that we actually unquarantined this test
					if originalLine, exists := originalPositions[testFunc.Name.Name]; exists {
						actuallyUnquarantinedTests = append(actuallyUnquarantinedTests, UnquarantinedTest{
							Name:         testFunc.Name.Name,
							OriginalLine: originalLine,
						})
					}
				}
			}
		}
	}

	// Format the modified AST
	var modifiedNode bytes.Buffer
	if err := format.Node(&modifiedNode, fset, node); err != nil {
		return "", nil, fmt.Errorf("failed to format modified source: %w", err)
	}

	modifiedSource := modifiedNode.String()

	return modifiedSource, actuallyUnquarantinedTests, nil
}

// isQuarantineConditional checks if an if statement is the quarantine conditional block
// by looking for the specific pattern we use for quarantining.
func isQuarantineConditional(ifStmt *ast.IfStmt) bool {
	// Check if the condition is: os.Getenv("RUN_QUARANTINED_TESTS") != "true"
	if binExpr, ok := ifStmt.Cond.(*ast.BinaryExpr); ok {
		if binExpr.Op == token.NEQ {
			// Check the left side: os.Getenv("RUN_QUARANTINED_TESTS")
			if callExpr, ok := binExpr.X.(*ast.CallExpr); ok {
				if selExpr, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
					if ident, ok := selExpr.X.(*ast.Ident); ok {
						if ident.Name == "os" && selExpr.Sel.Name == "Getenv" {
							// Check the argument to Getenv
							if len(callExpr.Args) == 1 {
								if basicLit, ok := callExpr.Args[0].(*ast.BasicLit); ok {
									if basicLit.Kind == token.STRING &&
										strings.Contains(basicLit.Value, RunQuarantinedTestsEnvVar) {
										// Check the right side: "true"
										if rightLit, ok := binExpr.Y.(*ast.BasicLit); ok {
											if rightLit.Kind == token.STRING && rightLit.Value == `"true"` {
												return true
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	return false
}
