//go:build example_project

package nested_project

import (
	"fmt"
	"testing"

	"github.com/smartcontractkit/branch-out/golang/example_project"
)

func BenchmarkExampleProject(b *testing.B) {
	example_project.Helper(b, "This benchmark is in a nested project and will fail unless it's skipped")
}

func FuzzExampleProject(f *testing.F) {
	f.Add(1)

	f.Fuzz(func(t *testing.T, i int) {
		example_project.Helper(
			t,
			fmt.Sprintf("This fuzz test is in a nested project and will fail unless it's skipped #%d", i),
		)
	})
}

func TestDifferentParam(x *testing.T) {
	example_project.Helper(
		x,
		"This test with a different param name for testing.T is in a nested project and will fail unless it's skipped",
	)
}
