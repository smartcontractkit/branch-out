package package_name_doesnt_match_dir_name

import "testing"

func TestOddlyNamedPackage(t *testing.T) {
	t.Parallel()

	t.Fail()
	t.Log("This test will fail unless it's skipped")
}
