module github.com/smartcontractkit/branch-out-example-project

go 1.24.6

replace (
	// Get proper local version of branch-out
	github.com/smartcontractkit/branch-out => ../..
)
