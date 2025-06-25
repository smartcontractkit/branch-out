// Package golang provides utilities for the Go programming language.
package golang

import (
	"fmt"

	"github.com/rs/zerolog"
)

type QuarantineTarget struct {
	PackageName string // Name of the Go package
	TestName    string // Name of the test function to quarantine
}

type QuarantineResult struct {
	PackageName        string
	TestName           string
	Quarantined        bool
	Error              error
	ModifiedSourceCode string
}

// QuarantineTests looks through a Go project to find and quarantine any tests that match the given targets.
// It returns a list of results for each target, including whether it was able to be quarantined, the modified source code, to quarantine the test, and any errors that occurred.
// The modified source code is returned so that it can be committed to the repository. You must do something with it, as the code is not edited or committed by this function.
func QuarantineTests(
	l zerolog.Logger,
	repoPath string,
	quarantineTargets []QuarantineTarget,
) ([]QuarantineResult, error) {
	// TODO: Implement pseudo-code
	// Loop through quarantine targets
	// For each target, get the source code
	// Parse the source code
	// Find and modify the target test function
	// Add t.Skip() at the beginning of the function
	// If you can't, mark it as unable to quarantine
	// Return report of which tests were quarantined and which were not

	packages, err := Packages(l, repoPath)
	if err != nil {
		return nil, err
	}

	results := make([]QuarantineResult, len(quarantineTargets))
	for i, target := range quarantineTargets {
		pkg, err := packages.Get(target.PackageName)
		if err != nil {
			return nil, err
		}

		results[i], err = quarantineTest(l, repoPath, pkg, target)
		if err != nil {
			return nil, err
		}
	}

	return nil, nil
}

func quarantineTest(
	l zerolog.Logger,
	repoPath string,
	pkg PackageInfo,
	target QuarantineTarget,
) (QuarantineResult, error) {
	return QuarantineResult{}, fmt.Errorf("not implemented")
}
