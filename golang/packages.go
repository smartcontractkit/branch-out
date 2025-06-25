package golang

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/tools/go/packages"
)

// Absolute path to root directory -> PackageInfo
var (
	// ErrTestNotFound is returned when a test is not found in the go code.
	ErrTestNotFound = errors.New("test not found")
	// ErrPackageNotFound is returned when a package is not found in the go code.
	ErrPackageNotFound = errors.New("package not found")
)

// PackagesInfo contains all the packages found in a Go project.
type PackagesInfo struct {
	Packages map[string]PackageInfo
}

// Get returns the PackageInfo for the given import path.
// Note that the import path is the full import path, not just the package name.
func (p *PackagesInfo) Get(importPath string) (PackageInfo, error) {
	if pkg, ok := p.Packages[importPath]; ok {
		return pkg, nil
	}

	return PackageInfo{}, fmt.Errorf("%w: %s", ErrPackageNotFound, importPath)
}

// PackageInfo contains comprehensive information about a Go package
type PackageInfo struct {
	ImportPath   string   // Package import path (e.g., "github.com/user/repo/pkg")
	Name         string   // Package name
	Dir          string   // Directory containing the package
	GoFiles      []string // .go source files
	TestGoFiles  []string // _test.go files
	XTestGoFiles []string // _test.go files with different package names
	Module       string   // Module path
	IsCommand    bool     // True if this is a main package
}

// Packages finds all Go packages in the given directory and subdirectories
func Packages(l zerolog.Logger, rootDir string) (*PackagesInfo, error) {
	absRootDir, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, err
	}

	l = l.With().Str("rootDir", rootDir).Str("absRootDir", absRootDir).Logger()
	l.Trace().Msg("Loading packages")
	start := time.Now()
	config := &packages.Config{
		Mode:  packages.NeedName | packages.NeedModule | packages.NeedFiles,
		Dir:   rootDir,
		Tests: true,
	}

	// Use "./..." pattern to find all packages recursively
	pkgs, err := packages.Load(config, "./...")
	if err != nil {
		return nil, err
	}

	result := &PackagesInfo{
		Packages: make(map[string]PackageInfo),
	}

	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			l.Error().Err(pkg.Errors[0]).Msg("Error loading package")
			// Skip packages with errors, but continue processing others
			continue
		}

		info := PackageInfo{
			ImportPath: pkg.PkgPath,
			Name:       pkg.Name,
			IsCommand:  pkg.Name == "main",
		}

		if pkg.Module != nil {
			info.Module = pkg.Module.Path
		}

		// Separate regular files from test files
		for _, file := range pkg.GoFiles {
			if strings.HasSuffix(file, "_test.go") {
				info.TestGoFiles = append(info.TestGoFiles, file)
			} else {
				info.GoFiles = append(info.GoFiles, file)
			}
		}

		// Get directory from first file
		if len(pkg.GoFiles) > 0 {
			info.Dir = filepath.Dir(pkg.GoFiles[0])
		}

		result.Packages[info.ImportPath] = info
	}

	for _, pkg := range result.Packages {
		l.Trace().
			Strs("files", pkg.GoFiles).
			Strs("testFiles", pkg.TestGoFiles).
			Strs("xTestFiles", pkg.XTestGoFiles).
			Str("name", pkg.Name).
			Str("importPath", pkg.ImportPath).
			Str("module", pkg.Module).
			Bool("isCommand", pkg.IsCommand).
			Str("pkgDir", pkg.Dir).
			Msg("Found package")
	}

	l.Trace().Int("count", len(result.Packages)).Str("duration", time.Since(start).String()).Msg("Found packages")
	return result, nil
}
