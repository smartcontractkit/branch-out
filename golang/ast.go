// Package golang provides utilities for the Go programming language.
package golang

import (
	"go/ast"
	"slices"
	"strings"
)

// TestTarget describes a package and a list of test functions to target for operations.
type TestTarget struct {
	Package string   // Import path of the Go package
	Tests   []string // Names of the test functions in the package to target
}

// testsInFile searches for all test functions in the Go test file's AST that match the given test names.
func testsInFile(node *ast.File, testNames []string) []*ast.FuncDecl {
	if node == nil || len(testNames) == 0 {
		return nil
	}

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
	if funcDecl == nil || funcDecl.Name == nil || funcDecl.Type == nil {
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
				if ident.Name == "testing" {
					// Test functions should have *testing.T parameter
					if strings.HasPrefix(funcDecl.Name.Name, "Test") {
						return selectorExpr.Sel.Name == "T"
					}
					// Fuzz functions should have *testing.F parameter
					if strings.HasPrefix(funcDecl.Name.Name, "Fuzz") {
						return selectorExpr.Sel.Name == "F"
					}
				}
			}
		}
	}

	return false
}

// sanitizeTestTargets condenses the test targets and removes duplicates.
// This function works for both quarantine and unquarantine operations.
func sanitizeTestTargets(testTargets []TestTarget) []TestTarget {
	// Package -> tests
	seen := make(map[string][]string)
	for _, target := range testTargets {
		if _, ok := seen[target.Package]; !ok {
			seen[target.Package] = make([]string, 0, len(target.Tests))
		}
		for _, test := range target.Tests {
			if !slices.Contains(seen[target.Package], test) {
				seen[target.Package] = append(seen[target.Package], test)
			}
		}
	}

	sanitizedTargets := make([]TestTarget, 0, len(seen))
	for packageName, tests := range seen {
		sanitizedTargets = append(sanitizedTargets, TestTarget{
			Package: packageName,
			Tests:   tests,
		})
	}
	return sanitizedTargets
}
