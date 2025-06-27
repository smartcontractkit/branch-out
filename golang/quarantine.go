// Package golang provides utilities for the Go programming language.
package golang

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

// QuarantineTarget describes a package and a list of test functions to quarantine.
type QuarantineTarget struct {
	Package string   // Import path of the Go package
	Tests   []string // Names of the test functions in the package to quarantine
}

// QuarantineResults describes the result of quarantining multiple packages.
// The key is the import path of the package, and the value is the result of quarantining that package.
type QuarantineResults map[string]QuarantinePackageResults

// QuarantinePackageResults describes the result of quarantining a list of tests in a package.
type QuarantinePackageResults struct {
	Package   string            // Import path of the Go package (redundant, but kept for handy access)
	Successes []QuarantinedFile // Every file where we found and quarantined tests
	Failures  []string          // Names of the test functions that were not able to be quarantined
}

// QuarantinedFile describes the outputs of successfully quarantining a list of tests in a package in a single file.
type QuarantinedFile struct {
	Package            string   // Import path of the Go package (redundant, but kept for handy access)
	File               string   // File where the tests were found and quarantined (if any)
	Tests              []string // Names of all the test functions successfully quarantined in this file
	ModifiedSourceCode string   // Modified source code to quarantine the tests (if any)
}

// QuarantineMode describes the mode of quarantine to use.
type QuarantineMode string

const (
	// QuarantineModeCodeSkip adds t.Skip() to the test function.
	QuarantineModeCodeSkip QuarantineMode = "code_skip"
	// QuarantineModeComment only adds a comment to the test function to indicate that it was quarantined, but does not modify execution.
	QuarantineModeComment QuarantineMode = "comment"
)

// QuarantineOption is a function that can be used to configure the quarantine process.
type QuarantineOption func(*quarantineOptions)

// quarantineOptions describes the options for the quarantine process.
type quarantineOptions struct {
	mode QuarantineMode
}

// WithQuarantineMode sets the mode of quarantine to use.
func WithQuarantineMode(mode QuarantineMode) QuarantineOption {
	return func(options *quarantineOptions) {
		options.mode = mode
	}
}

// QuarantineTests looks through a Go project to find and quarantine any tests that match the given targets.
// It returns a list of results for each target, including whether it was able to be quarantined, and the modified source code to quarantine the test.
// The modified source code is returned so that it can be committed to the repository. You must do something with it, as the code is not edited or committed by this function.
func QuarantineTests(
	l zerolog.Logger,
	repoPath string,
	quarantineTargets []QuarantineTarget,
	buildFlags []string, // Passed to go command when loading packages
	options ...QuarantineOption,
) (QuarantineResults, error) {
	quarantineOptions := &quarantineOptions{
		mode: QuarantineModeCodeSkip,
	}
	for _, option := range options {
		option(quarantineOptions)
	}
	l = l.With().Str("repo_path", repoPath).Str("mode", string(quarantineOptions.mode)).Logger()

	packages, err := Packages(l, repoPath, buildFlags)
	if err != nil {
		return nil, err
	}

	var (
		sanitizedTargets  = sanitizeQuarantineTargets(quarantineTargets)
		testsToQuarantine int
	)
	for _, target := range sanitizedTargets { // Calculate the largest possible amount of results by how many tests we have to quarantine
		testsToQuarantine += len(target.Tests)
	}

	start := time.Now()
	l.Info().Int("quarantine_targets", len(sanitizedTargets)).Msg("Quarantining tests")
	var (
		packageResultsChan = make(chan QuarantinePackageResults, len(sanitizedTargets))
		eg                 = errgroup.Group{}
	)

	for _, target := range sanitizedTargets {
		eg.Go(func() error {
			pkg, err := packages.Get(target.Package)
			if err != nil {
				return fmt.Errorf("failed to get package %s: %w", target.Package, err)
			}
			results, err := quarantinePackage(l, pkg, target.Tests, quarantineOptions.mode)
			packageResultsChan <- results
			return err
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	close(packageResultsChan)

	var (
		successfullyQuarantined = make([]string, 0, testsToQuarantine)
		failedToQuarantine      = make([]string, 0, testsToQuarantine)
		results                 = make(QuarantineResults, len(sanitizedTargets))
	)
	for result := range packageResultsChan {
		results[result.Package] = result
		for _, success := range result.Successes {
			successfullyQuarantined = append(successfullyQuarantined, success.Tests...)
		}
		failedToQuarantine = append(failedToQuarantine, result.Failures...)
	}

	l.Info().
		Strs("successfully_quarantined", successfullyQuarantined).
		Strs("failed_to_quarantine", failedToQuarantine).
		Str("duration", time.Since(start).String()).
		Msg("Quarantine results")

	return results, nil
}

// sanitizeQuarantineTargets condenses the quarantine targets and removes duplicates.
func sanitizeQuarantineTargets(quarantineTargets []QuarantineTarget) []QuarantineTarget {
	// Package -> tests
	seen := make(map[string][]string)
	for _, target := range quarantineTargets {
		if _, ok := seen[target.Package]; !ok {
			seen[target.Package] = make([]string, 0, len(target.Tests))
		}
		for _, test := range target.Tests {
			if !slices.Contains(seen[target.Package], test) {
				seen[target.Package] = append(seen[target.Package], test)
			}
		}
	}

	sanitizedTargets := make([]QuarantineTarget, 0, len(seen))
	for packageName, tests := range seen {
		sanitizedTargets = append(sanitizedTargets, QuarantineTarget{
			Package: packageName,
			Tests:   tests,
		})
	}
	return sanitizedTargets
}

// quarantinePackage looks for test functions in all test files in a package and quarantines them.
func quarantinePackage(
	l zerolog.Logger,
	pkg PackageInfo,
	testsToQuarantine []string,
	mode QuarantineMode,
) (QuarantinePackageResults, error) {
	l = l.With().
		Str("package", pkg.ImportPath).
		Strs("tests_to_quarantine", testsToQuarantine).
		Logger()
	l.Trace().Msg("Quarantining tests")

	var (
		haveQuarantined = make(map[string]bool) // Track which tests we've already quarantined for quick lookup
		results         = QuarantinePackageResults{
			Package:   pkg.ImportPath,
			Successes: make([]QuarantinedFile, 0, len(testsToQuarantine)),
			Failures:  make([]string, 0, len(testsToQuarantine)),
		}
	)

	// Look through each test file in the package to see if we can find any of our target tests to quarantine.
	for _, testFile := range pkg.TestGoFiles {
		l = l.With().Str("test_file", testFile).Logger()

		// Parse the Go file using AST
		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, testFile, nil, parser.ParseComments)
		if err != nil {
			return results, fmt.Errorf(
				"failed to parse %s while quarantining tests in %s: %w",
				testFile,
				pkg.ImportPath,
				err,
			)
		}

		foundTests := testsInFile(node, testsToQuarantine)
		foundTestNames := make([]string, 0, len(foundTests))
		for _, test := range foundTests {
			haveQuarantined[test.Name.Name] = true
			foundTestNames = append(foundTestNames, test.Name.Name)
		}

		var modifiedSource string
		switch mode {
		case QuarantineModeCodeSkip:
			modifiedSource, err = quarantineTestsSkip(fset, node, foundTests)
		case QuarantineModeComment:
			modifiedSource, err = quarantineTestsComment(fset, node, foundTests)
		default:
			return results, fmt.Errorf("invalid quarantine mode: %s", mode)
		}
		if err != nil {
			return results, fmt.Errorf("failed to quarantine tests in file %s: %w", testFile, err)
		}

		l.Trace().Strs("quarantined_tests", foundTestNames).Msg("Quarantined tests in file")

		results.Successes = append(results.Successes, QuarantinedFile{
			Package:            pkg.ImportPath,
			File:               testFile,
			Tests:              foundTestNames,
			ModifiedSourceCode: modifiedSource,
		})
	}

	// Add any tests that were not found to the failures
	for _, test := range testsToQuarantine {
		if !haveQuarantined[test] {
			results.Failures = append(results.Failures, test)
		}
	}

	return results, nil
}

// testsInFile searches for all test functions in the Go test file's AST that match the given test names.
func testsInFile(node *ast.File, testNames []string) []*ast.FuncDecl {
	found := make([]*ast.FuncDecl, 0, len(node.Decls))
	for _, decl := range node.Decls {
		// Check if this declaration is a function
		if funcDecl, ok := decl.(*ast.FuncDecl); ok {
			// Check if it's a test function with the right name
			if isTestFunction(funcDecl) && slices.Contains(testNames, funcDecl.Name.Name) {
				found = append(found, funcDecl)
			}
		}
	}
	return found
}

// isTestFunction checks if a function declaration is a test function
func isTestFunction(funcDecl *ast.FuncDecl) bool {
	if funcDecl.Name == nil {
		return false
	}

	if !strings.HasPrefix(funcDecl.Name.Name, "Test") && !strings.HasPrefix(funcDecl.Name.Name, "Fuzz") {
		return false
	}

	// Check the function signature
	if funcDecl.Type.Params == nil || len(funcDecl.Type.Params.List) != 1 {
		return false
	}

	param := funcDecl.Type.Params.List[0]

	// Check if parameter is *testing.T or *testing.F
	if starExpr, ok := param.Type.(*ast.StarExpr); ok {
		if selectorExpr, ok := starExpr.X.(*ast.SelectorExpr); ok {
			if ident, ok := selectorExpr.X.(*ast.Ident); ok {
				return (ident.Name == "testing" && (selectorExpr.Sel.Name == "T" || selectorExpr.Sel.Name == "F"))
			}
		}
	}

	return false
}

// quarantineTestsSkip adds t.Skip() to the beginning of the test function
func quarantineTestsSkip(fset *token.FileSet, node *ast.File, testFuncs []*ast.FuncDecl) (string, error) {
	for _, testFunc := range testFuncs {
		paramName := "t" // default fallback
		if testFunc.Type.Params != nil && len(testFunc.Type.Params.List) > 0 {
			param := testFunc.Type.Params.List[0]
			if len(param.Names) > 0 && param.Names[0] != nil {
				paramName = param.Names[0].Name
			}
		}

		// Create the paramName.Skip() statement
		skipCall := &ast.ExprStmt{
			X: &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   &ast.Ident{Name: paramName},
					Sel: &ast.Ident{Name: "Skip"},
				},
				Args: []ast.Expr{
					&ast.BasicLit{
						Kind:  token.STRING,
						Value: `"Flaky test quarantined. Done automatically by branch-out (https://github.com/smartcontractkit/branch-out)"`,
					},
				},
			},
		}

		// Insert t.Skip() at the beginning of the function body
		if testFunc.Body != nil && len(testFunc.Body.List) > 0 {
			// Insert at the beginning
			testFunc.Body.List = append([]ast.Stmt{skipCall}, testFunc.Body.List...)
		} else if testFunc.Body != nil {
			// Empty function body
			testFunc.Body.List = []ast.Stmt{skipCall}
		}
	}

	var modifiedNode bytes.Buffer
	// Convert the modified AST back to source code
	if err := format.Node(&modifiedNode, fset, node); err != nil {
		return "", fmt.Errorf("failed to format modified source: %w", err)
	}

	return modifiedNode.String(), nil
}

// quarantineTestsComment adds a comment to the test function to indicate that it was quarantined, but does not modify execution.
func quarantineTestsComment(fset *token.FileSet, node *ast.File, testFuncs []*ast.FuncDecl) (string, error) {
	for _, testFunc := range testFuncs {
		// Create a comment for quarantine
		quarantineComment := &ast.Comment{
			Text: "// This test has been identified as flaky and quarantined in CI\n// Done automatically by branch-out (https://github.com/smartcontractkit/branch-out)",
		}

		// Add to existing doc comments or create new ones
		if testFunc.Doc != nil {
			// Prepend to existing comments
			testFunc.Doc.List = append([]*ast.Comment{quarantineComment}, testFunc.Doc.List...)
		} else {
			// Create new doc comments
			testFunc.Doc = &ast.CommentGroup{
				List: []*ast.Comment{quarantineComment},
			}
		}
	}

	var modifiedNode bytes.Buffer
	// Convert the modified AST back to source code
	if err := format.Node(&modifiedNode, fset, node); err != nil {
		return "", fmt.Errorf("failed to format modified source: %w", err)
	}

	return modifiedNode.String(), nil
}
