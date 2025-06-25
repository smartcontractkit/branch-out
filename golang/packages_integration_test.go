package golang

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/internal/testhelpers"
)

const exampleProjectDir = "example_project"

var (
	exampleProjectPackages = []string{
		"github.com/smartcontractkit/branch-out/golang/example_project",
		"github.com/smartcontractkit/branch-out/golang/example_project/package_name_doesnt_match_dir_name",
		"github.com/smartcontractkit/branch-out/golang/example_project/testpackage_test",
		"github.com/smartcontractkit/branch-out/golang/example_project/nested_project",
		"github.com/smartcontractkit/branch-out/golang/example_project/nested_project/nested_package_name_doesnt_match_dir_name",
		"github.com/smartcontractkit/branch-out/golang/example_project/nested_project/nested_test_package",
	}
	exampleProjectBuildFlags = []string{"-tags", "example_project"}
)

func TestIntegrationPackages(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration packages test in short mode")
	}

	l := testhelpers.Logger(t)
	packages, err := Packages(l, exampleProjectDir, exampleProjectBuildFlags...)
	require.NoError(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			l.Error().Msg("Test failed, showing all packages found")
			t.Log(packages.String())
		}
	})

	for _, pkg := range exampleProjectPackages {
		_, err := packages.Get(pkg)
		assert.NoError(t, err, "package should be found")
	}
}
