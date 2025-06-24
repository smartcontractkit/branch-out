// Package golang provides utilities for the Go programming language.
package golang

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"strings"
)

// QuarantineTest modifies Go source code to quarantine a test by adding t.Skip() at the beginning.
// It takes the source code as a string and the test function name to quarantine.
func QuarantineTest(sourceCode, testFunctionName string) (string, error) {
	if testFunctionName == "" {
		return sourceCode, fmt.Errorf("test function name cannot be empty")
	}

	// Parse the source code
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "", sourceCode, parser.ParseComments)
	if err != nil {
		return sourceCode, fmt.Errorf("failed to parse Go source code: %w", err)
	}

	// Find and modify the target test function
	testFound := false
	ast.Inspect(node, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok {
			// Check if this is the target test function
			if fn.Name.Name == testFunctionName && isTestFunction(fn) {
				testFound = true
				addSkipToFunction(fn)
			}
		}
		return true
	})

	if !testFound {
		return sourceCode, fmt.Errorf("test function '%s' not found", testFunctionName)
	}

	// Convert the modified AST back to source code
	var buf strings.Builder
	if err := format.Node(&buf, fset, node); err != nil {
		return sourceCode, fmt.Errorf("failed to format modified source code: %w", err)
	}

	return buf.String(), nil
}

// isTestFunction checks if a function declaration is a test function
func isTestFunction(fn *ast.FuncDecl) bool {
	// Test functions must:
	// 1. Start with "Test"
	// 2. Have exactly one parameter of type *testing.T
	if !strings.HasPrefix(fn.Name.Name, "Test") {
		return false
	}

	if fn.Type.Params == nil || len(fn.Type.Params.List) != 1 {
		return false
	}

	param := fn.Type.Params.List[0]
	if starExpr, ok := param.Type.(*ast.StarExpr); ok {
		if selectorExpr, ok := starExpr.X.(*ast.SelectorExpr); ok {
			if ident, ok := selectorExpr.X.(*ast.Ident); ok {
				return ident.Name == "testing" && selectorExpr.Sel.Name == "T"
			}
		}
	}

	return false
}

// addSkipToFunction adds t.Skip() call at the beginning of the function body
func addSkipToFunction(fn *ast.FuncDecl) {
	if fn.Body == nil || len(fn.Body.List) == 0 {
		// If function has no body, create one with just t.Skip()
		fn.Body = &ast.BlockStmt{
			List: []ast.Stmt{createSkipStatement()},
		}
		return
	}

	// Check if t.Skip() is already present at the beginning
	if hasSkipAtBeginning(fn.Body.List) {
		return // Already quarantined
	}

	// Add t.Skip() at the beginning of the function
	skipStmt := createSkipStatement()
	fn.Body.List = append([]ast.Stmt{skipStmt}, fn.Body.List...)
}

// hasSkipAtBeginning checks if the first statement is already a t.Skip() call
func hasSkipAtBeginning(stmts []ast.Stmt) bool {
	if len(stmts) == 0 {
		return false
	}

	if exprStmt, ok := stmts[0].(*ast.ExprStmt); ok {
		if callExpr, ok := exprStmt.X.(*ast.CallExpr); ok {
			if selectorExpr, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
				if ident, ok := selectorExpr.X.(*ast.Ident); ok {
					return ident.Name == "t" && selectorExpr.Sel.Name == "Skip"
				}
			}
		}
	}

	return false
}

// createSkipStatement creates an AST node for t.Skip("quarantined by branch-out")
func createSkipStatement() ast.Stmt {
	return &ast.ExprStmt{
		X: &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   &ast.Ident{Name: "t"},
				Sel: &ast.Ident{Name: "Skip"},
			},
			Args: []ast.Expr{
				&ast.BasicLit{
					Kind:  token.STRING,
					Value: `"quarantined by branch-out"`,
				},
			},
		},
	}
}
