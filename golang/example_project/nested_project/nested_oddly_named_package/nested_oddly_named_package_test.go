//go:build example_project

package nested_package_name_doesnt_match_dir_name

import "testing"

func TestOddlyNamedPackage(t *testing.T) {
	t.Parallel()

	t.Log(
		"This test's package name doesn't match the standard directory path naming scheme in a nested project and will fail unless it's skipped",
	)
	t.Fail()
}
