module github.com/smartcontractkit/branch-out-example-project/nested_project

go 1.25.0

replace (
	github.com/smartcontractkit/branch-out-example-project => ../
	// Get proper local version of branch-out
	github.com/smartcontractkit/branch-out => ../../../
)

require github.com/smartcontractkit/branch-out-example-project v0.0.0-00010101000000-000000000000
