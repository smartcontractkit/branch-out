//go:build example_project

package nested_test_package_test

import "testing"

func TestTestPackage(t *testing.T) {
	t.Fail()
	t.Log(
		"This test lives in a package with the _test suffix in a nested project and will fail unless it's skipped",
	)
}
