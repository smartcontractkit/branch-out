//go:build example_project

package package_name_doesnt_match_dir_name

import (
	"testing"

	example_project "github.com/smartcontractkit/branch-out-example-project"
)

func TestOddlyNamedPackage(t *testing.T) {
	t.Parallel()

	example_project.Helper(t, "This test's package name doesn't match the standard directory path naming scheme")
}
