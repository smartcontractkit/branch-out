// Package cmd provides the CLI for the branch-out application.
package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/fang"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/smartcontractkit/branch-out/config"
	"github.com/smartcontractkit/branch-out/logging"
	"github.com/smartcontractkit/branch-out/processing"
)

var (
	v         = viper.New()
	appConfig config.Config
	logger    zerolog.Logger
)

// root is the root command for the CLI.
var root = &cobra.Command{
	Use:   "branch-out",
	Short: "Branch Out accentuates the capabilities of Trunk.io's flaky test tools",
	Long: `
Branch Out accentuates the capabilities of Trunk.io's flaky test tools by branching out to other common services for a flaky test flow.

It does so by running a server that listens for webhooks from Trunk.io's flaky test tool. When a test is marked as flaky, Branch Out will:

1. Create a new Jira ticket to fix the flaky test
2. Create PR to quarantine the flaky test on GitHub

When a test is marked as not flaky, Branch Out will:

1. Update the Jira ticket to reflect the test is no longer flaky
2. Make a PR to un-quarantine the flaky test on GitHub

Configuration is read from CLI flags > environment variables > a .env file.
`,
	Example: `
# Default run
branch-out
# Run with debug logging, log to file, and bind on 8080
branch-out --log-level debug --log-path branch-out.log --port 8080
# Provide GitHub Token config via CLI flag
branch-out --github-token <github-token-value>
# Configure Jira integration
branch-out --jira-base-domain mycompany.atlassian.net --jira-project-key PROJ
`,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		var err error

		appConfig, err = config.Load(
			config.WithViper(v),
			config.WithCommand(cmd),
		)
		if err != nil {
			return err
		}

		opts := []logging.Option{
			logging.WithLevel(appConfig.LogLevel),
			logging.WithFileName(appConfig.LogPath),
			logging.WithSecrets(appConfig.GetSecrets()),
		}

		logger, err = logging.New(opts...)
		if err != nil {
			return err
		}

		logger.Debug().Str("log_level", appConfig.LogLevel).Int("port", appConfig.Port).Msg("Loaded config")
		marshaled, err := appConfig.MarshalJSON()
		if err != nil {
			logger.Warn().Err(err).Msg("Failed to marshal config for logging.")
		}
		logger.Debug().Str("config", string(marshaled)).Msg("Configuration")

		return nil
	},
	RunE: func(cmd *cobra.Command, _ []string) error {
		srv, err := processing.NewServer(
			processing.WithLogger(logger),
			processing.WithConfig(appConfig),
		)
		if err != nil {
			return fmt.Errorf("failed to create server: %w", err)
		}

		err = srv.Start(cmd.Context())
		if err != nil {
			logger.Error().Err(err).Msg("Server failure")
		}
		return err
	},
}

func init() {
	config.MustBindConfig(root, v)
}

// Execute is the entry point for the CLI.
func Execute() {
	if err := fang.Execute(context.Background(), root, fang.WithVersion(config.VersionString())); err != nil {
		os.Exit(1)
	}
}
