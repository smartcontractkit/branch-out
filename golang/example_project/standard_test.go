//go:build example_project

package example_project

import (
	"testing"
)

func TestStandard1(t *testing.T) {
	t.Parallel()

	Helper(t, "This is a standard test")
}

func TestStandard2(t *testing.T) {
	t.Parallel()

	Helper(t, "This is a standard test")
}

func TestStandard3(t *testing.T) {
	t.Parallel()

	Helper(t, "This is a standard test")
}
