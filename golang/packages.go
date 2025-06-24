package golang

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/tools/go/packages"
)

// Absolute path to root directory -> PackageInfo
var (
	packagesCache      = map[string][]PackageInfo{}
	packagesCacheMutex = sync.RWMutex{}

	// ErrTestNotFound is returned when a test is not found in the go code.
	ErrTestNotFound = errors.New("test not found")
)

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
func Packages(l zerolog.Logger, rootDir string) ([]PackageInfo, error) {
	absRootDir, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, err
	}

	packagesCacheMutex.RLock()
	cachedPackages, ok := packagesCache[absRootDir]
	packagesCacheMutex.RUnlock()
	if ok {
		return cachedPackages, nil
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

	var result []PackageInfo
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

		result = append(result, info)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ImportPath < result[j].ImportPath
	})

	packagesCacheMutex.Lock()
	packagesCache[absRootDir] = result
	packagesCacheMutex.Unlock()

	for _, pkg := range result {
		l.Trace().
			Strs("files", pkg.GoFiles).
			Strs("testFiles", pkg.TestGoFiles).
			Strs("xTestFiles", pkg.XTestGoFiles).
			Str("name", pkg.Name).
			Str("importPath", pkg.ImportPath).
			Str("module", pkg.Module).
			Bool("isCommand", pkg.IsCommand).
			Str("pkgDir", pkg.Dir).
			Msg("Loaded package")
	}

	l.Trace().Str("duration", time.Since(start).String()).Msg("Loaded packages")
	return result, nil
}
