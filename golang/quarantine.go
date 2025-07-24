// Package golang provides utilities for the Go programming language.
package golang

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

// RunQuarantinedTestsEnvVar is the environment variable that controls whether quarantined tests are run.
const RunQuarantinedTestsEnvVar = "RUN_QUARANTINED_TESTS"

// QuarantineTarget describes a package and a list of test functions to quarantine.
type QuarantineTarget struct {
	Package string   // Import path of the Go package
	Tests   []string // Names of the test functions in the package to quarantine
}

// QuarantineResults describes the result of quarantining multiple packages.
// The key is the import path of the package, and the value is the result of quarantining that package.
type QuarantineResults map[string]QuarantinePackageResults

// String returns a string representation of the quarantine results.
// Good for debugging and logging.
func (q QuarantineResults) String() string {
	var b strings.Builder
	for _, result := range q {
		b.WriteString(result.Package)
		b.WriteString("\n")
		b.WriteString("--------------------------------\n")
		if len(result.Successes) > 0 {
			b.WriteString("Successes\n\n")
			for _, success := range result.Successes {
				if len(success.TestNames()) > 0 {
					b.WriteString(fmt.Sprintf("%s: %s\n", success.File, strings.Join(success.TestNames(), ", ")))
				} else {
					b.WriteString(fmt.Sprintf("%s: No tests quarantined\n", success.File))
				}
			}
		} else {
			b.WriteString("\nNo successes!\n")
		}

		if len(result.Failures) > 0 {
			b.WriteString("\nFailures\n\n")
			for _, failure := range result.Failures {
				b.WriteString(fmt.Sprintf("%s\n", failure))
			}
		} else {
			b.WriteString("\nNo failures!\n")
		}
	}
	return b.String()
}

// Markdown returns a Markdown representation of the quarantine results.
// Good for a PR description.
func (q QuarantineResults) Markdown() string {
	var md strings.Builder
	md.WriteString("# Quarantined Flaky Tests using branch-out\n\n")

	allFileUpdates := make(map[string]string)
	for _, result := range q {
		emoji := "🟢"
		if len(result.Failures) > 0 {
			emoji = "🔴"
		}
		md.WriteString(fmt.Sprintf("## `%s` %s\n\n", result.Package, emoji))
		// Process successes
		if len(result.Successes) > 0 {
			md.WriteString(fmt.Sprintf("### Successfully Quarantined %d tests\n\n", result.SuccessfulTestsCount()))
			md.WriteString("| File | Tests |\n")
			md.WriteString("|------|-------|\n")
			for _, file := range result.Successes {
				md.WriteString(
					fmt.Sprintf("| [%s](%s) | %s |\n", file.File, file.File, strings.Join(file.TestNames(), ", ")),
				)
				allFileUpdates[file.File] = file.ModifiedSourceCode
			}
			md.WriteString("\n")
		}

		// Process failures
		if len(result.Failures) > 0 {
			md.WriteString(
				fmt.Sprintf("### Failed to Quarantine %d tests. Need manual intervention!\n\n", len(result.Failures)),
			)
			for _, test := range result.Failures {
				md.WriteString(fmt.Sprintf("- %s\n", test))
			}
			md.WriteString("\n")
		}
	}
	md.WriteString("\n\n---\n\n")
	md.WriteString(
		"Created automatically by [branch-out](https://github.com/smartcontractkit/branch-out).",
	)
	return md.String()
}

// QuarantinePackageResults describes the result of quarantining a list of tests in a package.
type QuarantinePackageResults struct {
	Package   string            // Import path of the Go package (redundant, but kept for handy access)
	Successes []QuarantinedFile // Every file where we found and quarantined tests
	Failures  []string          // Names of the test functions that were not able to be quarantined
}

// SuccessfulTestsCount returns the number of tests that were successfully quarantined.
func (q QuarantinePackageResults) SuccessfulTestsCount() int {
	count := 0
	for _, success := range q.Successes {
		count += len(success.TestNames())
	}
	return count
}

// QuarantinedFile describes the outputs of successfully quarantining a list of tests in a package in a single file.
type QuarantinedFile struct {
	Package            string            // Import path of the Go package (redundant, but kept for handy access)
	File               string            // Relative path to the file where the tests were found and quarantined (if any)
	FileAbs            string            // Absolute path to the file where the tests were found and quarantined on the local filesystem (if any)
	Tests              []QuarantinedTest // All the test functions successfully quarantined in this file
	ModifiedSourceCode string            // Modified source code to quarantine the tests (if any)
}

// TestNames returns the names of the test functions that were quarantined in this file.
func (q QuarantinedFile) TestNames() []string {
	names := make([]string, len(q.Tests))
	for i, test := range q.Tests {
		names[i] = test.Name
	}
	return names
}

// QuarantinedTest describes a test function that was quarantined.
type QuarantinedTest struct {
	Name         string // Name of the test function that was quarantined
	OriginalLine int    // Line number of the test function that was quarantined
	ModifiedLine int    // Line number of the test function that was quarantined after modification of the file
}

// QuarantineOption is a function that can be used to configure the quarantine process.
type QuarantineOption func(*quarantineOptions)

// quarantineOptions describes the options for the quarantine process.
type quarantineOptions struct {
	buildFlags []string
}

// WithBuildFlags sets the build flags to use when loading packages.
func WithBuildFlags(buildFlags []string) QuarantineOption {
	return func(options *quarantineOptions) {
		options.buildFlags = buildFlags
	}
}

// QuarantineTests looks through a Go project to find and quarantine any tests that match the given targets.
// It returns a list of results for each target, including whether it was able to be quarantined, and the modified source code to quarantine the test.
// The modified source code is returned so that it can be committed to the repository.
// You must do something with it, as the code is not edited or committed by this function.
//
// Tests quarantined by this process will use t.Skip() to skip the test, unless the environment variable RUN_QUARANTINED_TESTS is set to "true".
func QuarantineTests(
	l zerolog.Logger,
	repoPath string,
	quarantineTargets []QuarantineTarget,
	options ...QuarantineOption,
) (QuarantineResults, error) {
	quarantineOptions := &quarantineOptions{
		buildFlags: []string{},
	}
	for _, option := range options {
		option(quarantineOptions)
	}

	packages, err := Packages(l, repoPath, quarantineOptions.buildFlags)
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
	l.Info().Msg("Quarantining tests")
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
			results, err := quarantinePackage(l, repoPath, pkg, target.Tests)
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
			for _, test := range success.TestNames() {
				successfullyQuarantined = append(successfullyQuarantined, fmt.Sprintf("%s.%s", success.Package, test))
			}
		}
		for _, failure := range result.Failures {
			failedToQuarantine = append(failedToQuarantine, fmt.Sprintf("%s/%s", result.Package, failure))
		}
	}

	l.Info().
		Strs("successfully_quarantined", successfullyQuarantined).
		Strs("failed_to_quarantine", failedToQuarantine).
		Str("duration", time.Since(start).String()).
		Msg("Quarantine results")

	return results, nil
}

// WriteQuarantineResultsToFiles writes successfully quarantined tests to the file system.
func WriteQuarantineResultsToFiles(l zerolog.Logger, results QuarantineResults) error {
	for _, result := range results {
		for _, success := range result.Successes {
			if err := os.WriteFile(success.FileAbs, []byte(success.ModifiedSourceCode), 0600); err != nil {
				return fmt.Errorf("failed to write quarantine results to %s: %w", success.FileAbs, err)
			}
			l.Trace().
				Str("file", success.FileAbs).
				Str("package", success.Package).
				Strs("quarantined_tests", success.TestNames()).
				Msg("Wrote quarantine results")
		}
	}
	return nil
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
	repoPath string,
	pkg PackageInfo,
	testsToQuarantine []string,
) (QuarantinePackageResults, error) {
	l = l.With().
		Str("package", pkg.ImportPath).
		Strs("tests_to_quarantine", testsToQuarantine).
		Strs("test_files", pkg.TestGoFiles).
		Logger()
	l.Debug().Msg("Quarantining tests in package")

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
		l := l.With().Str("test_file", testFile).Logger()

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

		modifiedSource, quarantinedTests, err := skipTests(fset, node, foundTests)
		if err != nil {
			return results, fmt.Errorf("failed to quarantine tests in file %s: %w", testFile, err)
		}

		l.Debug().Strs("newly_quarantined_tests", foundTestNames).Msg("Successfully quarantined tests in file")

		absRepoPath, err := filepath.Abs(repoPath)
		if err != nil {
			return results, fmt.Errorf("failed to get absolute path of repo %s: %w", repoPath, err)
		}
		relativeFilePath := strings.TrimPrefix(testFile, absRepoPath)
		relativeFilePath = strings.TrimPrefix(relativeFilePath, string(filepath.Separator))
		results.Successes = append(results.Successes, QuarantinedFile{
			Package:            pkg.ImportPath,
			File:               relativeFilePath,
			FileAbs:            testFile,
			Tests:              quarantinedTests,
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

// skipTests adds conditional quarantine logic to the beginning of the test function
//
//	if os.Getenv("RUN_QUARANTINED_TESTS") != "true" {
//	    t.Skip("Flaky test quarantined. Ticket <Jira ticket>. Done automatically by branch-out (https://github.com/smartcontractkit/branch-out)")
//	} else {
//	    t.Logf("'RUN_QUARANTINED_TESTS' set to '%s', running quarantined test", os.Getenv("RUN_QUARANTINED_TESTS"))
//	}
func skipTests(
	fset *token.FileSet,
	node *ast.File,
	testFuncs []*ast.FuncDecl,
) (string, []QuarantinedTest, error) {
	// Ensure "os" package is imported for the conditional logic
	if len(testFuncs) > 0 && !hasImport(node, "os") {
		addImport(node, "os")
	}

	// Store original line numbers and test names
	originalPositions := make(map[string]int)
	for _, testFunc := range testFuncs {
		originalPositions[testFunc.Name.Name] = fset.Position(testFunc.Pos()).Line
	}

	// Apply modifications (same as existing implementation)
	for _, testFunc := range testFuncs {
		paramName := "t" // default fallback
		if testFunc.Type.Params != nil && len(testFunc.Type.Params.List) > 0 {
			param := testFunc.Type.Params.List[0]
			if len(param.Names) > 0 && param.Names[0] != nil {
				paramName = param.Names[0].Name
			}
		}

		conditionalStmt := &ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X: &ast.CallExpr{
					Fun: &ast.SelectorExpr{
						X:   &ast.Ident{Name: "os"},
						Sel: &ast.Ident{Name: "Getenv"},
					},
					Args: []ast.Expr{
						&ast.BasicLit{
							Kind:  token.STRING,
							Value: fmt.Sprintf(`"%s"`, RunQuarantinedTestsEnvVar),
						},
					},
				},
				Op: token.NEQ,
				Y: &ast.BasicLit{
					Kind:  token.STRING,
					Value: `"true"`,
				},
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{
					&ast.ExprStmt{
						X: &ast.CallExpr{
							Fun: &ast.SelectorExpr{
								X:   &ast.Ident{Name: paramName},
								Sel: &ast.Ident{Name: "Skip"},
							},
							Args: []ast.Expr{
								&ast.BasicLit{
									Kind:  token.STRING,
									Value: `"Flaky test quarantined. Ticket <Jira ticket>. Done automatically by branch-out (https://github.com/smartcontractkit/branch-out)"`,
								},
							},
						},
					},
				},
			},
			Else: &ast.BlockStmt{
				List: []ast.Stmt{
					&ast.ExprStmt{
						X: &ast.CallExpr{
							Fun: &ast.SelectorExpr{
								X:   &ast.Ident{Name: paramName},
								Sel: &ast.Ident{Name: "Logf"},
							},
							Args: []ast.Expr{
								&ast.BasicLit{
									Kind:  token.STRING,
									Value: `"'` + RunQuarantinedTestsEnvVar + `' set to '%s', running quarantined test"`,
								},
								&ast.CallExpr{
									Fun: &ast.SelectorExpr{
										X:   &ast.Ident{Name: "os"},
										Sel: &ast.Ident{Name: "Getenv"},
									},
									Args: []ast.Expr{
										&ast.BasicLit{
											Kind:  token.STRING,
											Value: fmt.Sprintf(`"%s"`, RunQuarantinedTestsEnvVar),
										},
									},
								},
							},
						},
					},
				},
			},
		}

		if testFunc.Body != nil && len(testFunc.Body.List) > 0 {
			testFunc.Body.List = append([]ast.Stmt{conditionalStmt}, testFunc.Body.List...)
		} else if testFunc.Body != nil {
			testFunc.Body.List = []ast.Stmt{conditionalStmt}
		}
	}

	// Format the modified AST
	var modifiedNode bytes.Buffer
	if err := format.Node(&modifiedNode, fset, node); err != nil {
		return "", nil, fmt.Errorf("failed to format modified source: %w", err)
	}

	// Parse the formatted result to get exact line numbers
	modifiedSource := modifiedNode.String()
	newFset := token.NewFileSet()
	newNode, err := parser.ParseFile(newFset, "", modifiedSource, parser.ParseComments)
	if err != nil {
		return "", nil, fmt.Errorf("failed to parse modified source: %w", err)
	}

	// Find the modified line numbers
	quarantinedTests := make([]QuarantinedTest, 0, len(testFuncs))
	for _, decl := range newNode.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok {
			if isTestFunction(funcDecl) {
				if originalLine, exists := originalPositions[funcDecl.Name.Name]; exists {
					quarantinedTests = append(quarantinedTests, QuarantinedTest{
						Name:         funcDecl.Name.Name,
						OriginalLine: originalLine,
						ModifiedLine: newFset.Position(funcDecl.Pos()).Line,
					})
				}
			}
		}
	}

	return modifiedSource, quarantinedTests, nil
}

// hasImport checks if the given import path is already imported in the file
func hasImport(node *ast.File, importPath string) bool {
	for _, imp := range node.Imports {
		if imp.Path != nil && strings.Trim(imp.Path.Value, `"`) == importPath {
			return true
		}
	}
	return false
}

// addImport adds an import to the file's import list
func addImport(node *ast.File, importPath string) {
	// Create the import spec
	importSpec := &ast.ImportSpec{
		Path: &ast.BasicLit{
			Kind:  token.STRING,
			Value: `"` + importPath + `"`,
		},
	}

	// If there are no imports yet, create a new import declaration
	if len(node.Decls) == 0 || node.Imports == nil {
		importDecl := &ast.GenDecl{
			Tok:   token.IMPORT,
			Specs: []ast.Spec{importSpec},
		}
		node.Decls = append([]ast.Decl{importDecl}, node.Decls...)
		node.Imports = []*ast.ImportSpec{importSpec}
		return
	}

	// Find the first import declaration and add to it
	for i, decl := range node.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.IMPORT {
			genDecl.Specs = append(genDecl.Specs, importSpec)
			node.Imports = append(node.Imports, importSpec)
			return
		}
		// If we hit a non-import declaration, insert a new import declaration before it
		if i == 0 || (i == 1 && isPackageDeclaration(node.Decls[0])) {
			importDecl := &ast.GenDecl{
				Tok:   token.IMPORT,
				Specs: []ast.Spec{importSpec},
			}
			node.Decls = append(node.Decls[:i], append([]ast.Decl{importDecl}, node.Decls[i:]...)...)
			node.Imports = append(node.Imports, importSpec)
			return
		}
	}

	// Fallback: add at the beginning
	importDecl := &ast.GenDecl{
		Tok:   token.IMPORT,
		Specs: []ast.Spec{importSpec},
	}
	node.Decls = append([]ast.Decl{importDecl}, node.Decls...)
	node.Imports = append(node.Imports, importSpec)
}

// isPackageDeclaration checks if a declaration is a package declaration
func isPackageDeclaration(decl ast.Decl) bool {
	_, ok := decl.(*ast.GenDecl)
	return !ok // Package declarations are not GenDecl
}
