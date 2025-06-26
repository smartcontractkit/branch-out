//go:build example_project

package example_project

import (
	"fmt"
	"testing"
)

// TestSubTestsStatic shows some subtests with static string names.
func TestSubTestsStatic(t *testing.T) {
	t.Parallel()

	t.Run("subtest 1", func(t *testing.T) {
		t.Parallel()

		Helper(t, "This is a static subtest")
	})

	t.Run("subtest 2", func(t *testing.T) {
		t.Parallel()

		Helper(t, "This is a static subtest")
	})

	Helper(t, "This is a parent test of static subtests")
}

// TestSubTestsTableStatic uses table tests with static string names
func TestSubTestsTableStatic(t *testing.T) {
	t.Parallel()

	subtests := []struct {
		name string
	}{
		{name: "subtest 1"},
		{name: "subtest 2"},
	}

	for _, subtest := range subtests {
		t.Run(subtest.name, func(t *testing.T) {
			t.Parallel()

			Helper(t, "This is a static table test test")
		})
	}

	Helper(t, "This is a parent test of static table tests")
}

// TestSubTestsTableDynamic uses table tests with dynamic string names
func TestSubTestsTableDynamic(t *testing.T) {
	t.Parallel()

	subtests := []struct {
		name string
		num  int
	}{
		{num: 1},
		{num: 2},
	}

	for _, subtest := range subtests {
		t.Run(fmt.Sprintf("subtest %d", subtest.num), func(t *testing.T) {
			t.Parallel()

			Helper(t, "This is a dynamic table test")
		})
	}

	Helper(t, "This is a parent test of dynamic table tests")
}
