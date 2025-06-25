package example_project

import "testing"

func BenchmarkExampleProject(b *testing.B) {
	b.Fail()
	b.Log("This benchmark will fail unless it's skipped")
}

func FuzzExampleProject(f *testing.F) {
	f.Add(1)

	f.Fail()
	f.Log("This fuzz test will fail unless it's skipped")
}

func TestDifferentParam(x *testing.T) {
	x.Fail()
	x.Log("This test will fail unless it's skipped")
}
