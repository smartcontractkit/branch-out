// Package github provides utilities for manipulating GitHub branches and PRs.
package github

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/smartcontractkit/branch-out/golang"
)

// QuarantineTests quarantines multiple Go tests by adding t.Skip() to the test functions and making a PR to the default branch.
func (c *Client) QuarantineTests(
	ctx context.Context,
	l zerolog.Logger,
	repoURL string,
	targets []golang.TestTarget,
	options ...FlakyTestOption,
) error {
	return processTests(ctx, c, l, repoURL, targets, TestOperationConfig[golang.QuarantineResults]{
		OperationType: "quarantine",
		PRTitlePrefix: "Quarantine Flaky Tests",
		CoreFunc: func(l zerolog.Logger, repoPath string, targets []golang.TestTarget, buildFlagsOpt ...interface{}) (golang.QuarantineResults, error) {
			if len(buildFlagsOpt) > 0 {
				if opt, ok := buildFlagsOpt[0].(golang.QuarantineOption); ok {
					return golang.QuarantineTests(l, repoPath, targets, opt)
				}
			}
			return golang.QuarantineTests(l, repoPath, targets)
		},
		BuildFlagsOption: func(buildFlags []string) interface{} {
			return golang.WithBuildFlags(buildFlags)
		},
		MetricsInc:      c.metrics.IncQuarantineOperation,
		MetricsRecord:   c.metrics.RecordQuarantineFilesModified,
		MetricsDuration: c.metrics.RecordQuarantineDuration,
		LogMessage:      "Created or updated pull request",
	}, options...)
}
