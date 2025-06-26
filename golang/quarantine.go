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

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

// QuarantineTarget describes a test function to quarantine.
type QuarantineTarget struct {
	PackageName string // Name of the Go package
	TestName    string // Name of the test function to quarantine
}

// QuarantineResult describes the result of quarantining a test function.
type QuarantineResult struct {
	PackageName        string
	TestName           string
	Quarantined        bool
	File               string
	ModifiedSourceCode string
}

// QuarantineTests looks through a Go project to find and quarantine any tests that match the given targets.
// It returns a list of results for each target, including whether it was able to be quarantined, and the modified source code to quarantine the test.
// The modified source code is returned so that it can be committed to the repository. You must do something with it, as the code is not edited or committed by this function.
func QuarantineTests(
	l zerolog.Logger,
	repoPath string,
	quarantineTargets []QuarantineTarget,
	buildFlags ...string,
) ([]QuarantineResult, error) {
	packages, err := Packages(l, repoPath, buildFlags...)
	if err != nil {
		return nil, err
	}

	var (
		results = make([]QuarantineResult, len(quarantineTargets))
		eg      = errgroup.Group{}
	)

	for i, target := range quarantineTargets {
		eg.Go(func() error {
			var quarantineErr error
			results[i], quarantineErr = quarantineTest(packages, target)
			return quarantineErr
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return results, nil
}

func quarantineTest(
	packages *PackagesInfo,
	target QuarantineTarget,
) (QuarantineResult, error) {
	pkg, err := packages.Get(target.PackageName)
	if err != nil {
		return QuarantineResult{}, err
	}

	// Look through each test file to find the target test function
	for _, testFile := range pkg.TestGoFiles {
		filePath := filepath.Join(pkg.Dir, testFile)

		// Parse the Go file using AST
		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
		if err != nil {
			continue // Skip files that can't be parsed
		}

		// Find the test function using AST traversal
		testFunc := findTestFunctionInAST(node, target.TestName)
		if testFunc == nil {
			continue // Test function not found in this file
		}

		// Add t.Skip() to quarantine the test
		modifiedSource, err := quarantineTestInAST(fset, node, testFunc)
		if err != nil {
			return QuarantineResult{
				PackageName: target.PackageName,
				TestName:    target.TestName,
				Quarantined: false,
				File:        testFile,
			}, err
		}

		return QuarantineResult{
			PackageName:        target.PackageName,
			TestName:           target.TestName,
			Quarantined:        true,
			File:               testFile,
			ModifiedSourceCode: modifiedSource,
		}, nil
	}

	// Test function not found in any file
	return QuarantineResult{
		PackageName: target.PackageName,
		TestName:    target.TestName,
		Quarantined: false,
	}, nil
}

// findTestFunctionInAST searches for a specific test function in the AST
func findTestFunctionInAST(node *ast.File, testName string) *ast.FuncDecl {
	for _, decl := range node.Decls {
		// Check if this declaration is a function
		if funcDecl, ok := decl.(*ast.FuncDecl); ok {
			// Check if it's a test function with the right name
			if isTestFunction(funcDecl) && funcDecl.Name.Name == testName {
				return funcDecl
			}
		}
	}
	return nil
}

// isTestFunction checks if a function declaration is a test function
func isTestFunction(funcDecl *ast.FuncDecl) bool {
	if funcDecl.Name == nil {
		return false
	}

	if !strings.HasPrefix(funcDecl.Name.Name, "Test") {
		return false
	}

	// Check the function signature
	if funcDecl.Type.Params == nil || len(funcDecl.Type.Params.List) != 1 {
		return false
	}

	param := funcDecl.Type.Params.List[0]

	// Check if parameter is *testing.T
	if starExpr, ok := param.Type.(*ast.StarExpr); ok {
		if selectorExpr, ok := starExpr.X.(*ast.SelectorExpr); ok {
			if ident, ok := selectorExpr.X.(*ast.Ident); ok {
				return ident.Name == "testing" && selectorExpr.Sel.Name == "T"
			}
		}
	}

	return false
}

// quarantineTestInAST adds t.Skip() to the beginning of the test function
func quarantineTestInAST(fset *token.FileSet, node *ast.File, testFunc *ast.FuncDecl) (string, error) {
	// Extract the actual parameter name from the function signature
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
					Value: `"Test quarantined by branch-out"`,
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
	} else {
		return "", fmt.Errorf("test function %s has no body", testFunc.Name.Name)
	}

	// Convert the modified AST back to source code
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, node); err != nil {
		return "", fmt.Errorf("failed to format modified source: %w", err)
	}

	return buf.String(), nil
}
