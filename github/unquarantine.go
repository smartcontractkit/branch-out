package github

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/smartcontractkit/branch-out/golang"
)

// UnquarantineTests unquarantines multiple Go tests by removing the skip logic from test functions and making a PR to the default branch.
func (c *Client) UnquarantineTests(
	ctx context.Context,
	l zerolog.Logger,
	repoURL string,
	targets []golang.TestTarget,
	options ...FlakyTestOption,
) error {
	return processTests(ctx, c, l, repoURL, targets, TestOperationConfig[golang.Results]{
		OperationType: "unquarantine",
		PRTitlePrefix: "Unquarantine Recovered Tests",
		CoreFunc: func(l zerolog.Logger, repoPath string, targets []golang.TestTarget, buildFlagsOpt ...interface{}) (golang.Results, error) {
			if len(buildFlagsOpt) > 0 {
				if opt, ok := buildFlagsOpt[0].(golang.Option); ok {
					return golang.UnquarantineTests(l, repoPath, targets, opt)
				}
			}
			return golang.UnquarantineTests(l, repoPath, targets)
		},
		BuildFlagsOption: func(buildFlags []string) interface{} {
			return golang.WithBuildFlags(buildFlags)
		},
		MetricsInc:      c.metrics.IncUnquarantineOperation,
		MetricsRecord:   c.metrics.RecordUnquarantineFilesModified,
		MetricsDuration: c.metrics.RecordUnquarantineDuration,
		LogMessage:      "Created or updated unquarantine pull request",
	}, options...)
}
