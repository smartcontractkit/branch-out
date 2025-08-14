//go:build example_project

package nested_project

import (
	"fmt"
	"testing"

	example_project "github.com/smartcontractkit/branch-out-example-project"
)

// TestPassSubTestsStatic shows some subtests with static string names.
func TestPassSubTestsStatic(t *testing.T) {
	t.Parallel()

	t.Run("subtest 1", func(t *testing.T) {
		t.Parallel()

		example_project.Helper(t, "This is a static sub test inside of a nested Go project")
	})

	t.Run("subtest 2", func(t *testing.T) {
		t.Parallel()

		example_project.Helper(t, "This is a static sub test inside of a nested Go project")
	})

	example_project.Helper(t, "This is a parent test of static sub tests")
}

// TestPassSubTestsTableStatic uses table tests with static string names
func TestPassSubTestsTableStatic(t *testing.T) {
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

			example_project.Helper(t, "This is a static table sub test inside of a nested Go project")
		})
	}

	example_project.Helper(t, "This is a parent test of static table sub tests")
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

			example_project.Helper(
				t,
				"This is a dynamic sub test inside of a nested Go project",
			)
		})
	}

	example_project.Helper(t, "This is a parent test of dynamic sub tests")
}

func TestSubSubTestsStatic(t *testing.T) {
	t.Parallel()

	t.Run("parent subtest", func(t *testing.T) {
		t.Parallel()

		t.Run("sub-subtest 1", func(t *testing.T) {
			t.Parallel()

			example_project.Helper(t, "This is a static sub-subtest")
		})

		t.Run("sub-subtest 2", func(t *testing.T) {
			t.Parallel()

			example_project.Helper(t, "This is a static sub-subtest")
		})
	})

	example_project.Helper(t, "This is a parent test of static sub-subtests")
}
