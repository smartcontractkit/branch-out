package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/smartcontractkit/branch-out/github"
	"github.com/smartcontractkit/branch-out/golang"
)

var (
	repoURL string
	targets []string
)

var quarantineCmd = &cobra.Command{
	Use:   "quarantine",
	Short: "Quarantine tests in a Go project",
	PreRunE: func(_ *cobra.Command, _ []string) error {
		if githubToken == "" {
			return errors.New("github-token is required")
		}
		if repoURL == "" {
			return errors.New("repo-url is required")
		}
		if len(targets) == 0 {
			return errors.New("targets are required")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, _ []string) error {
		owner, repo, err := github.ParseRepoURL(repoURL)
		if err != nil {
			return fmt.Errorf("failed to parse repo URL: %w", err)
		}

		githubClient := github.NewClient(logger, github.WithToken(githubToken))

		quarantineTargets, err := parseTargets(targets)
		if err != nil {
			return fmt.Errorf("failed to parse targets: %w", err)
		}

		err = githubClient.QuarantineTests(cmd.Context(), logger, owner, repo, quarantineTargets)
		if err != nil {
			return fmt.Errorf("failed to quarantine tests: %w", err)
		}

		return nil
	},
}

func init() {
	root.AddCommand(quarantineCmd)

	quarantineCmd.Flags().
		StringVar(&repoURL, "repo-url", "", "The URL of the GitHub repository to quarantine tests in")
	quarantineCmd.Flags().
		StringSliceVar(&targets, "targets", []string{}, "The targets to quarantine tests for (e.g. 'github.com/owner/repo/pkg.TestName, github.com/owner/repo/pkg.TestName2')")
}

func parseTargets(targets []string) ([]golang.QuarantineTarget, error) {
	quarantineTargets := make([]golang.QuarantineTarget, 0, len(targets))

	for _, target := range targets {
		lastDotIndex := strings.LastIndex(target, ".")
		if lastDotIndex == -1 {
			return nil, fmt.Errorf("invalid target format '%s': expected 'package.TestName'", target)
		}

		pkg := target[:lastDotIndex]
		test := target[lastDotIndex+1:]

		quarantineTargets = append(quarantineTargets, golang.QuarantineTarget{
			Package: pkg,
			Tests:   []string{test},
		})
	}

	return quarantineTargets, nil
}
