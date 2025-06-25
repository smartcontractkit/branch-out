//go:build example_project

package nested_project

import "testing"

func TestStandard1(t *testing.T) {
	t.Parallel()

	standardTestHelper(t)
}

func TestStandard2(t *testing.T) {
	t.Parallel()

	standardTestHelper(t)
}

func TestStandard3(t *testing.T) {
	t.Parallel()

	standardTestHelper(t)
}

func standardTestHelper(t *testing.T) {
	t.Helper()

	t.Log(
		"This is a standard test inside of a nested Go project. It will fail unless it's skipped",
	)
	t.Fail()
}
