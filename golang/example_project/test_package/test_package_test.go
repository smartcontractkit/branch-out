//go:build example_project

package test_package_test

import (
	"testing"

	"github.com/smartcontractkit/branch-out/golang/example_project"
)

func TestTestPackage(t *testing.T) {
	t.Parallel()

	example_project.Helper(t, "This test lives in a package with the _test suffix")
}
