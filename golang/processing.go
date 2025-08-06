package golang

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

// options describes the options for a test operation.
type options struct {
	buildFlags []string
}

// Option is a function that can be used to configure a test operation.
type Option func(*options)

// WithBuildFlags sets the build flags to use when loading packages.
func WithBuildFlags(buildFlags []string) Option {
	return func(o *options) {
		o.buildFlags = buildFlags
	}
}

// testModifier is a function that modifies a list of test functions in a file.
type testModifier func(fset *token.FileSet, node *ast.File, testFuncs []*ast.FuncDecl) (string, []Test, error)

// processTests is a generic function that processes tests based on the provided operation.
func processTests(
	l zerolog.Logger,
	repoPath string,
	targets []TestTarget,
	operation OperationType,
	modifier testModifier,
	opts ...Option,
) (Results, error) {
	options := &options{
		buildFlags: []string{},
	}
	for _, opt := range opts {
		opt(options)
	}

	packages, err := Packages(l, repoPath, options.buildFlags)
	if err != nil {
		return nil, err
	}

	var (
		sanitizedTargets = sanitizeTestTargets(targets)
		testsToProcess   int
	)
	for _, target := range sanitizedTargets {
		testsToProcess += len(target.Tests)
	}

	start := time.Now()
	l.Info().Str("operation", string(operation)).Msg("Processing tests")
	var (
		packageResultsChan = make(chan PackageResults, len(sanitizedTargets))
		eg                 = errgroup.Group{}
	)

	for _, target := range sanitizedTargets {
		pkg, ok := packages.Packages[target.Package]
		if !ok {
			l.Warn().Str("package", target.Package).Msg("Package not found, skipping")
			continue
		}

		eg.Go(func() error {
			results, err := processPackage(l, repoPath, pkg, target.Tests, modifier)
			if err != nil {
				return err
			}
			packageResultsChan <- results
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	close(packageResultsChan)

	var (
		successfullyProcessed = make([]string, 0, testsToProcess)
		failedToProcess       = make([]string, 0, testsToProcess)
		results               = make(Results, len(sanitizedTargets))
	)
	for result := range packageResultsChan {
		results[result.Package] = result
		for _, success := range result.Successes {
			for _, test := range success.Tests {
				successfullyProcessed = append(successfullyProcessed, fmt.Sprintf("%s.%s", success.Package, test.Name))
			}
		}
		for _, failure := range result.Failures {
			failedToProcess = append(failedToProcess, fmt.Sprintf("%s.%s", result.Package, failure))
		}
	}

	l.Info().
		Str("operation", string(operation)).
		Strs("successfully_processed", successfullyProcessed).
		Strs("failed_to_process", failedToProcess).
		Str("duration", time.Since(start).String()).
		Msg("Processing results")

	return results, nil
}

// processPackage looks for test functions in all test files in a package and processes them.
func processPackage(
	l zerolog.Logger,
	repoPath string,
	pkg PackageInfo,
	testsToProcess []string,
	modifier testModifier,
) (PackageResults, error) {
	l = l.With().
		Str("package", pkg.ImportPath).
		Strs("tests_to_process", testsToProcess).
		Strs("test_files", pkg.TestGoFiles).
		Logger()
	l.Debug().Msg("Processing tests in package")

	var (
		haveProcessed = make(map[string]bool) // Track which tests we've already processed for quick lookup
		results       = PackageResults{
			Package:   pkg.ImportPath,
			Successes: make([]File, 0, len(testsToProcess)),
			Failures:  make([]string, 0, len(testsToProcess)),
		}
	)

	// Look through each test file in the package to see if we can find any of our target tests to process.
	for _, testFile := range pkg.TestGoFiles {
		// Parse the Go file using AST
		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, testFile, nil, parser.ParseComments)
		if err != nil {
			return PackageResults{}, fmt.Errorf("failed to parse file %s: %w", testFile, err)
		}

		// Find all test functions in the file that match our targets
		var testsToModify []*ast.FuncDecl
		for _, test := range testsToProcess {
			testFunc := findTestFunc(node, test)
			if testFunc != nil {
				testsToModify = append(testsToModify, testFunc)
				haveProcessed[test] = true
			}
		}
		if len(testsToModify) == 0 {
			continue
		}

		// Process the tests
		modifiedSource, modifiedTests, err := modifier(fset, node, testsToModify)
		if err != nil {
			return PackageResults{}, fmt.Errorf("failed to process tests in file %s: %w", testFile, err)
		}

		results.Successes = append(results.Successes, File{
			Package:            pkg.ImportPath,
			File:               strings.TrimPrefix(testFile, repoPath+"/"),
			FileAbs:            testFile,
			Tests:              modifiedTests,
			ModifiedSourceCode: modifiedSource,
		})
	}

	// Add any tests that were not found to the failures
	for _, test := range testsToProcess {
		if _, ok := haveProcessed[test]; !ok {
			results.Failures = append(results.Failures, test)
		}
	}

	return results, nil
}

// findTestFunc finds a test function by name in a given AST file.
func findTestFunc(file *ast.File, testName string) *ast.FuncDecl {
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == testName {
			return fn
		}
	}
	return nil
}
