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
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

// QuarantineTarget describes a package and a list of test functions to quarantine.
type QuarantineTarget struct {
	Package string             // Import path of the Go package
	Tests   []TestToQuarantine // Names of the test functions in the package to quarantine
}

// TestNames returns the names of the test functions to quarantine.
func (q QuarantineTarget) TestNames() []string {
	names := make([]string, 0, len(q.Tests))
	for _, test := range q.Tests {
		names = append(names, test.Name)
	}
	return names
}

// JiraTicketForTestName returns the test to quarantine with the given name.
// Returns the empty string if the test is not found.
func (q QuarantineTarget) JiraTicketForTestName(testName string) string {
	for _, test := range q.Tests {
		if test.Name == testName {
			return test.JiraTicket
		}
	}
	return ""
}

// TestToQuarantine describes a test to quarantine and the associated Jira ticket.
type TestToQuarantine struct {
	Name       string // Name of the test function to quarantine, e.g. "TestFoo"
	JiraTicket string // Jira ticket of the test function to quarantine, e.g. "JIRA-123"
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
func (q QuarantineResults) Markdown(owner, repo, branch string) string {
	var md strings.Builder
	md.WriteString("# Quarantined Flaky Tests using branch-out\n\n")

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
				githubBlobURL := fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s", owner, repo, branch, file.File)

				// Create individual test links with line numbers
				var testLinks []string
				for _, test := range file.Tests {
					testLink := fmt.Sprintf("[%s](%s#L%d)", test.Name, githubBlobURL, test.ModifiedLine)
					testLinks = append(testLinks, testLink)
				}

				md.WriteString(
					fmt.Sprintf("| [%s](%s) | %s |\n", file.File, githubBlobURL, strings.Join(testLinks, ", ")),
				)
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
	GoModDir  string            // Directory containing the go.mod file for the package
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
	JiraTicket   string // Jira ticket of the test function that was quarantined
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
			results, err := quarantinePackage(l, repoPath, pkg, target)
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
			if len(success.Tests) == 0 {
				continue
			}

			if err := os.WriteFile(success.FileAbs, []byte(success.ModifiedSourceCode), 0600); err != nil {
				return fmt.Errorf("failed to write quarantine results to %s: %w", success.FileAbs, err)
			}
			l.Trace().
				Str("file", success.FileAbs).
				Str("package", success.Package).
				Strs("quarantined_tests", success.TestNames()).
				Msg("Wrote quarantine results")
		}

		// Tidy the go.mod file to ensure the quarantine package is added to the module's dependencies.
		goTidy := exec.Command("go", "mod", "tidy")
		goTidy.Dir = result.GoModDir
		output, err := goTidy.CombinedOutput()
		if err != nil {
			return fmt.Errorf(
				"failed to tidy go.mod file: %w\nCommand: %s\nDirectory: %s\nOutput:\n%s",
				err,
				goTidy.String(),
				goTidy.Dir,
				string(output),
			)
		}
	}

	return nil
}

// sanitizeQuarantineTargets condenses the quarantine targets and removes duplicates.
func sanitizeQuarantineTargets(quarantineTargets []QuarantineTarget) []QuarantineTarget {
	// Package -> tests
	seen := make(map[string][]TestToQuarantine)
	for _, target := range quarantineTargets {
		if _, ok := seen[target.Package]; !ok {
			seen[target.Package] = make([]TestToQuarantine, 0, len(target.Tests))
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
	quarantineTarget QuarantineTarget,
) (QuarantinePackageResults, error) {
	testNames := quarantineTarget.TestNames()
	l = l.With().
		Str("package", pkg.ImportPath).
		Strs("test_files", pkg.TestGoFiles).
		Strs("tests_to_quarantine", testNames).
		Logger()
	l.Debug().Msg("Quarantining tests in package")

	var (
		haveQuarantined = make(map[string]bool) // Track which tests we've already quarantined for quick lookup
		results         = QuarantinePackageResults{
			Package:   pkg.ImportPath,
			GoModDir:  pkg.Module.Dir,
			Successes: make([]QuarantinedFile, 0, len(testNames)),
			Failures:  make([]string, 0, len(testNames)),
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

		foundTests := testsInFile(node, quarantineTarget)
		foundTestNames := make([]string, 0, len(foundTests))
		for _, test := range foundTests {
			haveQuarantined[test.Name] = true
			foundTestNames = append(foundTestNames, test.Name)
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
	for _, test := range quarantineTarget.Tests {
		if !haveQuarantined[test.Name] {
			results.Failures = append(results.Failures, test.Name)
		}
	}

	return results, nil
}

// foundTest describes a test function that was found in a file and matches a test to quarantine.
type foundTest struct {
	FuncDecl *ast.FuncDecl
	TestToQuarantine
}

// testsInFile searches for all test functions in the Go test file's AST that match the given test names.
func testsInFile(node *ast.File, quarantineTarget QuarantineTarget) []foundTest {
	testNames := quarantineTarget.TestNames()
	found := make([]foundTest, 0, len(testNames))
	for _, decl := range node.Decls {
		// Check if this declaration is a function
		if funcDecl, ok := decl.(*ast.FuncDecl); ok {
			// Check if it's a test function with the right name
			if isTestFunction(funcDecl) && slices.Contains(testNames, funcDecl.Name.Name) {
				found = append(found, foundTest{
					FuncDecl: funcDecl,
					TestToQuarantine: TestToQuarantine{
						Name:       funcDecl.Name.Name,
						JiraTicket: quarantineTarget.JiraTicketForTestName(funcDecl.Name.Name),
					},
				})
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

// skipTests adds conditional quarantine logic to the beginning of the test function using quarantine.Flaky().
func skipTests(
	fset *token.FileSet,
	fileRootNode *ast.File,
	testsToSkip []foundTest,
) (string, []QuarantinedTest, error) {
	// Ensure quarantine package is imported for the conditional logic
	if len(testsToSkip) > 0 && !hasImport(fileRootNode, "github.com/smartcontractkit/branch-out/quarantine") {
		addImport(fileRootNode, "github.com/smartcontractkit/branch-out/quarantine")
	}

	// Store original line numbers and test names
	originalPositions := make(map[string]int)
	for _, testToSkip := range testsToSkip {
		originalPositions[testToSkip.Name] = fset.Position(testToSkip.FuncDecl.Pos()).Line
	}

	quarantinedTests := make([]QuarantinedTest, 0, len(testsToSkip))
	// Apply modifications
	for _, testToSkip := range testsToSkip {
		paramName := "t" // default fallback for testing.T param name
		funcDecl := testToSkip.FuncDecl
		if funcDecl.Type.Params != nil && len(funcDecl.Type.Params.List) > 0 {
			param := funcDecl.Type.Params.List[0]
			if len(param.Names) > 0 && param.Names[0] != nil {
				paramName = param.Names[0].Name
			}
		}

		quarantineCall := &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   &ast.Ident{Name: "quarantine"},
				Sel: &ast.Ident{Name: "Flaky"},
			},
			Args: []ast.Expr{
				&ast.BasicLit{Kind: token.STRING, Value: paramName},
				&ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf(`"%s"`, testToSkip.JiraTicket)},
			},
		}

		funcDecl.Body.List = append(
			[]ast.Stmt{&ast.ExprStmt{X: quarantineCall}},
			funcDecl.Body.List...,
		)
		quarantinedTests = append(quarantinedTests, QuarantinedTest{
			Name:         testToSkip.Name,
			JiraTicket:   testToSkip.JiraTicket,
			OriginalLine: originalPositions[testToSkip.Name],
		})
	}

	// Format the modified AST
	var modifiedNode bytes.Buffer
	if err := format.Node(&modifiedNode, fset, fileRootNode); err != nil {
		return "", nil, fmt.Errorf("failed to format modified source: %w", err)
	}

	// Parse the formatted result to get exact line numbers
	modifiedSource := modifiedNode.String()
	newFset := token.NewFileSet()
	newNode, err := parser.ParseFile(newFset, "", modifiedSource, parser.ParseComments)
	if err != nil {
		return "", nil, fmt.Errorf("failed to parse modified source: %w", err)
	}

	// Find the modified line numbers and update the quarantinedTests slice
	for _, decl := range newNode.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok {
			if isTestFunction(funcDecl) {
				for index, quarantinedTest := range quarantinedTests {
					if quarantinedTest.Name == funcDecl.Name.Name {
						quarantinedTests[index].ModifiedLine = newFset.Position(funcDecl.Pos()).Line
					}
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
func addImport(fileRootNode *ast.File, importPath string) {
	// Create the import spec
	importSpec := &ast.ImportSpec{
		Path: &ast.BasicLit{
			Kind:  token.STRING,
			Value: `"` + importPath + `"`,
		},
	}

	// If there are no imports yet, create a new import declaration
	if len(fileRootNode.Decls) == 0 || fileRootNode.Imports == nil {
		importDecl := &ast.GenDecl{
			Tok:   token.IMPORT,
			Specs: []ast.Spec{importSpec},
		}
		fileRootNode.Decls = append([]ast.Decl{importDecl}, fileRootNode.Decls...)
		fileRootNode.Imports = []*ast.ImportSpec{importSpec}
		return
	}

	// Find the first import declaration and add to it
	for i, decl := range fileRootNode.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.IMPORT {
			genDecl.Specs = append(genDecl.Specs, importSpec)
			fileRootNode.Imports = append(fileRootNode.Imports, importSpec)
			return
		}
		// If we hit a non-import declaration, insert a new import declaration before it
		if i == 0 || (i == 1 && isPackageDeclaration(fileRootNode.Decls[0])) {
			importDecl := &ast.GenDecl{
				Tok:   token.IMPORT,
				Specs: []ast.Spec{importSpec},
			}
			fileRootNode.Decls = append(
				fileRootNode.Decls[:i],
				append([]ast.Decl{importDecl}, fileRootNode.Decls[i:]...)...)
			fileRootNode.Imports = append(fileRootNode.Imports, importSpec)
			return
		}
	}

	// Fallback: add at the beginning
	importDecl := &ast.GenDecl{
		Tok:   token.IMPORT,
		Specs: []ast.Spec{importSpec},
	}
	fileRootNode.Decls = append([]ast.Decl{importDecl}, fileRootNode.Decls...)
	fileRootNode.Imports = append(fileRootNode.Imports, importSpec)
}

// isPackageDeclaration checks if a declaration is a package declaration
func isPackageDeclaration(decl ast.Decl) bool {
	_, ok := decl.(*ast.GenDecl)
	return !ok // Package declarations are not GenDecl
}
