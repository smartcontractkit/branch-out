package golang

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsTestFunction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		code     string
		funcName string
		expected bool
	}{
		{
			name:     "valid Test function with *testing.T",
			code:     `func TestExample(t *testing.T) {}`,
			funcName: "TestExample",
			expected: true,
		},
		{
			name:     "valid Fuzz function with *testing.F",
			code:     `func FuzzExample(f *testing.F) {}`,
			funcName: "FuzzExample",
			expected: true,
		},
		{
			name:     "invalid - regular function",
			code:     `func Example() {}`,
			funcName: "Example",
			expected: false,
		},
		{
			name:     "invalid - Test function without proper signature",
			code:     `func TestExample() {}`,
			funcName: "TestExample",
			expected: false,
		},
		{
			name:     "invalid - Test function with wrong parameter type",
			code:     `func TestExample(t string) {}`,
			funcName: "TestExample",
			expected: false,
		},
		{
			name:     "invalid - Test function with multiple parameters",
			code:     `func TestExample(t *testing.T, extra string) {}`,
			funcName: "TestExample",
			expected: false,
		},
		{
			name:     "invalid - Test function with non-pointer parameter",
			code:     `func TestExample(t testing.T) {}`,
			funcName: "TestExample",
			expected: false,
		},
		{
			name:     "valid - Test function starting with TestSomething",
			code:     `func TestSomethingElse(t *testing.T) {}`,
			funcName: "TestSomethingElse",
			expected: true,
		},
		{
			name:     "invalid - function starting with Test but not testing",
			code:     `func TestHelper(helper *SomeType) {}`,
			funcName: "TestHelper",
			expected: false,
		},
		{
			name:     "invalid - Fuzz function with wrong parameter type",
			code:     `func FuzzExample(t *testing.T) {}`,
			funcName: "FuzzExample",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Parse the function code
			src := `package main
import "testing"
` + tt.code

			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "", src, parser.ParseComments)
			require.NoError(t, err)

			// Find the function declaration
			var funcDecl *ast.FuncDecl
			for _, decl := range file.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == tt.funcName {
					funcDecl = fn
					break
				}
			}
			require.NotNil(t, funcDecl, "Function %s not found", tt.funcName)

			// Test the function
			result := isTestFunction(funcDecl)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsTestFunction_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("nil function declaration", func(t *testing.T) {
		t.Parallel()
		result := isTestFunction(nil)
		assert.False(t, result)
	})

	t.Run("function with nil name", func(t *testing.T) {
		t.Parallel()
		funcDecl := &ast.FuncDecl{
			Name: nil,
		}
		result := isTestFunction(funcDecl)
		assert.False(t, result)
	})

	t.Run("function with nil type", func(t *testing.T) {
		t.Parallel()
		funcDecl := &ast.FuncDecl{
			Name: &ast.Ident{Name: "TestExample"},
			Type: nil,
		}
		result := isTestFunction(funcDecl)
		assert.False(t, result)
	})

	t.Run("function with nil params", func(t *testing.T) {
		t.Parallel()
		funcDecl := &ast.FuncDecl{
			Name: &ast.Ident{Name: "TestExample"},
			Type: &ast.FuncType{
				Params: nil,
			},
		}
		result := isTestFunction(funcDecl)
		assert.False(t, result)
	})
}

func TestTestsInFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		code      string
		testNames []string
		expected  []string // Expected function names found
	}{
		{
			name: "find single test function",
			code: `package main
import "testing"
func TestExample(t *testing.T) {}
func TestOther(t *testing.T) {}
func NotATest() {}`,
			testNames: []string{"TestExample"},
			expected:  []string{"TestExample"},
		},
		{
			name: "find multiple test functions",
			code: `package main
import "testing"
func TestA(t *testing.T) {}
func TestB(t *testing.T) {}
func TestC(t *testing.T) {}
func NotATest() {}`,
			testNames: []string{"TestA", "TestC"},
			expected:  []string{"TestA", "TestC"},
		},
		{
			name: "find fuzz functions",
			code: `package main
import "testing"
func FuzzExample(f *testing.F) {}
func TestExample(t *testing.T) {}`,
			testNames: []string{"FuzzExample", "TestExample"},
			expected:  []string{"FuzzExample", "TestExample"},
		},
		{
			name: "no matches found",
			code: `package main
import "testing"
func TestA(t *testing.T) {}
func TestB(t *testing.T) {}`,
			testNames: []string{"TestC", "TestD"},
			expected:  []string{},
		},
		{
			name: "ignore non-test functions",
			code: `package main
import "testing"
func TestExample(t *testing.T) {}
func TestHelper() {} // Not a valid test function
func Example() {}`,
			testNames: []string{"TestExample", "TestHelper", "Example"},
			expected:  []string{"TestExample"}, // Only valid test functions
		},
		{
			name: "empty test names",
			code: `package main
import "testing"
func TestExample(t *testing.T) {}`,
			testNames: []string{},
			expected:  []string{},
		},
		{
			name: "duplicate test names",
			code: `package main
import "testing"
func TestExample(t *testing.T) {}`,
			testNames: []string{"TestExample", "TestExample"},
			expected:  []string{"TestExample"}, // Should only find once
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Parse the code
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "", tt.code, parser.ParseComments)
			require.NoError(t, err)

			// Call the function
			result := testsInFile(file, tt.testNames)

			// Extract function names from results
			var foundNames []string
			for _, funcDecl := range result {
				foundNames = append(foundNames, funcDecl.Name.Name)
			}

			assert.ElementsMatch(t, tt.expected, foundNames)
		})
	}
}

func TestTestsInFile_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("nil file", func(t *testing.T) {
		t.Parallel()
		result := testsInFile(nil, []string{"TestExample"})
		assert.Empty(t, result)
	})

	t.Run("file with no declarations", func(t *testing.T) {
		t.Parallel()
		file := &ast.File{
			Decls: nil,
		}
		result := testsInFile(file, []string{"TestExample"})
		assert.Empty(t, result)
	})

	t.Run("file with only non-function declarations", func(t *testing.T) {
		t.Parallel()
		code := `package main
var x int
const y = 5`

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "", code, parser.ParseComments)
		require.NoError(t, err)

		result := testsInFile(file, []string{"TestExample"})
		assert.Empty(t, result)
	})

	t.Run("nil test names slice", func(t *testing.T) {
		t.Parallel()
		code := `package main
import "testing"
func TestExample(t *testing.T) {}`

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "", code, parser.ParseComments)
		require.NoError(t, err)

		result := testsInFile(file, nil)
		assert.Empty(t, result)
	})
}
