//go:build example_project

package nested_project

import (
	"testing"

	"github.com/smartcontractkit/branch-out/golang/example_project"
)

func TestStandard1(t *testing.T) {
	t.Parallel()

	example_project.Helper(t, "This is a standard test inside of a nested Go project")
}

func TestStandard2(t *testing.T) {
	t.Parallel()

	example_project.Helper(t, "This is a standard test inside of a nested Go project")
}

func TestStandard3(t *testing.T) {
	t.Parallel()

	example_project.Helper(t, "This is a standard test inside of a nested Go project")
}
