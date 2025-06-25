//go:build example_project

package example_project

import (
	"fmt"
	"testing"
)

// TestPassSubTestsStatic shows some subtests with static string names.
func TestPassSubTestsStatic(t *testing.T) {
	t.Parallel()

	t.Run("subtest 1", func(t *testing.T) {
		t.Parallel()

		testHelper(t)
	})

	t.Run("subtest 2", func(t *testing.T) {
		testHelper(t)
	})
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

			testHelper(t)
		})
	}
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

			testHelper(t)
		})
	}
}
