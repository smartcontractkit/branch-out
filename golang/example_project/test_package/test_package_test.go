//go:build example_project

package test_package_test

import "testing"

func TestTestPackage(t *testing.T) {
	t.Fail()
	t.Log("This test lives in a package with the _test suffix. It will fail unless it's skipped")
}
