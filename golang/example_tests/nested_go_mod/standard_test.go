//go:build example_tests

package example_tests

import "testing"

func TestStandard1(t *testing.T) {
	t.Parallel()

	testHelper(t)
}

func TestStandard2(t *testing.T) {
	t.Parallel()

	testHelper(t)
}

func TestStandard3(t *testing.T) {
	t.Parallel()

	testHelper(t)
}

func testHelper(t *testing.T) {
	t.Helper()

	t.Fail()
	t.Log("This test will fail unless it's skipped")
}
