//go:build example_project

package nested_test_package_test

import (
	"testing"

	"github.com/smartcontractkit/branch-out/golang/example_project"
)

func TestTestPackage(t *testing.T) {
	example_project.Helper(t, "This test lives in a package with the _test suffix in a nested project")
}
