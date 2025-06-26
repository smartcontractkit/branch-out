package testhelpers

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// CopyDir recursively copies a directory from src to dst.
// Handy for copying files needed for testing into a temp directory.
func CopyDir(tb testing.TB, src, dst string) error {
	tb.Helper()

	err := os.MkdirAll(dst, 0750)
	if err != nil {
		return err
	}

	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() {
			if err := srcFile.Close(); err != nil {
				tb.Logf("Error closing file %s: %v", path, err)
			}
		}()

		dstFile, err := os.Create(dstPath)
		if err != nil {
			return err
		}
		defer func() {
			if err := dstFile.Close(); err != nil {
				tb.Logf("Error closing file %s: %v", dstPath, err)
			}
		}()

		_, err = srcFile.WriteTo(dstFile)
		return err
	})
}

// CopyFile copies a file from src to dst.
// Handy for copying files needed for testing into a temp directory.
func CopyFile(tb testing.TB, src, dst string) error {
	tb.Helper()

	if _, err := os.Stat(src); os.IsNotExist(err) {
		return fmt.Errorf("source file '%s' does not exist", src)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0750); err != nil {
		return fmt.Errorf("failed to create directory '%s' for destination file '%s': %w", filepath.Dir(dst), dst, err)
	}

	destInfo, err := os.Stat(dst)
	if err == nil {
		if destInfo.IsDir() {
			return fmt.Errorf("destination file '%s' is a directory", dst)
		}
		return fmt.Errorf("destination file '%s' already exists", dst)
	}

	srcBytes, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("failed to open source file '%s': %w", src, err)
	}

	if err := os.WriteFile(dst, srcBytes, 0600); err != nil {
		return fmt.Errorf("failed to write file '%s' to '%s': %w", src, dst, err)
	}

	return nil
}

// ShowDirContents prints the full directory structure starting from the given path.
// Handy for debugging test failures in temp directories, like in testscript integration tests.
func ShowDirContents(tb testing.TB, rootPath string) error {
	tb.Helper()

	tb.Logf("Directory structure for: %s", rootPath)

	return filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("error walking %s: %w", path, err)
		}

		// Calculate relative path from root for cleaner display
		relPath, err := filepath.Rel(rootPath, path)
		if err != nil {
			relPath = path // fallback to absolute path
		}

		// Create indentation based on directory depth
		depth := len(
			filepath.SplitList(strings.ReplaceAll(relPath, string(filepath.Separator), string(filepath.ListSeparator))),
		)
		if relPath == "." {
			depth = 0
		}
		indent := strings.Repeat("  ", depth)

		if d.IsDir() {
			tb.Logf("%s[DIR]  %s/", indent, d.Name())
		} else {
			tb.Logf("%s[FILE] %s", indent, d.Name())
		}

		return nil
	})
}
