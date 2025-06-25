package golang_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/golang"
	"github.com/smartcontractkit/branch-out/internal/testhelpers"
	"github.com/smartcontractkit/branch-out/logging"
)

const exampleProjectDir = "example_project"

var exampleProjectPackages = []string{
	"github.com/smartcontractkit/branch-out/golang/example_project",
	"github.com/smartcontractkit/branch-out/golang/example_project/nested_go_mod",
	"github.com/smartcontractkit/branch-out/golang/example_project/package_name_doesnt_match_dir_name",
}

func TestPackages(t *testing.T) {
	t.Parallel()

	l := testhelpers.Logger(t, logging.WithConsoleLog(true))
	packages, err := golang.Packages(l, exampleProjectDir)
	require.NoError(t, err)

	for _, pkg := range exampleProjectPackages {
		pkgInfo, err := packages.Get(pkg)
		require.NoError(t, err, "package should be found")

		t.Log(pkgInfo)
	}
}
