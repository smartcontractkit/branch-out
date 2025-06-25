//go:build example_project

package nested_project

import "testing"

func BenchmarkExampleProject(b *testing.B) {
	b.Fail()
	b.Log("This benchmark is in a nested project and will fail unless it's skipped")
}

func FuzzExampleProject(f *testing.F) {
	f.Add(1)

	f.Fail()
	f.Log("This fuzz test is in a nested project and will fail unless it's skipped")
}

func TestDifferentParam(x *testing.T) {
	x.Fail()
	x.Log(
		"This test with a different param name for testing.T is in a nested project and will fail unless it's skipped",
	)
}
