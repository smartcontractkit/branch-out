// Package golang provides utilities for analyzing and modifying Go source code.
package golang

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"

	"github.com/rs/zerolog"
	"golang.org/x/tools/go/ast/astutil"
)

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
	opts ...Option,
) (Results, error) {
	return processTests(l, repoPath, unquarantineTargets, OperationUnquarantine, unskipTests, opts...)
}

// WriteUnquarantineResultsToFiles writes successfully unquarantined tests to the file system.
func WriteUnquarantineResultsToFiles(l zerolog.Logger, results Results) error {
	return writeResultsToFiles(l, results, string(OperationUnquarantine))
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
) (string, []Test, error) {
	// Store original line numbers and test names
	originalPositions := make(map[string]int)
	for _, testFunc := range testFuncs {
		originalPositions[testFunc.Name.Name] = fset.Position(testFunc.Pos()).Line
	}

	// Remove quarantine logic from each test function
	actuallyUnquarantinedTests := make([]Test, 0, len(testFuncs))
	for _, testFunc := range testFuncs {
		if len(testFunc.Body.List) > 0 {
			if ifStmt, ok := testFunc.Body.List[0].(*ast.IfStmt); ok && isQuarantineConditional(ifStmt) {
				testFunc.Body.List = testFunc.Body.List[1:]
				actuallyUnquarantinedTests = append(actuallyUnquarantinedTests, Test{
					Name:         testFunc.Name.Name,
					OriginalLine: originalPositions[testFunc.Name.Name],
				})
			}
		}
	}

	// Clean up imports: remove "os" import if it's no longer used
	// Note: astutil.DeleteImport will only remove it if it's truly unused
	if len(actuallyUnquarantinedTests) > 0 {
		astutil.DeleteImport(fset, node, "os")
	}

	// Format the modified AST
	var modifiedNode bytes.Buffer
	if err := format.Node(&modifiedNode, fset, node); err != nil {
		return "", nil, fmt.Errorf("failed to format modified AST: %w", err)
	}

	modifiedSource := modifiedNode.String()

	return modifiedSource, actuallyUnquarantinedTests, nil
}

// isQuarantineConditional checks if an if statement is the quarantine conditional block
// by looking for the specific pattern we use for quarantining.
func isQuarantineConditional(ifStmt *ast.IfStmt) bool {
	// Check if the condition is: os.Getenv("RUN_QUARANTINED_TESTS") != "true"
	if binExpr, ok := ifStmt.Cond.(*ast.BinaryExpr); ok && binExpr.Op == token.NEQ {
		if callExpr, ok := binExpr.X.(*ast.CallExpr); ok {
			if selExpr, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
				if x, ok := selExpr.X.(*ast.Ident); ok && x.Name == "os" && selExpr.Sel.Name == "Getenv" {
					if len(callExpr.Args) == 1 {
						if arg, ok := callExpr.Args[0].(*ast.BasicLit); ok &&
							arg.Value == fmt.Sprintf(`"%s"`, RunQuarantinedTestsEnvVar) {
							if lit, ok := binExpr.Y.(*ast.BasicLit); ok && lit.Value == `"true"` {
								return true
							}
						}
					}
				}
			}
		}
	}
	return false
}
