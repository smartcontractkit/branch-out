package golang

import (
	"errors"
	"fmt"
	"maps"
	"os"
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

func (p *PackagesInfo) String() string {
	allPackages := make([]string, 0, len(p.Packages)+1)
	allPackages = append(allPackages, "All packages:")
	for _, pkg := range p.Packages {
		allPackages = append(allPackages, pkg.String())
	}

	return strings.Join(allPackages, "\n\n")
}

// Get returns the PackageInfo for the given import path.
// Note that the import path is the full import path, not just the package name.
func (p *PackagesInfo) Get(importPath string) (PackageInfo, error) {
	if pkg, ok := p.Packages[importPath]; ok {
		return pkg, nil
	}

	allPackages := make([]string, 0, len(p.Packages))
	for pkg := range p.Packages {
		allPackages = append(allPackages, pkg)
	}

	return PackageInfo{}, fmt.Errorf(
		"%w: %s\nall packages:\n%s",
		ErrPackageNotFound,
		importPath,
		strings.Join(allPackages, "\n"),
	)
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

func (p *PackageInfo) String() string {
	str := strings.Builder{}
	str.WriteString(p.ImportPath)
	str.WriteString("\n")
	str.WriteString(fmt.Sprintf("Name: %s", p.Name))
	str.WriteString("\n")
	str.WriteString(fmt.Sprintf("Dir: %s", p.Dir))
	str.WriteString("\n")
	if len(p.GoFiles) > 0 {
		str.WriteString(fmt.Sprintf("GoFiles: %v", p.GoFiles))
		str.WriteString("\n")
	}
	if len(p.TestGoFiles) > 0 {
		str.WriteString(fmt.Sprintf("TestGoFiles: %v", p.TestGoFiles))
		str.WriteString("\n")
	}
	if len(p.XTestGoFiles) > 0 {
		str.WriteString(fmt.Sprintf("XTestGoFiles: %v", p.XTestGoFiles))
	}
	str.WriteString(fmt.Sprintf("Module: %s", p.Module))
	str.WriteString("\n")
	str.WriteString(fmt.Sprintf("IsCommand: %t", p.IsCommand))

	return str.String()
}

// Packages finds all Go packages in the given Go project directory and subdirectories.
// This includes packages of nested Go projects by discovering all go.mod files and
// loading packages from each module root.
// buildFlags are passed to the go command when loading packages, e.g. []string{"-tags", "build_tag"}
func Packages(l zerolog.Logger, rootDir string, buildFlags []string) (*PackagesInfo, error) {
	absRootDir, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, err
	}

	l = l.With().Str("rootDir", rootDir).Str("absRootDir", absRootDir).Logger()
	l.Trace().Msg("Loading packages")
	start := time.Now()

	// Find all go.mod files to identify all Go modules in the directory tree
	goModDirs, err := findGoModDirectories(absRootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to find go.mod files: %w", err)
	}

	if len(goModDirs) == 0 {
		l.Warn().Msg("No go.mod files found in directory tree")
		return &PackagesInfo{Packages: make(map[string]PackageInfo)}, nil
	}

	l.Trace().Strs("goModDirs", goModDirs).Msg("Found Go module directories")

	result := &PackagesInfo{
		Packages: make(map[string]PackageInfo),
	}

	// Load packages from each Go module
	for _, modDir := range goModDirs {
		modulePackages, err := loadPackagesFromModule(modDir, buildFlags)
		if err != nil {
			return nil, fmt.Errorf("failed to load packages from module %s: %w", modDir, err)
		}

		// Merge packages from this module into the result
		maps.Copy(result.Packages, modulePackages)
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

// findGoModDirectories recursively finds all directories containing go.mod files
func findGoModDirectories(rootDir string) ([]string, error) {
	var goModDirs []string

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories and vendor directories
		if info.IsDir() && (strings.HasPrefix(info.Name(), ".") || info.Name() == "vendor") {
			return filepath.SkipDir
		}

		if info.Name() == "go.mod" {
			goModDirs = append(goModDirs, filepath.Dir(path))
		}

		return nil
	})

	return goModDirs, err
}

// loadPackagesFromModule loads all packages from a single Go module
func loadPackagesFromModule(moduleDir string, buildFlags []string) (map[string]PackageInfo, error) {
	config := &packages.Config{
		Mode:       packages.NeedName | packages.NeedModule | packages.NeedFiles,
		Dir:        moduleDir,
		Tests:      true,
		BuildFlags: buildFlags,
	}

	pkgs, err := packages.Load(config, "./...")
	if err != nil {
		return nil, fmt.Errorf("failed to load packages from module %s: %w", moduleDir, err)
	}

	result := make(map[string]PackageInfo)

	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			pkgErrors := make([]string, 0, len(pkg.Errors))
			for _, pkgErr := range pkg.Errors {
				pkgErrors = append(pkgErrors, pkgErr.Msg)
			}
			retErr := fmt.Errorf("error loading package %s\nerrors:\n%s", pkg.PkgPath, strings.Join(pkgErrors, "\n"))
			return nil, retErr
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

		if len(pkg.GoFiles) > 0 {
			info.Dir = filepath.Dir(pkg.GoFiles[0])
		}

		result[info.ImportPath] = info
	}

	return result, nil
}
