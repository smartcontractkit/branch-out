//go:build example_project

package example_project

import (
	"fmt"
	"testing"
)

func FuzzExampleProject(f *testing.F) {
	f.Add(1)

	f.Fuzz(func(t *testing.T, i int) {
		Helper(t, fmt.Sprintf("This is a fuzz test seed #%d", i))
	})

	Helper(f, "This is a fuzz test")
}

func TestDifferentParam(x *testing.T) {
	x.Parallel()

	Helper(x, "This test uses 'x' instead of 't'")
}
