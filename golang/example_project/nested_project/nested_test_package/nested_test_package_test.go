//go:build example_project

package nested_test_package_test

import (
	"testing"

	example_project "github.com/smartcontractkit/branch-out-example-project"
)

func TestTestPackage(t *testing.T) {
	t.Parallel()

	example_project.Helper(t, "This test lives in a package with the _test suffix in a nested project")
}
