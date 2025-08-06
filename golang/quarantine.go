// Package golang provides utilities for analyzing and modifying Go source code.
package golang

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"

	"github.com/rs/zerolog"
	"golang.org/x/tools/go/ast/astutil"
)

// RunQuarantinedTestsEnvVar is the environment variable that controls whether quarantined tests are run.
const RunQuarantinedTestsEnvVar = "RUN_QUARANTINED_TESTS"

// QuarantineTests looks through a Go project to find and quarantine any tests that match the given targets.
// It returns a list of results for each target, including whether it was able to be quarantined, and the modified source code to quarantine the test.
// The modified source code is returned so that it can be committed to the repository.
// You must do something with it, as the code is not edited or committed by this function.
//
// Tests quarantined by this process will use t.Skip() to skip the test, unless the environment variable RUN_QUARANTINED_TESTS is set to "true".
func QuarantineTests(
	l zerolog.Logger,
	repoPath string,
	quarantineTargets []TestTarget,
	opts ...Option,
) (Results, error) {
	return processTests(l, repoPath, quarantineTargets, OperationQuarantine, skipTests, opts...)
}

// WriteQuarantineResultsToFiles writes successfully quarantined tests to the file system.
func WriteQuarantineResultsToFiles(l zerolog.Logger, results Results) error {
	return writeResultsToFiles(l, results, string(OperationQuarantine))
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
) (string, []Test, error) {
	// Ensure "os" package is imported for the conditional logic
	if len(testFuncs) > 0 && !hasImport(node, "os") {
		astutil.AddImport(fset, node, "os")
	}

	// Store original line numbers and test names
	originalPositions := make(map[string]int)
	for _, testFunc := range testFuncs {
		originalPositions[testFunc.Name.Name] = fset.Position(testFunc.Pos()).Line
	}

	// Apply modifications (same as existing implementation)
	for _, testFunc := range testFuncs {
		// Get the test parameter name (usually 't', but could be 'f' for fuzz tests or something else)
		testParamName := "t" // default fallback
		if testFunc.Type.Params != nil && len(testFunc.Type.Params.List) > 0 {
			firstParam := testFunc.Type.Params.List[0]
			if len(firstParam.Names) > 0 {
				testParamName = firstParam.Names[0].Name
			}
		}

		// Prepend the quarantine conditional to the beginning of the function body
		// We are building the AST manually here to ensure correct formatting
		// and to avoid any potential injection issues.
		// The code we are adding is:
		// if os.Getenv("RUN_QUARANTINED_TESTS") != "true" {
		//     <testParam>.Skip("Flaky test quarantined. Ticket <Jira ticket>. Done automatically by branch-out (https://github.com/smartcontractkit/branch-out)")
		// } else {
		//     <testParam>.Logf("'RUN_QUARANTINED_TESTS' set to '%s', running quarantined test", os.Getenv("RUN_QUARANTINED_TESTS"))
		// }
		ifStmt := &ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X: &ast.CallExpr{
					Fun: &ast.SelectorExpr{
						X:   ast.NewIdent("os"),
						Sel: ast.NewIdent("Getenv"),
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
								X:   ast.NewIdent(testParamName),
								Sel: ast.NewIdent("Skip"),
							},
							Args: []ast.Expr{
								&ast.BasicLit{
									Kind:  token.STRING,
									Value: `"Flaky test quarantined. See Jira ticket for details."`,
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
								X:   ast.NewIdent(testParamName),
								Sel: ast.NewIdent("Logf"),
							},
							Args: []ast.Expr{
								&ast.BasicLit{
									Kind:  token.STRING,
									Value: `"'RUN_QUARANTINED_TESTS' set to '%s', running quarantined test"`,
								},
								&ast.CallExpr{
									Fun: &ast.SelectorExpr{
										X:   ast.NewIdent("os"),
										Sel: ast.NewIdent("Getenv"),
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
		testFunc.Body.List = append([]ast.Stmt{ifStmt}, testFunc.Body.List...)
	}

	// Format the modified AST
	var modifiedNode bytes.Buffer
	if err := format.Node(&modifiedNode, fset, node); err != nil {
		return "", nil, fmt.Errorf("failed to format modified AST: %w", err)
	}

	// Parse the formatted result to get exact line numbers
	modifiedSource := modifiedNode.String()
	newFset := token.NewFileSet()
	newNode, err := parser.ParseFile(newFset, "", modifiedSource, parser.ParseComments)
	if err != nil {
		return "", nil, fmt.Errorf("failed to parse modified source: %w", err)
	}

	// Find the modified line numbers
	quarantinedTests := make([]Test, 0, len(testFuncs))
	for _, decl := range newNode.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			if _, ok := originalPositions[fn.Name.Name]; ok {
				quarantinedTests = append(quarantinedTests, Test{
					Name:         fn.Name.Name,
					OriginalLine: originalPositions[fn.Name.Name],
					ModifiedLine: newFset.Position(fn.Pos()).Line,
				})
			}
		}
	}

	return modifiedSource, quarantinedTests, nil
}

// hasImport checks if the given import path is already imported in the file
func hasImport(node *ast.File, importPath string) bool {
	for _, imp := range node.Imports {
		if imp.Path.Value == fmt.Sprintf(`"%s"`, importPath) {
			return true
		}
	}
	return false
}
